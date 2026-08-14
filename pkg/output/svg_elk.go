package output

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/michielvha/logger"
	sgraph "github.com/vhco-pro/stackgraph/pkg/graph"
)

//go:embed elk.bundled.js
var elkJS string

// ElkNode is the JSON structure ELK expects for input and returns with positions.
type ElkNode struct {
	ID            string            `json:"id"`
	Width         float64           `json:"width,omitempty"`
	Height        float64           `json:"height,omitempty"`
	X             float64           `json:"x,omitempty"`
	Y             float64           `json:"y,omitempty"`
	Children      []*ElkNode        `json:"children,omitempty"`
	Labels        []*ElkLabel       `json:"labels,omitempty"`
	Edges         []*ElkEdge        `json:"edges,omitempty"`
	Ports         []*ElkPort        `json:"ports,omitempty"`
	LayoutOptions map[string]string `json:"layoutOptions,omitempty"`
}

// ElkLabel defines a label on a node (used by ELK to compute padding for labels).
type ElkLabel struct {
	Text   string  `json:"text"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// ElkPort defines an edge attachment point on a node.
type ElkPort struct {
	ID            string            `json:"id"`
	LayoutOptions map[string]string `json:"layoutOptions,omitempty"`
}

// ElkEdge represents an edge in the ELK graph.
type ElkEdge struct {
	ID        string       `json:"id"`
	Sources   []string     `json:"sources"`
	Targets   []string     `json:"targets"`
	Sections  []ElkSection `json:"sections,omitempty"`
	Container string       `json:"container,omitempty"`
}

// ElkSection contains the routed edge path from ELK.
type ElkSection struct {
	StartPoint ElkPoint   `json:"startPoint"`
	EndPoint   ElkPoint   `json:"endPoint"`
	BendPoints []ElkPoint `json:"bendPoints,omitempty"`
}

// ElkPoint is an x,y coordinate.
type ElkPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// RenderSVG produces a self-contained SVG using ELK.js for layout with
// orthogonal edge routing and base64-embedded icons.
func RenderSVG(g *sgraph.Graph) ([]byte, error) {
	if len(g.Nodes) == 0 {
		return []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="400" height="80">` +
			`<text x="200" y="40" text-anchor="middle" font-family="Sans-Serif" font-size="14" fill="#6c757d">No resources found</text></svg>`), nil
	}

	// Build ELK graph JSON
	elkGraph, edgeMeta := buildElkGraph(g)

	// Run ELK layout via goja
	positions, edges, err := runElkLayout(elkGraph)
	if err != nil {
		return nil, fmt.Errorf("ELK layout failed: %w", err)
	}

	// Assemble the SVG from the shared building blocks in svg.go
	svg := generateElkSVG(g, positions, edges, edgeMeta)
	return []byte(svg), nil
}

func buildElkGraph(g *sgraph.Graph) (*ElkNode, map[string]*sgraph.Edge) {
	nodeIdx := make(map[string]*sgraph.Node)
	childMap := make(map[string][]string)
	for _, n := range g.Nodes {
		nodeIdx[n.ID] = n
		if n.Parent != "" {
			childMap[n.Parent] = append(childMap[n.Parent], n.ID)
		}
	}

	// Find root nodes
	var roots []string
	for _, n := range g.Nodes {
		if n.Parent == "" {
			roots = append(roots, n.ID)
		}
	}

	// Build ancestry check for edge filtering
	parentOf := make(map[string]string)
	for _, n := range g.Nodes {
		if n.Parent != "" {
			parentOf[n.ID] = n.Parent
		}
	}
	isAncestor := func(nodeID, ancestorID string) bool {
		cur := nodeID
		for {
			p, ok := parentOf[cur]
			if !ok || p == "" {
				return false
			}
			if p == ancestorID {
				return true
			}
			cur = p
		}
	}

	// Recursively build ELK node tree
	var buildNode func(id string) *ElkNode
	buildNode = func(id string) *ElkNode {
		n := nodeIdx[id]
		if n == nil {
			return nil
		}

		w, h := nodeSize(n)
		elkNode := &ElkNode{
			ID:     id,
			Width:  float64(w),
			Height: float64(h),
		}

		kids := childMap[id]
		if len(kids) > 0 || n.Type == sgraph.NodeTypeGroup {
			// Container — ensure minimum width accommodates label text
			label := containerLabel(n)
			minLabelWidth := float64(len([]rune(label))*8 + 50) // rough char width + icon + padding
			if elkNode.Width < minLabelWidth {
				elkNode.Width = minLabelWidth
			}

			elkNode.LayoutOptions = map[string]string{
				"elk.padding":              "[top=40,left=30,bottom=20,right=30]",
				"elk.nodeSize.constraints": "[MINIMUM_SIZE]",
				"elk.nodeSize.minimum":     fmt.Sprintf("(%d, 80)", int(minLabelWidth)),
			}
			for _, kid := range kids {
				child := buildNode(kid)
				if child != nil {
					elkNode.Children = append(elkNode.Children, child)
				}
			}
		}
		// Leaf nodes: no ports, no layout options — ELK auto-creates ports
		// and chooses the best attachment side (N/S/E/W)

		return elkNode
	}

	// Build root graph — ALL layout options go on root when using INCLUDE_CHILDREN
	root := &ElkNode{
		ID: "root",
		LayoutOptions: map[string]string{
			// Core algorithm
			"elk.algorithm":         "layered",
			"elk.direction":         "RIGHT",
			"elk.hierarchyHandling": "INCLUDE_CHILDREN",
			"elk.edgeRouting":       "ORTHOGONAL",

			// Node spacing
			"elk.spacing.nodeNode":                      "60",
			"elk.layered.spacing.nodeNodeBetweenLayers": "80",

			// Edge spacing
			"elk.spacing.edgeEdge":                      "15",
			"elk.spacing.edgeNode":                      "20",
			"elk.layered.spacing.edgeEdgeBetweenLayers": "20",
			"elk.layered.spacing.edgeNodeBetweenLayers": "20",

			// Canvas padding
			"elk.padding": "[top=40,left=40,bottom=40,right=40]",

			// Layout strategies
			"elk.layered.crossingMinimization.strategy":      "LAYER_SWEEP",
			"elk.layered.nodePlacement.strategy":             "NETWORK_SIMPLEX",
			"elk.layered.compaction.postCompaction.strategy": "EDGE_LENGTH",
		},
	}

	for _, rid := range roots {
		child := buildNode(rid)
		if child != nil {
			root.Children = append(root.Children, child)
		}
	}

	// Add edges — reference node IDs directly (NOT port IDs).
	// ELK auto-creates ports and chooses the best attachment side.
	// edgeMeta lets the SVG stage recover each routed edge's type and label.
	edgeMeta := make(map[string]*sgraph.Edge)
	edgeIdx := 0
	for _, e := range g.Edges {
		if isAncestor(e.Source, e.Target) || isAncestor(e.Target, e.Source) {
			continue
		}
		if nodeIdx[e.Source] == nil || nodeIdx[e.Target] == nil {
			logger.Debugf("skipping ELK edge %s -> %s: node not found in graph", e.Source, e.Target)
			continue
		}

		id := fmt.Sprintf("e%d", edgeIdx)
		edgeMeta[id] = e
		root.Edges = append(root.Edges, &ElkEdge{
			ID:      id,
			Sources: []string{e.Source},
			Targets: []string{e.Target},
		})
		edgeIdx++
	}

	return root, edgeMeta
}

// layoutTimeout bounds a single ELK layout computation. A pathological graph
// (or an ELK bug) must fail with an error, never hang the process.
const layoutTimeout = 30 * time.Second

// elkVM caches the goja VM with the 3.6MB ELK bundle already parsed, so
// repeated renders in one process (library use, tests) skip the ~300ms parse.
// goja VMs are not goroutine-safe; elkVMMu serializes layout runs.
var (
	elkVM   *goja.Runtime
	elkVMMu sync.Mutex
)

func getElkVM() (*goja.Runtime, error) {
	if elkVM != nil {
		return elkVM, nil
	}

	vm := goja.New()

	// Setup stubs — the ELK bundle expects a console and setTimeout.
	// setTimeout must exist BEFORE the bundle initializes.
	if _, err := vm.RunString(`
		var console = { log: function(){}, warn: function(){}, error: function(){}, info: function(){} };
		var setTimeout = function(f) { f(); };
	`); err != nil {
		return nil, fmt.Errorf("failed to setup JS stubs: %w", err)
	}

	if _, err := vm.RunString(elkJS); err != nil {
		return nil, fmt.Errorf("failed to load elk.js: %w", err)
	}

	if _, err := vm.RunString(`var elk = new ELK();`); err != nil {
		return nil, fmt.Errorf("failed to create ELK instance: %w", err)
	}

	elkVM = vm
	return vm, nil
}

func runElkLayout(elkGraph *ElkNode) (positions map[string]NodePosition, elkEdges []*ElkEdge, retErr error) {
	elkVMMu.Lock()
	defer elkVMMu.Unlock()

	vm, err := getElkVM()
	if err != nil {
		return nil, nil, err
	}
	// A previous run's timeout callback may have latched an interrupt after
	// its ClearInterrupt (Stop returning false while the callback runs) —
	// clear defensively before use.
	vm.ClearInterrupt()

	// A layout failure (timeout interrupt, ELK exception) can leave the cached
	// VM with corrupted mid-layout state — discard it so the next render
	// starts from a fresh bundle instead of inheriting the wreckage.
	defer func() {
		if retErr != nil {
			elkVM = nil
		}
	}()

	// Interrupt the VM if layout exceeds the timeout. Defers run LIFO: the
	// timer must be stopped BEFORE the interrupt is cleared, or a timer firing
	// in between would poison the cached VM for every later render.
	timer := time.AfterFunc(layoutTimeout, func() {
		vm.Interrupt(fmt.Sprintf("ELK layout exceeded %s", layoutTimeout))
	})
	defer vm.ClearInterrupt()
	defer timer.Stop()

	// Serialize graph to JSON
	graphJSON, err := json.Marshal(elkGraph)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal ELK graph: %w", err)
	}

	if err := vm.Set("graphJSON", string(graphJSON)); err != nil {
		return nil, nil, fmt.Errorf("failed to set graphJSON in JS: %w", err)
	}

	// Run layout — ELK returns a Promise, which the bundled build resolves
	// synchronously under our immediate setTimeout stub.
	if _, err := vm.RunString(`
		var layoutResult = null;
		var layoutError = null;
		elk.layout(JSON.parse(graphJSON)).then(function(result) {
			layoutResult = JSON.stringify(result);
		}).catch(function(err) {
			layoutError = err.toString();
		});
	`); err != nil {
		return nil, nil, fmt.Errorf("failed to run ELK layout: %w", err)
	}

	errVal, err := vm.RunString(`layoutError`)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read ELK error state: %w", err)
	}
	if errVal != nil && !goja.IsUndefined(errVal) && !goja.IsNull(errVal) {
		return nil, nil, fmt.Errorf("ELK layout error: %s", errVal.String())
	}

	resultVal, err := vm.RunString(`layoutResult`)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read ELK result: %w", err)
	}
	if resultVal == nil || goja.IsUndefined(resultVal) || goja.IsNull(resultVal) {
		return nil, nil, fmt.Errorf("ELK layout returned no result (promise may not have resolved)")
	}

	// Parse the result
	var result ElkNode
	if err := json.Unmarshal([]byte(resultVal.String()), &result); err != nil {
		return nil, nil, fmt.Errorf("failed to parse ELK result: %w", err)
	}

	// Release the per-render strings held by the cached VM's globals so a
	// large graph doesn't stay pinned in memory for the process lifetime.
	if _, err := vm.RunString(`graphJSON = null; layoutResult = null;`); err != nil {
		logger.Debugf("failed to clear ELK VM globals: %v", err)
	}

	// Extract positions recursively
	positions = make(map[string]NodePosition)
	var extractPositions func(node *ElkNode, offsetX, offsetY float64)
	extractPositions = func(node *ElkNode, offsetX, offsetY float64) {
		absX := offsetX + node.X
		absY := offsetY + node.Y
		positions[node.ID] = NodePosition{
			X:      absX + node.Width/2, // NodePosition is center-based; ELK reports top-left
			Y:      absY + node.Height/2,
			Width:  node.Width,
			Height: node.Height,
		}
		for _, child := range node.Children {
			extractPositions(child, absX, absY)
		}
	}
	for _, child := range result.Children {
		extractPositions(child, 0, 0)
	}

	// Build container absolute offset map (same offsets used for nodes)
	containerOffsets := make(map[string][2]float64) // container ID -> (absX, absY)
	containerOffsets["root"] = [2]float64{0, 0}
	var buildOffsets func(node *ElkNode, offX, offY float64)
	buildOffsets = func(node *ElkNode, offX, offY float64) {
		absX := offX + node.X
		absY := offY + node.Y
		containerOffsets[node.ID] = [2]float64{absX, absY}
		for _, child := range node.Children {
			buildOffsets(child, absX, absY)
		}
	}
	for _, child := range result.Children {
		buildOffsets(child, 0, 0)
	}

	// Extract edges from ALL containers and translate to absolute coordinates
	edges := elkEdges
	var collectEdges func(node *ElkNode)
	collectEdges = func(node *ElkNode) {
		for _, edge := range node.Edges {
			containerID := edge.Container
			if containerID == "" {
				containerID = node.ID
			}
			offset := containerOffsets[containerID]

			for si := range edge.Sections {
				edge.Sections[si].StartPoint.X += offset[0]
				edge.Sections[si].StartPoint.Y += offset[1]
				edge.Sections[si].EndPoint.X += offset[0]
				edge.Sections[si].EndPoint.Y += offset[1]
				for bi := range edge.Sections[si].BendPoints {
					edge.Sections[si].BendPoints[bi].X += offset[0]
					edge.Sections[si].BendPoints[bi].Y += offset[1]
				}
			}
			edges = append(edges, edge)
		}
		for _, child := range node.Children {
			collectEdges(child)
		}
	}
	collectEdges(&result)

	return positions, edges, nil
}

func generateElkSVG(g *sgraph.Graph, positions map[string]NodePosition, elkEdges []*ElkEdge, edgeMeta map[string]*sgraph.Edge) string {
	// Find canvas bounds
	var maxX, maxY float64
	for _, pos := range positions {
		right := pos.X + pos.Width/2
		bottom := pos.Y + pos.Height/2
		if right > maxX {
			maxX = right
		}
		if bottom > maxY {
			maxY = bottom
		}
	}
	canvasW := int(maxX) + 60
	canvasH := int(maxY) + 60

	var b strings.Builder

	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="%d" height="%d" font-family="Sans-Serif">`, canvasW, canvasH)
	b.WriteString("\n")

	// Dark mode CSS — auto-detects prefers-color-scheme. Base colors come from
	// the shared palette; container role rules are generated from the styles
	// the graph's mappings actually declare.
	fmt.Fprintf(&b, `<style>
  .sg-bg { fill: %s; }
  .sg-text { fill: %s; }
  .sg-text-sm { fill: %s; }
%s  .sg-node-box { fill: %s; stroke: %s; }
  @media (prefers-color-scheme: dark) {
    .sg-bg { fill: %s; }
    .sg-text { fill: %s; }
    .sg-text-sm { fill: %s; }
%s    .sg-node-box { fill: %s; stroke: %s; }
%s  }
</style>`,
		paletteLightBG, paletteLightText, paletteLightText,
		edgeClassCSS(false, "  "),
		paletteLightBG, paletteLightBorder,
		paletteDarkBG, paletteDarkText, paletteDarkTextSm,
		edgeClassCSS(true, "    "),
		paletteDarkBox, paletteDarkBorder,
		roleDarkCSS(g))
	b.WriteString("\n")

	fmt.Fprintf(&b, `<rect class="sg-bg" width="%d" height="%d" fill="%s"/>`, canvasW, canvasH, paletteLightBG)
	b.WriteString("\n")

	// Defs: arrowhead marker plus one shared <image> per distinct icon —
	// nodes reference them with <use> instead of re-embedding the data URI.
	iconRefs := buildIconDefs(&b, g)

	nodeIdx := make(map[string]*sgraph.Node)
	for _, n := range g.Nodes {
		nodeIdx[n.ID] = n
	}

	// Render containers back-to-front
	type containerEntry struct {
		node  *sgraph.Node
		pos   NodePosition
		depth int
	}
	var containers []containerEntry
	for _, n := range g.Nodes {
		pos, ok := positions[n.ID]
		if !ok {
			continue
		}
		if n.Type == sgraph.NodeTypeGroup || len(n.Children) > 0 {
			depth := 0
			cur := n.Parent
			for cur != "" {
				depth++
				if pn := nodeIdx[cur]; pn != nil {
					cur = pn.Parent
				} else {
					break
				}
			}
			containers = append(containers, containerEntry{node: n, pos: pos, depth: depth})
		}
	}
	sort.Slice(containers, func(i, j int) bool {
		return containers[i].depth < containers[j].depth
	})

	// Layer 1: Container rectangles (background)
	for _, c := range containers {
		renderContainerRect(&b, c.node, c.pos)
	}

	// Layer 2: Edges (on top of container backgrounds). Hierarchical edges can
	// carry multiple sections — render every one, arrowhead on the last only.
	// An edge ELK returned no sections for still draws as a straight line
	// between node centers: a crude edge beats a silently missing one.
	for _, edge := range elkEdges {
		meta := edgeMeta[edge.ID]
		if len(edge.Sections) == 0 {
			if len(edge.Sources) > 0 && len(edge.Targets) > 0 {
				src, sok := positions[edge.Sources[0]]
				tgt, tok := positions[edge.Targets[0]]
				if sok && tok {
					renderElkEdge(&b, ElkSection{
						StartPoint: ElkPoint{X: src.X, Y: src.Y},
						EndPoint:   ElkPoint{X: tgt.X, Y: tgt.Y},
					}, true, meta)
				}
			}
			continue
		}
		for si, section := range edge.Sections {
			renderElkEdge(&b, section, si == len(edge.Sections)-1, meta)
		}
	}

	// Layer 3: Container labels + icons (on top of edges so labels are never obscured)
	for _, c := range containers {
		renderContainerLabel(&b, c.node, c.pos, iconRefs)
	}

	// Layer 4: Leaf nodes (on top of everything)
	for _, n := range g.Nodes {
		pos, ok := positions[n.ID]
		if !ok {
			continue
		}
		if n.Type != sgraph.NodeTypeGroup && len(n.Children) == 0 {
			renderNode(&b, n, pos, iconRefs)
		}
	}

	b.WriteString("</svg>\n")
	return b.String()
}

func renderElkEdge(b *strings.Builder, section ElkSection, arrowhead bool, meta *sgraph.Edge) {
	var d strings.Builder

	// Start point
	fmt.Fprintf(&d, "M %.1f,%.1f", section.StartPoint.X, section.StartPoint.Y)

	// Bend points — orthogonal right-angle segments, straight lines
	for _, bp := range section.BendPoints {
		fmt.Fprintf(&d, " L %.1f,%.1f", bp.X, bp.Y)
	}

	// End point
	fmt.Fprintf(&d, " L %.1f,%.1f", section.EndPoint.X, section.EndPoint.Y)

	ec, title := edgeClassByType[""], ""
	if meta != nil {
		ec, title = edgeVisual(meta)
	}
	marker := ""
	if arrowhead {
		marker = ec.markerAttr
	}
	titleEl := ""
	if title != "" {
		titleEl = "<title>" + escapeXMLStr(title) + "</title>"
	}
	fmt.Fprintf(b, `<path class="%s" d="%s" fill="none" stroke="%s" stroke-width="1.5"%s%s>%s</path>`,
		ec.cssClass, d.String(), ec.light, ec.dashAttr, marker, titleEl)
	b.WriteString("\n")
}

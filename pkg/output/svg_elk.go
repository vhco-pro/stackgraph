package output

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dop251/goja"
	"github.com/michielvha/logger"
	sgraph "github.com/michielvha/stackgraph/pkg/graph"
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

// RenderElkSVG produces an SVG using ELK.js for layout with orthogonal edge routing.
func RenderElkSVG(g *sgraph.Graph) ([]byte, error) {
	if len(g.Nodes) == 0 {
		return []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="400" height="80">` +
			`<text x="200" y="40" text-anchor="middle" font-family="Sans-Serif" font-size="14" fill="#6c757d">No resources found</text></svg>`), nil
	}

	// Build ELK graph JSON
	elkGraph := buildElkGraph(g)

	// Run ELK layout via goja
	positions, edges, err := runElkLayout(elkGraph, g)
	if err != nil {
		return nil, fmt.Errorf("ELK layout failed: %w", err)
	}

	// Generate SVG (reuse dagre SVG generation with ELK positions)
	svg := generateElkSVG(g, positions, edges)
	return []byte(svg), nil
}

func buildElkGraph(g *sgraph.Graph) *ElkNode {
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

		w, h := dagreNodeSize(n)
		elkNode := &ElkNode{
			ID:     id,
			Width:  float64(w),
			Height: float64(h),
		}

		kids := childMap[id]
		if len(kids) > 0 || n.Type == sgraph.NodeTypeGroup {
			// Container — ensure minimum width accommodates label text
			label := n.Label
			if n.Service != "" {
				label = n.Service + " — " + n.Label
			}
			minLabelWidth := float64(len(label)*8 + 50) // rough char width + icon + padding
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

	// Add edges — reference node IDs directly (NOT port IDs)
	// ELK auto-creates ports and chooses the best attachment side
	edgeIdx := 0
	for _, e := range g.Edges {
		if isAncestor(e.Source, e.Target) || isAncestor(e.Target, e.Source) {
			continue
		}
		if nodeIdx[e.Source] == nil || nodeIdx[e.Target] == nil {
			logger.Debugf("skipping ELK edge %s -> %s: node not found in graph", e.Source, e.Target)
			continue
		}

		root.Edges = append(root.Edges, &ElkEdge{
			ID:      fmt.Sprintf("e%d", edgeIdx),
			Sources: []string{e.Source},
			Targets: []string{e.Target},
		})
		edgeIdx++
	}

	return root
}

func runElkLayout(elkGraph *ElkNode, _ *sgraph.Graph) (map[string]NodePosition, []*ElkEdge, error) {
	vm := goja.New()

	// Setup stubs — matches D2's approach for loading ELK in goja
	_, err := vm.RunString(`
		var console = { log: function(){}, warn: function(){}, error: function(){}, info: function(){} };
	`)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to setup console stub: %w", err)
	}

	// Load ELK.js
	_, err = vm.RunString(elkJS)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load elk.js: %w", err)
	}

	// Initialize ELK — same as D2's setup.js
	_, err = vm.RunString(`
		var setTimeout = function(f) { f(); };
		var elk = new ELK();
	`)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create ELK instance: %w", err)
	}

	// Serialize graph to JSON
	graphJSON, err := json.Marshal(elkGraph)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal ELK graph: %w", err)
	}

	// Set the graph and run layout
	if err := vm.Set("graphJSON", string(graphJSON)); err != nil {
		return nil, nil, fmt.Errorf("failed to set graphJSON in JS: %w", err)
	}
	_, err = vm.RunString(`var graph = JSON.parse(graphJSON);`)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse graph in JS: %w", err)
	}

	// Run layout — ELK returns a Promise
	_, err = vm.RunString(`
		var layoutResult = null;
		var layoutError = null;
		elk.layout(graph).then(function(result) {
			layoutResult = JSON.stringify(result);
		}).catch(function(err) {
			layoutError = err.toString();
		});
	`)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start ELK layout: %w", err)
	}

	// ELK with the bundled version should resolve synchronously
	// Check if promise resolved
	errVal, _ := vm.RunString(`layoutError`)
	if errVal != nil && !goja.IsUndefined(errVal) && !goja.IsNull(errVal) {
		return nil, nil, fmt.Errorf("ELK layout error: %s", errVal.String())
	}

	resultVal, _ := vm.RunString(`layoutResult`)
	if resultVal == nil || goja.IsUndefined(resultVal) || goja.IsNull(resultVal) {
		return nil, nil, fmt.Errorf("ELK layout returned no result (promise may not have resolved)")
	}

	// Parse the result
	var result ElkNode
	if err := json.Unmarshal([]byte(resultVal.String()), &result); err != nil {
		return nil, nil, fmt.Errorf("failed to parse ELK result: %w", err)
	}

	// Extract positions recursively
	positions := make(map[string]NodePosition)
	var extractPositions func(node *ElkNode, offsetX, offsetY float64)
	extractPositions = func(node *ElkNode, offsetX, offsetY float64) {
		absX := offsetX + node.X
		absY := offsetY + node.Y
		positions[node.ID] = NodePosition{
			X:      absX + node.Width/2, // dagre uses center, ELK uses top-left
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
	var edges []*ElkEdge
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

func generateElkSVG(g *sgraph.Graph, positions map[string]NodePosition, elkEdges []*ElkEdge) string {
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

	// Dark mode CSS (same as dagre)
	b.WriteString(`<style>
  .sg-bg { fill: #FFFFFF; }
  .sg-text { fill: #2D3436; }
  .sg-edge { stroke: #7B8894; }
  .sg-arrow { fill: #7B8894; }
  .sg-node-box { fill: #FFFFFF; stroke: #DEE2E6; }
  @media (prefers-color-scheme: dark) {
    .sg-bg { fill: #1a1a2e; }
    .sg-text { fill: #E0E0E0; }
    .sg-edge { stroke: #9CA3AF; }
    .sg-arrow { fill: #9CA3AF; }
    .sg-node-box { fill: #2a2a3e; stroke: #4A5568; }
    .sg-container-cloud { fill: #1e1e32; stroke: #4A90D9; }
    .sg-container-vpc { fill: #1e1530; stroke: #A855F7; }
    .sg-container-subnet { fill: #1a2e1a; stroke: #8BC34A; }
    .sg-container-sg { fill: #2e1a1a; stroke: #EF5350; }
    .sg-container-default { fill: #1e1e2e; stroke: #6B7280; }
    .sg-label-cloud { fill: #4A90D9; }
    .sg-label-vpc { fill: #C084FC; }
    .sg-label-subnet { fill: #8BC34A; }
    .sg-label-sg { fill: #EF5350; }
    .sg-label-default { fill: #9CA3AF; }
  }
</style>`)
	b.WriteString("\n")

	fmt.Fprintf(&b, `<rect class="sg-bg" width="%d" height="%d" fill="#FFFFFF"/>`, canvasW, canvasH)
	b.WriteString("\n")
	b.WriteString(`<defs><marker id="arrowhead" markerWidth="10" markerHeight="7" refX="10" refY="3.5" orient="auto"><polygon class="sg-arrow" points="0 0, 10 3.5, 0 7" fill="#7B8894"/></marker></defs>`)
	b.WriteString("\n")

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

	// Layer 2: Edges (on top of container backgrounds)
	for _, edge := range elkEdges {
		if len(edge.Sections) == 0 {
			continue
		}
		section := edge.Sections[0]
		renderElkEdge(&b, section)
	}

	// Layer 3: Container labels + icons (on top of edges so labels are never obscured)
	for _, c := range containers {
		renderContainerLabel(&b, c.node, c.pos)
	}

	// Layer 4: Leaf nodes (on top of everything)
	for _, n := range g.Nodes {
		pos, ok := positions[n.ID]
		if !ok {
			continue
		}
		if n.Type != sgraph.NodeTypeGroup && len(n.Children) == 0 {
			renderDagreNode(&b, n, pos)
		}
	}

	b.WriteString("</svg>\n")
	return b.String()
}

func renderElkEdge(b *strings.Builder, section ElkSection) {
	var d strings.Builder

	// Start point
	fmt.Fprintf(&d, "M %.1f,%.1f", section.StartPoint.X, section.StartPoint.Y)

	// Bend points — orthogonal right-angle segments, straight lines
	for _, bp := range section.BendPoints {
		fmt.Fprintf(&d, " L %.1f,%.1f", bp.X, bp.Y)
	}

	// End point
	fmt.Fprintf(&d, " L %.1f,%.1f", section.EndPoint.X, section.EndPoint.Y)

	fmt.Fprintf(b, `<path class="sg-edge" d="%s" fill="none" stroke="#7B8894" stroke-width="1.5" marker-end="url(#arrowhead)"/>`, d.String())
	b.WriteString("\n")
}

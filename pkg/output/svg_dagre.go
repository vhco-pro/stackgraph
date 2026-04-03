package output

import (
	"encoding/base64"
	_ "embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dop251/goja"
	sgraph "github.com/michielvha/stackgraph/pkg/graph"
	"github.com/michielvha/stackgraph/pkg/icons"
)

//go:embed dagre.min.js
var dagreJS string

// NodePosition holds the computed position from dagre layout.
type NodePosition struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// EdgePoints holds the computed edge route from dagre.
type EdgePoints struct {
	Points []Point `json:"points"`
}

// Point is an x,y coordinate.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

const (
	iconSize      = 64
	nodePadding   = 16
	labelHeight   = 20
	nodeWidth     = iconSize + nodePadding*2 // 96
	nodeHeight    = iconSize + labelHeight + nodePadding*2 + 8 // ~124
	boxNodeWidth  = 140
	boxNodeHeight = 52
	groupPadding  = 30
	groupLabelH   = 28
)

// RenderDagreSVG produces a self-contained SVG with dagre.js layout and base64-embedded icons.
func RenderDagreSVG(g *sgraph.Graph) ([]byte, error) {
	if len(g.Nodes) == 0 {
		return []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="400" height="80">` +
			`<text x="200" y="40" text-anchor="middle" font-family="Sans-Serif" font-size="14" fill="#6c757d">No resources found</text></svg>`), nil
	}

	// 1. Compute layout via dagre.js
	positions, edgeRoutes, err := computeDagreLayout(g)
	if err != nil {
		return nil, fmt.Errorf("dagre layout failed: %w", err)
	}

	// 2. Generate SVG
	svg := generateDagreSVG(g, positions, edgeRoutes)
	return []byte(svg), nil
}

func computeDagreLayout(g *sgraph.Graph) (map[string]NodePosition, map[string]EdgePoints, error) {
	vm := goja.New()

	// Load dagre.js
	_, err := vm.RunString(dagreJS)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load dagre.js: %w", err)
	}

	// Create compound graph
	_, err = vm.RunString(`
		var g = new dagre.graphlib.Graph({compound: true});
		g.setGraph({rankdir: "TB", nodesep: 80, ranksep: 100, marginx: 40, marginy: 40});
		g.setDefaultEdgeLabel(function() { return {}; });
	`)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to init dagre graph: %w", err)
	}

	// Build set of direct parent-child pairs to suppress redundant containment edges
	parentChildPairs := make(map[string]bool)
	for _, n := range g.Nodes {
		if n.Parent != "" {
			parentChildPairs[n.ID+"->"+n.Parent] = true
			parentChildPairs[n.Parent+"->"+n.ID] = true
		}
	}

	// Add nodes
	for _, n := range g.Nodes {
		w, h := dagreNodeSize(n)
		script := fmt.Sprintf(`g.setNode(%q, {width: %d, height: %d});`, n.ID, w, h)
		if _, err := vm.RunString(script); err != nil {
			return nil, nil, fmt.Errorf("failed to add node %s: %w", n.ID, err)
		}
	}

	// Set parent relationships
	for _, n := range g.Nodes {
		if n.Parent != "" {
			script := fmt.Sprintf(`g.setParent(%q, %q);`, n.ID, n.Parent)
			if _, err := vm.RunString(script); err != nil {
				return nil, nil, fmt.Errorf("failed to set parent for %s: %w", n.ID, err)
			}
		}
	}

	// Build set of compound (group) node IDs — only groups that actually have children.
	// Empty groups (e.g., security group with no resources inside) are treated as leaf nodes for edge purposes.
	groupIDs := make(map[string]bool)
	childCount := make(map[string]int)
	for _, n := range g.Nodes {
		if n.Parent != "" {
			childCount[n.Parent]++
		}
	}
	for _, n := range g.Nodes {
		if (n.Type == sgraph.NodeTypeGroup || len(n.Children) > 0) && childCount[n.ID] > 0 {
			groupIDs[n.ID] = true
		}
	}

	// Add edges to dagre — only between leaf nodes (not groups).
	// Cross-compound edges cause dagre to crash, so we skip edges
	// involving group nodes for layout purposes. We still render
	// all edges in the SVG by drawing lines between computed positions.
	for _, e := range g.Edges {
		if parentChildPairs[e.Source+"->"+e.Target] {
			continue
		}
		if e.Source == e.Target {
			continue
		}
		// Only add edges between leaf nodes to dagre
		if groupIDs[e.Source] || groupIDs[e.Target] {
			continue
		}
		script := fmt.Sprintf(`g.setEdge(%q, %q);`, e.Source, e.Target)
		if _, err := vm.RunString(script); err != nil {
			continue
		}
	}

	// Run layout
	if _, err := vm.RunString(`dagre.layout(g);`); err != nil {
		return nil, nil, fmt.Errorf("dagre layout failed: %w", err)
	}

	// Extract node positions
	posJSON, err := vm.RunString(`
		var result = {};
		g.nodes().forEach(function(id) {
			result[id] = g.node(id);
		});
		JSON.stringify(result);
	`)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to extract positions: %w", err)
	}

	positions := make(map[string]NodePosition)
	if err := json.Unmarshal([]byte(posJSON.String()), &positions); err != nil {
		return nil, nil, fmt.Errorf("failed to parse positions: %w", err)
	}

	// Extract edge routes
	edgeJSON, err := vm.RunString(`
		var edges = {};
		g.edges().forEach(function(e) {
			var edge = g.edge(e);
			edges[e.v + "->" + e.w] = {points: edge.points || []};
		});
		JSON.stringify(edges);
	`)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to extract edges: %w", err)
	}

	edgeRoutes := make(map[string]EdgePoints)
	if err := json.Unmarshal([]byte(edgeJSON.String()), &edgeRoutes); err != nil {
		return nil, nil, fmt.Errorf("failed to parse edges: %w", err)
	}

	return positions, edgeRoutes, nil
}

func dagreNodeSize(n *sgraph.Node) (w, h int) {
	if n.Type == sgraph.NodeTypeGroup || len(n.Children) > 0 {
		// Groups get minimum size — dagre will expand to fit children
		return 200, 100
	}
	iconPath := getIconPath(n)
	if iconPath != "" {
		return nodeWidth, nodeHeight
	}
	return boxNodeWidth, boxNodeHeight
}

func generateDagreSVG(g *sgraph.Graph, positions map[string]NodePosition, edgeRoutes map[string]EdgePoints) string {
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

	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="%d" height="%d" font-family="Sans-Serif">`, canvasW, canvasH))
	b.WriteString("\n")

	// Background
	b.WriteString(fmt.Sprintf(`<rect width="%d" height="%d" fill="#FFFFFF"/>`, canvasW, canvasH))
	b.WriteString("\n")

	// Arrow marker
	b.WriteString(`<defs><marker id="arrowhead" markerWidth="10" markerHeight="7" refX="10" refY="3.5" orient="auto"><polygon points="0 0, 10 3.5, 0 7" fill="#7B8894"/></marker></defs>`)
	b.WriteString("\n")

	// Build parent→children index for rendering order
	nodeIdx := make(map[string]*sgraph.Node)
	for _, n := range g.Nodes {
		nodeIdx[n.ID] = n
	}

	// Render containers first (back to front), then leaf nodes on top
	// Sort: groups first by depth, then resources
	for _, n := range g.Nodes {
		pos, ok := positions[n.ID]
		if !ok {
			continue
		}
		if n.Type == sgraph.NodeTypeGroup || len(n.Children) > 0 {
			renderDagreContainer(&b, n, pos)
		}
	}
	for _, n := range g.Nodes {
		pos, ok := positions[n.ID]
		if !ok {
			continue
		}
		if n.Type != sgraph.NodeTypeGroup && len(n.Children) == 0 {
			renderDagreNode(&b, n, pos)
		}
	}

	// Render edges
	// Build lookup sets for filtering
	// Build set of groups with actual children (for edge filtering)
	childCounts := make(map[string]int)
	parentOf := make(map[string]string)
	for _, n := range g.Nodes {
		if n.Parent != "" {
			childCounts[n.Parent]++
			parentOf[n.ID] = n.Parent
		}
	}
	groupIDSet := make(map[string]bool)
	for _, n := range g.Nodes {
		if (n.Type == sgraph.NodeTypeGroup || len(n.Children) > 0) && childCounts[n.ID] > 0 {
			groupIDSet[n.ID] = true
		}
	}

	for _, e := range g.Edges {
		// Skip edges between parent and direct child (shown via nesting)
		if parentOf[e.Source] == e.Target || parentOf[e.Target] == e.Source {
			continue
		}
		// Skip edges where BOTH endpoints are containers (not useful visually)
		if groupIDSet[e.Source] && groupIDSet[e.Target] {
			continue
		}

		// Use dagre-computed route if available
		key := e.Source + "->" + e.Target
		if route, ok := edgeRoutes[key]; ok && len(route.Points) >= 2 {
			renderDagreEdge(&b, route.Points)
			continue
		}

		// Fallback: draw line between leaf nodes only
		// If target is a container, skip (containment is shown visually)
		if groupIDSet[e.Target] || groupIDSet[e.Source] {
			continue
		}

		srcPos, sok := positions[e.Source]
		tgtPos, tok := positions[e.Target]
		if sok && tok {
			// Draw from bottom of source to top of target
			renderDagreEdge(&b, []Point{
				{X: srcPos.X, Y: srcPos.Y + srcPos.Height/2},
				{X: tgtPos.X, Y: tgtPos.Y - tgtPos.Height/2},
			})
		}
	}

	b.WriteString("</svg>\n")
	return b.String()
}

func renderDagreContainer(b *strings.Builder, n *sgraph.Node, pos NodePosition) {
	x := pos.X - pos.Width/2
	y := pos.Y - pos.Height/2

	fill, stroke, dash := dagreContainerStyle(n)

	dashAttr := ""
	if dash {
		dashAttr = ` stroke-dasharray="6,3"`
	}

	// Container border
	b.WriteString(fmt.Sprintf(`<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" rx="8" fill="%s" stroke="%s" stroke-width="2"%s/>`,
		x, y, pos.Width, pos.Height, fill, stroke, dashAttr))
	b.WriteString("\n")

	// Label with optional icon (top-left)
	label := n.Label
	if n.Service != "" {
		label = n.Service + " — " + n.Label
	}
	labelColor := dagreContainerLabelColor(n)

	// Check for group icon
	groupIcon := containerGroupIcon(n)
	labelX := x + 12
	if groupIcon != "" {
		iconData := embedIconBase64(groupIcon)
		if iconData != "" {
			// Render small icon before label
			fmt.Fprintf(b, `<image href="%s" x="%.0f" y="%.0f" width="24" height="24"/>`, iconData, x+8, y+4)
			b.WriteString("\n")
			labelX = x + 38 // shift label right of icon
		}
	}

	fmt.Fprintf(b, `<text x="%.0f" y="%.0f" font-size="13" font-weight="600" fill="%s">%s</text>`,
		labelX, y+22, labelColor, escapeXMLStr(label))
	b.WriteString("\n")
}

// containerGroupIcon returns the embedded icon path for container/group labels.
func containerGroupIcon(n *sgraph.Node) string {
	switch n.ResourceType {
	case "aws_cloud":
		return "aws/Groups/AWS-Cloud_32.png"
	case "aws_vpc":
		return "aws/Arch_Networking-Content-Delivery/Arch_Amazon-Virtual-Private-Cloud_64.png"
	case "aws_subnet":
		// Could distinguish public vs private via attributes
		return "aws/Groups/Public-subnet_32.png"
	case "aws_security_group":
		return "aws/Arch_Security-Identity/Arch_AWS-Shield_64.png"
	default:
		return ""
	}
}

func renderDagreNode(b *strings.Builder, n *sgraph.Node, pos NodePosition) {
	cx := pos.X
	cy := pos.Y
	x := cx - pos.Width/2
	y := cy - pos.Height/2

	iconPath := getIconPath(n)
	iconData := ""
	if iconPath != "" {
		iconData = embedIconBase64(iconPath)
	}

	if iconData != "" {
		// Icon-based node: icon centered, label below
		iconX := cx - float64(iconSize)/2
		iconY := y + float64(nodePadding)
		b.WriteString(fmt.Sprintf(`<image href="%s" x="%.0f" y="%.0f" width="%d" height="%d"/>`,
			iconData, iconX, iconY, iconSize, iconSize))
		b.WriteString("\n")

		// Label below icon
		labelY := iconY + float64(iconSize) + 16
		label := n.Label
		if n.Service != "" {
			label = n.Service + "\n" + n.Label
		}
		if n.Count > 1 {
			label += fmt.Sprintf(" (x%d)", n.Count)
		}
		// Split multi-line labels
		lines := strings.Split(label, "\n")
		for i, line := range lines {
			b.WriteString(fmt.Sprintf(`<text x="%.0f" y="%.0f" text-anchor="middle" font-size="11" fill="#2D3436">%s</text>`,
				cx, labelY+float64(i)*14, escapeXMLStr(line)))
			b.WriteString("\n")
		}
	} else {
		// Generic box node
		fill := "#FFFFFF"
		stroke := "#DEE2E6"
		switch n.Action {
		case sgraph.ActionCreate:
			fill = "#E8F5E9"
			stroke = "#43A047"
		case sgraph.ActionDelete:
			fill = "#FFEBEE"
			stroke = "#E53935"
		case sgraph.ActionUpdate:
			fill = "#FFF8E1"
			stroke = "#FB8C00"
		}

		b.WriteString(fmt.Sprintf(`<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" rx="6" fill="%s" stroke="%s" stroke-width="1.5"/>`,
			x, y, pos.Width, pos.Height, fill, stroke))
		b.WriteString("\n")

		label := n.Label
		if n.Service != "" {
			label = n.Service + " — " + n.Label
		}
		if n.Count > 1 {
			label += fmt.Sprintf(" (x%d)", n.Count)
		}
		if len(label) > 24 {
			label = label[:21] + "..."
		}
		b.WriteString(fmt.Sprintf(`<text x="%.0f" y="%.0f" text-anchor="middle" font-size="11" font-weight="500" fill="#2D3436">%s</text>`,
			cx, cy+4, escapeXMLStr(label)))
		b.WriteString("\n")
	}
}

func renderDagreEdge(b *strings.Builder, points []Point) {
	if len(points) < 2 {
		return
	}

	// Dagre points are polyline waypoints — render as straight line segments
	// This matches dagre-d3's default (d3.curveLinear)
	var d strings.Builder
	fmt.Fprintf(&d, "M %.1f,%.1f", points[0].X, points[0].Y)
	for i := 1; i < len(points); i++ {
		fmt.Fprintf(&d, " L %.1f,%.1f", points[i].X, points[i].Y)
	}

	fmt.Fprintf(b, `<path d="%s" fill="none" stroke="#7B8894" stroke-width="1.5" marker-end="url(#arrowhead)"/>`, d.String())
	b.WriteString("\n")
}

func embedIconBase64(iconPath string) string {
	data, err := icons.GetIconBytes(iconPath)
	if err != nil {
		return ""
	}

	ext := strings.ToLower(filepath.Ext(iconPath))
	mime := "image/png"
	if ext == ".svg" {
		mime = "image/svg+xml"
	}

	return fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(data))
}

func dagreContainerStyle(n *sgraph.Node) (fill, stroke string, dashed bool) {
	switch n.ResourceType {
	// Cloud boundaries
	case "aws_cloud":
		return "#FFFFFF", "#232F3E", false
	case "azure_cloud":
		return "#FFFFFF", "#0078D4", false
	case "gcp_cloud":
		return "#FFFFFF", "#4285F4", false

	// AWS
	case "aws_vpc":
		return "#F8F4FF", "#8C4FFF", false
	case "aws_subnet":
		return "#F2F7EE", "#7CB342", false
	case "aws_security_group":
		return "#FFF5F5", "#E53935", true
	case "aws_autoscaling_group":
		return "#DEEBF7", "#FF69B4", true
	case "aws_ecs_cluster", "aws_eks_cluster":
		return "#FFF8E1", "#FF9900", true

	// Azure
	case "azurerm_resource_group":
		return "#F0F8FF", "#0078D4", true
	case "azurerm_virtual_network":
		return "#E8F4FC", "#0078D4", false

	// GCP
	case "google_compute_network":
		return "#E3F2FD", "#4285F4", false
	case "google_compute_subnetwork":
		return "#EDE7F6", "#7C4DFF", false

	default:
		return "#F8F9FA", "#ADB5BD", true
	}
}

func dagreContainerLabelColor(n *sgraph.Node) string {
	switch n.ResourceType {
	case "aws_cloud":
		return "#232F3E"
	case "azure_cloud":
		return "#0078D4"
	case "gcp_cloud":
		return "#4285F4"
	case "aws_vpc":
		return "#6B21A8"
	case "aws_subnet":
		return "#558B2F"
	case "aws_security_group":
		return "#C62828"
	case "azurerm_resource_group", "azurerm_virtual_network":
		return "#0078D4"
	case "google_compute_network":
		return "#1565C0"
	default:
		return "#495057"
	}
}

func escapeXMLStr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}


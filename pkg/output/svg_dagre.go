package output

import (
	"encoding/base64"
	_ "embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
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

	// Build set of nodes that have children in the compound graph
	hasChildren := make(map[string]bool)
	for _, n := range g.Nodes {
		if n.Parent != "" {
			hasChildren[n.Parent] = true
		}
	}

	// Add edges to dagre — only between leaf nodes (not compound parents).
	// Dagre's compound graph crashes on edges involving compound parent nodes.
	// All edges are still drawn in the SVG regardless — this is just for layout.
	for _, e := range g.Edges {
		if e.Source == e.Target {
			continue
		}
		// Skip containment edges
		if parentChildPairs[e.Source+"->"+e.Target] || parentChildPairs[e.Target+"->"+e.Source] {
			continue
		}
		// Skip if either endpoint is a compound parent (dagre limitation)
		if hasChildren[e.Source] || hasChildren[e.Target] {
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

	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="%d" height="%d" font-family="Sans-Serif">`, canvasW, canvasH)
	b.WriteString("\n")

	// Dark mode CSS — auto-detects prefers-color-scheme
	b.WriteString(`<style>
  .sg-bg { fill: #FFFFFF; }
  .sg-text { fill: #2D3436; }
  .sg-text-sm { fill: #2D3436; }
  .sg-edge { stroke: #7B8894; }
  .sg-arrow { fill: #7B8894; }
  .sg-node-box { fill: #FFFFFF; stroke: #DEE2E6; }
  @media (prefers-color-scheme: dark) {
    .sg-bg { fill: #1a1a2e; }
    .sg-text { fill: #E0E0E0; }
    .sg-text-sm { fill: #B0B0B0; }
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

	// Background
	fmt.Fprintf(&b, `<rect class="sg-bg" width="%d" height="%d" fill="#FFFFFF"/>`, canvasW, canvasH)
	b.WriteString("\n")

	// Arrow marker
	b.WriteString(`<defs><marker id="arrowhead" markerWidth="10" markerHeight="7" refX="10" refY="3.5" orient="auto"><polygon class="sg-arrow" points="0 0, 10 3.5, 0 7" fill="#7B8894"/></marker></defs>`)
	b.WriteString("\n")

	// Build parent→children index for rendering order
	nodeIdx := make(map[string]*sgraph.Node)
	for _, n := range g.Nodes {
		nodeIdx[n.ID] = n
	}

	// Render containers back-to-front (outermost first, innermost last)
	// so inner containers render on top of outer ones
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
			// Compute nesting depth
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
	// Sort by depth ascending (outermost drawn first = background)
	sort.Slice(containers, func(i, j int) bool {
		return containers[i].depth < containers[j].depth
	})
	for _, c := range containers {
		renderDagreContainer(&b, c.node, c.pos)
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
	// Build ancestry map for containment check
	parentOf := make(map[string]string)
	for _, n := range g.Nodes {
		if n.Parent != "" {
			parentOf[n.ID] = n.Parent
		}
	}

	// isAncestor checks if ancestor is a parent/grandparent/etc of node
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

	for _, e := range g.Edges {
		// Skip edges where one node is an ancestor of the other (shown via nesting)
		if isAncestor(e.Source, e.Target) || isAncestor(e.Target, e.Source) {
			continue
		}

		// Both endpoints need positions to draw
		srcPos, sok := positions[e.Source]
		tgtPos, tok := positions[e.Target]
		if !sok || !tok {
			continue
		}

		// Use dagre-computed route if available
		key := e.Source + "->" + e.Target
		if route, ok := edgeRoutes[key]; ok && len(route.Points) >= 2 {
			renderDagreEdge(&b, route.Points)
			continue
		}

		// Fallback: draw straight line between node centers (bottom → top)
		renderDagreEdge(&b, []Point{
			{X: srcPos.X, Y: srcPos.Y + srcPos.Height/2},
			{X: tgtPos.X, Y: tgtPos.Y - tgtPos.Height/2},
		})
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

	// Container border — with CSS class for dark mode
	cssClass := dagreContainerCSSClass(n)
	fmt.Fprintf(b, `<rect class="%s" x="%.0f" y="%.0f" width="%.0f" height="%.0f" rx="8" fill="%s" stroke="%s" stroke-width="2"%s/>`,
		cssClass, x, y, pos.Width, pos.Height, fill, stroke, dashAttr)
	b.WriteString("\n")

	// Label with optional icon (top-left)
	label := n.Label
	if n.Service != "" {
		label = n.Service + " — " + n.Label
	}
	labelColor := dagreContainerLabelColor(n)
	labelCSSClass := dagreContainerLabelCSSClass(n)

	// Check for group icon
	groupIcon := containerGroupIcon(n)
	labelX := x + 12
	if groupIcon != "" {
		iconData := embedIconBase64(groupIcon)
		if iconData != "" {
			fmt.Fprintf(b, `<image href="%s" x="%.0f" y="%.0f" width="24" height="24"/>`, iconData, x+8, y+4)
			b.WriteString("\n")
			labelX = x + 38
		}
	}

	fmt.Fprintf(b, `<text class="%s" x="%.0f" y="%.0f" font-size="13" font-weight="600" fill="%s">%s</text>`,
		labelCSSClass, labelX, y+22, labelColor, escapeXMLStr(label))
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
	// Azure
	case "azure_cloud":
		return "" // no embedded Azure logo yet
	case "azurerm_resource_group":
		return "azure/general/10007-icon-service-Resource-Groups.svg"
	case "azurerm_virtual_network":
		return "azure/networking/10061-icon-service-Virtual-Networks.svg"
	case "azurerm_subnet":
		return "azure/networking/10061-icon-service-Virtual-Networks.svg"
	// GCP
	case "gcp_cloud":
		return "" // no embedded GCP logo yet
	case "google_compute_network":
		return "gcp/virtual_private_cloud/virtual_private_cloud.svg"
	case "google_compute_subnetwork":
		return "gcp/virtual_private_cloud/virtual_private_cloud.svg"
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
			fmt.Fprintf(b, `<text class="sg-text" x="%.0f" y="%.0f" text-anchor="middle" font-size="11" fill="#2D3436">%s</text>`,
				cx, labelY+float64(i)*14, escapeXMLStr(line))
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

		fmt.Fprintf(b, `<rect class="sg-node-box" x="%.0f" y="%.0f" width="%.0f" height="%.0f" rx="6" fill="%s" stroke="%s" stroke-width="1.5"/>`,
			x, y, pos.Width, pos.Height, fill, stroke)
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
		fmt.Fprintf(b, `<text class="sg-text" x="%.0f" y="%.0f" text-anchor="middle" font-size="11" font-weight="500" fill="#2D3436">%s</text>`,
			cx, cy+4, escapeXMLStr(label))
		b.WriteString("\n")
	}
}

// getIconPath maps resource types to embedded icon file paths.
func getIconPath(n *sgraph.Node) string {
	switch n.ResourceType {
	// AWS Compute
	case "aws_instance":
		return "aws/Arch_Compute/Arch_Amazon-EC2_64.png"
	case "aws_lambda_function":
		return "aws/Arch_Compute/Arch_AWS-Lambda_64.png"
	case "aws_autoscaling_group":
		return "aws/Arch_Compute/Arch_Amazon-EC2-Auto-Scaling_64.png"

	// AWS Containers
	case "aws_ecs_cluster", "aws_ecs_service", "aws_ecs_task_definition":
		return "aws/Arch_Containers/Arch_Amazon-Elastic-Container-Service_64.png"
	case "aws_eks_cluster", "aws_eks_node_group":
		return "aws/Arch_Containers/Arch_Amazon-Elastic-Kubernetes-Service_64.png"

	// AWS Networking
	case "aws_lb":
		return "aws/Arch_Networking-Content-Delivery/Arch_Elastic-Load-Balancing_64.png"
	case "aws_route53_zone", "aws_route53_record":
		return "aws/Arch_Networking-Content-Delivery/Arch_Amazon-Route-53_64.png"
	case "aws_cloudfront_distribution":
		return "aws/Arch_Networking-Content-Delivery/Arch_Amazon-CloudFront_64.png"
	case "aws_api_gateway_rest_api", "aws_apigatewayv2_api":
		return "aws/Arch_Networking-Content-Delivery/Arch_Amazon-API-Gateway_64.png"

	// AWS Storage
	case "aws_s3_bucket":
		return "aws/Arch_Storage/Arch_Amazon-Simple-Storage-Service_64.png"
	case "aws_ebs_volume":
		return "aws/Arch_Storage/Arch_Amazon-Elastic-Block-Store_64.png"

	// AWS Database
	case "aws_db_instance", "aws_rds_cluster":
		return "aws/Arch_Databases/Arch_Amazon-RDS_64.png"
	case "aws_dynamodb_table":
		return "aws/Arch_Databases/Arch_Amazon-DynamoDB_64.png"
	case "aws_elasticache_cluster", "aws_elasticache_replication_group":
		return "aws/Arch_Databases/Arch_Amazon-ElastiCache_64.png"

	// AWS Security
	case "aws_iam_role", "aws_iam_policy", "aws_iam_user", "aws_iam_instance_profile":
		return "aws/Arch_Security-Identity/Arch_AWS-Identity-and-Access-Management_64.png"
	case "aws_kms_key":
		return "aws/Arch_Security-Identity/Arch_AWS-Key-Management-Service_64.png"
	case "aws_secretsmanager_secret":
		return "aws/Arch_Security-Identity/Arch_AWS-Secrets-Manager_64.png"

	// AWS Messaging
	case "aws_sqs_queue":
		return "aws/Arch_Application-Integration/Arch_Amazon-Simple-Queue-Service_64.png"
	case "aws_sns_topic", "aws_sns_topic_subscription":
		return "aws/Arch_Application-Integration/Arch_Amazon-Simple-Notification-Service_64.png"

	// AWS Monitoring
	case "aws_cloudwatch_log_group", "aws_cloudwatch_metric_alarm":
		return "aws/Arch_Management-Tools/Arch_Amazon-CloudWatch_64.png"

	// Azure Compute
	case "azurerm_virtual_machine", "azurerm_linux_virtual_machine", "azurerm_windows_virtual_machine":
		return "azure/compute/10021-icon-service-Virtual-Machine.svg"
	case "azurerm_kubernetes_cluster":
		return "azure/containers/10023-icon-service-Kubernetes-Services.svg"

	// Azure Networking
	case "azurerm_virtual_network":
		return "azure/networking/10061-icon-service-Virtual-Networks.svg"
	case "azurerm_network_security_group":
		return "azure/networking/10067-icon-service-Network-Security-Groups.svg"
	case "azurerm_public_ip":
		return "azure/networking/10069-icon-service-Public-IP-Addresses.svg"

	// Azure Database
	case "azurerm_mssql_server", "azurerm_mssql_database":
		return "azure/databases/10130-icon-service-SQL-Database.svg"

	// Azure Storage
	case "azurerm_storage_account":
		return "azure/storage/10086-icon-service-Storage-Accounts.svg"

	// Azure General
	case "azurerm_resource_group":
		return "azure/general/10007-icon-service-Resource-Groups.svg"

	// GCP Compute
	case "google_compute_instance":
		return "gcp/compute_engine/compute_engine.svg"
	case "google_container_cluster":
		return "gcp/google_kubernetes_engine/google_kubernetes_engine.svg"
	case "google_cloudfunctions_function", "google_cloudfunctions2_function":
		return "gcp/cloud_functions/cloud_functions.svg"

	// GCP Storage/DB
	case "google_storage_bucket":
		return "gcp/cloud_storage/cloud_storage.svg"
	case "google_sql_database_instance":
		return "gcp/cloud_sql/cloud_sql.svg"

	// GCP Networking
	case "google_compute_network":
		return "gcp/virtual_private_cloud/virtual_private_cloud.svg"

	default:
		return ""
	}
}

func renderDagreEdge(b *strings.Builder, points []Point) {
	if len(points) < 2 {
		return
	}

	var d strings.Builder

	if len(points) == 2 {
		// 1-rank edge: straight line from source bottom to target top
		fmt.Fprintf(&d, "M %.1f,%.1f L %.1f,%.1f",
			points[0].X, points[0].Y, points[1].X, points[1].Y)
	} else if len(points) == 3 {
		// 2-rank edge: gentle S-curve through midpoint
		// Use a quadratic bezier with the middle point as control
		fmt.Fprintf(&d, "M %.1f,%.1f Q %.1f,%.1f %.1f,%.1f",
			points[0].X, points[0].Y,
			points[1].X, points[1].Y,
			points[2].X, points[2].Y)
	} else {
		// Multi-rank edge: cubic B-spline (basis spline)
		// This is what dagre-d3 uses with d3.curveBasis.
		// The B-spline approximates the path through the waypoints
		// without overshooting, producing smooth flowing curves.
		//
		// Algorithm: for n points, generate n-1 cubic bezier segments.
		// Each segment uses weighted averages of consecutive points
		// as control points, ensuring C2 continuity.
		fmt.Fprintf(&d, "M %.1f,%.1f", points[0].X, points[0].Y)

		// First segment: line to the first B-spline point
		bx := (points[0].X + 4*points[1].X + points[2].X) / 6
		by := (points[0].Y + 4*points[1].Y + points[2].Y) / 6
		fmt.Fprintf(&d, " L %.1f,%.1f", bx, by)

		// Middle segments: cubic bezier using basis spline formula
		for i := 1; i < len(points)-2; i++ {
			p0 := points[i]
			p1 := points[i+1]
			p2 := points[i+2]

			// Control point 1: 2/3 toward p1 from the B-spline point at i
			cp1x := (2*p0.X + p1.X) / 3
			cp1y := (2*p0.Y + p1.Y) / 3

			// Control point 2: 1/3 toward p1 from the B-spline point at i+1
			cp2x := (p0.X + 2*p1.X) / 3
			cp2y := (p0.Y + 2*p1.Y) / 3

			// End point: B-spline point at i+1
			ex := (p0.X + 4*p1.X + p2.X) / 6
			ey := (p0.Y + 4*p1.Y + p2.Y) / 6

			fmt.Fprintf(&d, " C %.1f,%.1f %.1f,%.1f %.1f,%.1f",
				cp1x, cp1y, cp2x, cp2y, ex, ey)
		}

		// Last segment: line to the final point
		last := points[len(points)-1]
		fmt.Fprintf(&d, " L %.1f,%.1f", last.X, last.Y)
	}

	fmt.Fprintf(b, `<path class="sg-edge" d="%s" fill="none" stroke="#7B8894" stroke-width="1.5" marker-end="url(#arrowhead)"/>`, d.String())
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

	// Azure (official architecture diagram colors)
	case "azurerm_resource_group":
		return "#F0F0F0", "#767676", true
	case "azurerm_virtual_network":
		return "#E7F4E4", "#50E6FF", false
	case "azurerm_subnet":
		return "#F2F2F2", "#B3B3B3", false

	// GCP (official brand color hierarchy: blue > green > yellow > red)
	case "google_compute_network":
		return "#E6F4EA", "#34A853", false
	case "google_compute_subnetwork":
		return "#F1F3F4", "#5F6368", false

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
	case "azurerm_resource_group":
		return "#767676"
	case "azurerm_virtual_network":
		return "#0078D4"
	case "azurerm_subnet":
		return "#666666"
	case "google_compute_network":
		return "#34A853"
	case "google_compute_subnetwork":
		return "#5F6368"
	default:
		return "#495057"
	}
}

func dagreContainerCSSClass(n *sgraph.Node) string {
	switch n.ResourceType {
	case "aws_cloud", "azure_cloud", "gcp_cloud":
		return "sg-container-cloud"
	case "aws_vpc", "azurerm_virtual_network", "google_compute_network":
		return "sg-container-vpc"
	case "aws_subnet", "azurerm_subnet", "google_compute_subnetwork":
		return "sg-container-subnet"
	case "aws_security_group", "azurerm_network_security_group":
		return "sg-container-sg"
	case "azurerm_resource_group":
		return "sg-container-default"
	default:
		return "sg-container-default"
	}
}

func dagreContainerLabelCSSClass(n *sgraph.Node) string {
	switch n.ResourceType {
	case "aws_cloud", "azure_cloud", "gcp_cloud":
		return "sg-label-cloud"
	case "aws_vpc", "azurerm_virtual_network", "google_compute_network":
		return "sg-label-vpc"
	case "aws_subnet", "azurerm_subnet", "google_compute_subnetwork":
		return "sg-label-subnet"
	case "aws_security_group", "azurerm_network_security_group":
		return "sg-label-sg"
	default:
		return "sg-label-default"
	}
}

func escapeXMLStr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}


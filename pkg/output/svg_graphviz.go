package output

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"

	sgraph "github.com/michielvha/stackgraph/pkg/graph"
	"github.com/michielvha/stackgraph/pkg/icons"
)

// RenderGraphvizSVG produces a Terravision-style infrastructure diagram using go-graphviz.
// Uses exact Graphviz parameters extracted from Terravision's source code.
func RenderGraphvizSVG(g *sgraph.Graph) ([]byte, error) {
	if len(g.Nodes) == 0 {
		return []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="400" height="80">` +
			`<text x="200" y="40" text-anchor="middle" font-family="sans-serif" font-size="14" fill="#6c757d">No resources found</text></svg>`), nil
	}

	// Write embedded icons to temp files (Graphviz needs absolute file paths)
	iconTempFiles := make(map[string]string)
	defer func() {
		for _, f := range iconTempFiles {
			os.Remove(f)
		}
	}()

	// Generate DOT with exact Terravision parameters
	dot := generateTerravisionDOT(g, iconTempFiles)

	// Parse and render via go-graphviz
	ctx := context.Background()
	gv, err := graphviz.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create graphviz: %w", err)
	}

	parsed, err := cgraph.ParseBytes([]byte(dot))
	if err != nil {
		return nil, fmt.Errorf("failed to parse DOT: %w", err)
	}
	defer parsed.Close()

	var buf bytes.Buffer
	if err := gv.Render(ctx, parsed, graphviz.SVG, &buf); err != nil {
		return nil, fmt.Errorf("failed to render SVG: %w", err)
	}

	return buf.Bytes(), nil
}

// generateTerravisionDOT creates DOT with exact Terravision visual parameters.
func generateTerravisionDOT(g *sgraph.Graph, iconTempFiles map[string]string) string {
	var b strings.Builder

	// === Graph-level attributes (exact Terravision Canvas._default_graph_attrs) ===
	b.WriteString("digraph infrastructure {\n")
	b.WriteString("  rankdir=TB;\n")
	b.WriteString("  splines=ortho;\n")
	b.WriteString("  overlap=false;\n")
	b.WriteString("  nodesep=3;\n")
	b.WriteString("  ranksep=5;\n")
	b.WriteString("  pad=1.5;\n")
	b.WriteString("  fontname=\"Sans-Serif\";\n")
	b.WriteString("  fontsize=30;\n")
	b.WriteString("  fontcolor=\"#2D3436\";\n")
	b.WriteString("  labelloc=t;\n")
	b.WriteString("  concentrate=false;\n")
	b.WriteString("  center=true;\n")
	b.WriteString("\n")

	// === Default node attributes (exact Terravision Canvas._default_node_attrs) ===
	b.WriteString("  node [\n")
	b.WriteString("    shape=box,\n")
	b.WriteString("    style=rounded,\n")
	b.WriteString("    fixedsize=true,\n")
	b.WriteString("    width=1.4,\n")
	b.WriteString("    height=1.4,\n")
	b.WriteString("    labelloc=b,\n")
	b.WriteString("    imagepos=c,\n")
	b.WriteString("    imagescale=true,\n")
	b.WriteString("    fontname=\"Sans-Serif\",\n")
	b.WriteString("    fontsize=14,\n")
	b.WriteString("    fontcolor=\"#2D3436\"\n")
	b.WriteString("  ];\n\n")

	// === Default edge attributes ===
	b.WriteString("  edge [\n")
	b.WriteString("    color=\"#7B8894\",\n")
	b.WriteString("    fontcolor=\"#2D3436\",\n")
	b.WriteString("    fontname=\"Sans-Serif\",\n")
	b.WriteString("    fontsize=13\n")
	b.WriteString("  ];\n\n")

	// Build parent→children index
	childMap := make(map[string][]string)
	nodeIdx := make(map[string]*sgraph.Node)
	for _, n := range g.Nodes {
		nodeIdx[n.ID] = n
		if n.Parent != "" {
			childMap[n.Parent] = append(childMap[n.Parent], n.ID)
		}
	}

	// Build set of edges that are parent→child containment (suppress these visually)
	containmentEdges := make(map[string]bool)
	for _, n := range g.Nodes {
		if n.Parent != "" {
			containmentEdges[n.ID+"->"+n.Parent] = true
			containmentEdges[n.Parent+"->"+n.ID] = true
		}
	}

	// Find root nodes
	var roots []string
	for _, n := range g.Nodes {
		if n.Parent == "" {
			roots = append(roots, n.ID)
		}
	}

	// Write nodes recursively (groups as subgraphs, resources as nodes)
	for _, rid := range roots {
		writeTvNode(&b, rid, nodeIdx, childMap, iconTempFiles, 1)
	}

	b.WriteString("\n")

	// Write edges (skip containment edges — they're shown via nesting)
	for _, e := range g.Edges {
		if containmentEdges[e.Source+"->"+e.Target] {
			continue
		}
		src := gvSanitize(e.Source)
		tgt := gvSanitize(e.Target)
		attrs := "dir=forward"
		if e.Label != "" {
			attrs += fmt.Sprintf(", xlabel=\"  %s  \"", e.Label)
		}
		b.WriteString(fmt.Sprintf("  %q -> %q [%s];\n", src, tgt, attrs))
	}

	b.WriteString("}\n")
	return b.String()
}

func writeTvNode(b *strings.Builder, id string, nodeIdx map[string]*sgraph.Node, childMap map[string][]string, iconTempFiles map[string]string, depth int) {
	n := nodeIdx[id]
	if n == nil {
		return
	}

	indent := strings.Repeat("  ", depth)
	kids := childMap[id]
	sanitized := gvSanitize(id)

	if n.Type == sgraph.NodeTypeGroup || len(kids) > 0 {
		// === Subgraph cluster (Terravision Cluster style) ===
		b.WriteString(fmt.Sprintf("%ssubgraph %q {\n", indent, "cluster_"+sanitized))

		// Container styling (Terravision-exact per resource type)
		style, pencolor, bgcolor, margin, fontcolor := tvClusterAttrs(n)
		b.WriteString(fmt.Sprintf("%s  style=%q;\n", indent, style))
		if pencolor != "" {
			b.WriteString(fmt.Sprintf("%s  pencolor=%q;\n", indent, pencolor))
		} else {
			b.WriteString(fmt.Sprintf("%s  pencolor=\"\";\n", indent))
		}
		if bgcolor != "" {
			b.WriteString(fmt.Sprintf("%s  color=%q;\n", indent, bgcolor))
		}
		b.WriteString(fmt.Sprintf("%s  margin=%q;\n", indent, margin))
		b.WriteString(fmt.Sprintf("%s  labeljust=l;\n", indent))
		b.WriteString(fmt.Sprintf("%s  fontname=\"Sans-Serif\";\n", indent))
		b.WriteString(fmt.Sprintf("%s  fontsize=14;\n", indent))

		// Label — use HTML table format like Terravision for rich labels
		label := n.Label
		if n.Service != "" {
			label = n.Service + " — " + n.Label
		}
		// HTML label with colored font matching the container's theme
		b.WriteString(fmt.Sprintf("%s  label=<<TABLE BORDER=\"0\" CELLBORDER=\"0\" CELLSPACING=\"0\"><TR><TD><FONT color=%q>%s</FONT></TD></TR></TABLE>>;\n",
			indent, fontcolor, escapeHTML(label)))

		// Invisible anchor for empty clusters
		if len(kids) == 0 {
			b.WriteString(fmt.Sprintf("%s  %q [shape=none, label=\"\", width=0.5, height=0.3, fixedsize=true];\n",
				indent, sanitized+"_anchor"))
		}

		for _, kid := range kids {
			writeTvNode(b, kid, nodeIdx, childMap, iconTempFiles, depth+1)
		}

		b.WriteString(fmt.Sprintf("%s}\n", indent))
	} else {
		// === Resource node ===
		// Try to resolve icon
		iconPath := gvGetIconPath(n)
		tmpPath := resolveIconTemp(iconPath, iconTempFiles)

		label := n.Label
		if n.Service != "" {
			label = n.Service + "\\n" + n.Label
		}
		if n.Count > 1 {
			label += fmt.Sprintf("\\n(x%d)", n.Count)
		}

		// Calculate height based on label lines (Terravision: 1.9 + 0.4 per newline)
		lineCount := strings.Count(label, "\\n")
		height := 1.9 + float64(lineCount)*0.4

		if tmpPath != "" {
			// Terravision icon-as-node: shape=none, icon is the visual, label below
			b.WriteString(fmt.Sprintf("%s%q [shape=none, label=%q, labelloc=b, image=%q, imagescale=true, fixedsize=true, width=1.4, height=%.1f];\n",
				indent, sanitized, label, tmpPath, height))
		} else {
			// Generic fallback — styled rounded box (Terravision default node style)
			style := "rounded"
			fill := ""
			color := ""
			switch n.Action {
			case sgraph.ActionCreate:
				style = "\"rounded,filled\""
				fill = "#E8F5E9"
				color = "#43A047"
			case sgraph.ActionDelete:
				style = "\"rounded,filled\""
				fill = "#FFEBEE"
				color = "#E53935"
			case sgraph.ActionUpdate:
				style = "\"rounded,filled\""
				fill = "#FFF8E1"
				color = "#FB8C00"
			}
			attrs := fmt.Sprintf("label=%q, fixedsize=true, width=1.4, height=1.4", label)
			if fill != "" {
				attrs += fmt.Sprintf(", style=%s, fillcolor=%q, color=%q", style, fill, color)
			}
			b.WriteString(fmt.Sprintf("%s%q [%s];\n", indent, sanitized, attrs))
		}
	}
}

func resolveIconTemp(iconPath string, iconTempFiles map[string]string) string {
	if iconPath == "" {
		return ""
	}
	if cached, ok := iconTempFiles[iconPath]; ok {
		return cached
	}
	tmpPath, err := icons.WriteIconToTemp(iconPath)
	if err != nil {
		return ""
	}
	iconTempFiles[iconPath] = tmpPath
	return tmpPath
}

// tvClusterAttrs returns Terravision-exact subgraph attributes per resource type.
func tvClusterAttrs(n *sgraph.Node) (style, pencolor, bgcolor, margin, fontcolor string) {
	switch n.ResourceType {
	case "aws_vpc":
		return "solid", "#8C4FFF", "", "50", "#8C4FFF"
	case "aws_subnet":
		// Public subnet: filled green, no border
		// TODO: distinguish public vs private by checking map_public_ip_on_launch attr
		return "filled", "", "#F2F7EE", "50", "#558B2F"
	case "aws_security_group":
		return "solid", "red", "", "50", "red"
	case "aws_autoscaling_group":
		return "dashed", "pink", "#DEEBF7", "50", "#C2185B"
	case "aws_ecs_cluster", "aws_eks_cluster":
		return "dashed", "#FF9900", "", "50", "#E65100"

	// Azure
	case "azurerm_resource_group":
		return "dashed", "#0078D4", "", "30", "#0078D4"
	case "azurerm_virtual_network":
		return "filled,dashed", "#0078D4", "#E8F4FC", "40", "#0078D4"
	case "azurerm_subnet":
		return "filled", "#CCCCCC", "#FFFFFF", "20", "#666666"

	// GCP
	case "google_compute_network":
		return "filled", "", "#E3F2FD", "70", "#1565C0"
	case "google_compute_subnetwork":
		return "filled", "", "#EDE7F6", "70", "#4527A0"

	default:
		return "dashed", "#ADB5BD", "", "50", "#495057"
	}
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func gvSanitize(id string) string {
	r := strings.ReplaceAll(id, ".", "_")
	r = strings.ReplaceAll(r, "[", "_")
	r = strings.ReplaceAll(r, "]", "")
	r = strings.ReplaceAll(r, "\"", "")
	return r
}

func gvGetIconPath(n *sgraph.Node) string {
	switch n.ResourceType {
	case "aws_instance":
		return "aws/compute/ec2.svg"
	case "aws_lambda_function":
		return "aws/compute/lambda.svg"
	case "aws_lb":
		return "aws/networking/alb.svg"
	case "aws_s3_bucket":
		return "aws/storage/s3.svg"
	case "aws_db_instance", "aws_rds_cluster":
		return "aws/database/rds.svg"
	case "aws_ecs_cluster", "aws_ecs_service":
		return "aws/containers/ecs.svg"
	case "aws_route53_zone", "aws_route53_record":
		return "aws/networking/route53.svg"
	case "aws_iam_role", "aws_iam_policy", "aws_iam_user":
		return "aws/security/iam.svg"
	case "aws_internet_gateway":
		return "aws/networking/igw.svg"
	case "aws_security_group":
		return "aws/security/sg.svg"
	default:
		return ""
	}
}

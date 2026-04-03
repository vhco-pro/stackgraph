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
// Strategy: generate a DOT string with full styling, then render it via go-graphviz's WASM engine.
func RenderGraphvizSVG(g *sgraph.Graph) ([]byte, error) {
	if len(g.Nodes) == 0 {
		return []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="400" height="80">` +
			`<text x="200" y="40" text-anchor="middle" font-family="sans-serif" font-size="14" fill="#6c757d">No resources found</text></svg>`), nil
	}

	// Write embedded icons to temp files (Graphviz needs file paths)
	iconTempFiles := make(map[string]string)
	defer func() {
		for _, f := range iconTempFiles {
			os.Remove(f)
		}
	}()

	// Generate DOT with full Terravision-style attributes
	dot := generateStyledDOT(g, iconTempFiles)

	// Parse and render via go-graphviz
	ctx := context.Background()
	gv, err := graphviz.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create graphviz: %w", err)
	}

	graph, err := cgraph.ParseBytes([]byte(dot))
	if err != nil {
		return nil, fmt.Errorf("failed to parse DOT: %w", err)
	}
	defer graph.Close()

	var buf bytes.Buffer
	if err := gv.Render(ctx, graph, graphviz.SVG, &buf); err != nil {
		return nil, fmt.Errorf("failed to render SVG: %w", err)
	}

	return buf.Bytes(), nil
}

// generateStyledDOT creates a DOT string with Terravision-style visual parameters.
func generateStyledDOT(g *sgraph.Graph, iconTempFiles map[string]string) string {
	var b strings.Builder

	b.WriteString("digraph infrastructure {\n")
	b.WriteString("  rankdir=TB;\n")
	b.WriteString("  compound=true;\n")
	b.WriteString("  nodesep=1.0;\n")
	b.WriteString("  ranksep=1.5;\n")
	b.WriteString("  pad=0.5;\n")
	b.WriteString("  fontname=\"Sans-Serif\";\n")
	b.WriteString("  node [fontname=\"Sans-Serif\", fontsize=11, fontcolor=\"#2D3436\"];\n")
	b.WriteString("  edge [color=\"#7B8894\", fontname=\"Sans-Serif\", fontsize=9, arrowhead=normal];\n\n")

	// Build parent→children index
	childMap := make(map[string][]string)
	nodeIdx := make(map[string]*sgraph.Node)
	for _, n := range g.Nodes {
		nodeIdx[n.ID] = n
		if n.Parent != "" {
			childMap[n.Parent] = append(childMap[n.Parent], n.ID)
		}
	}

	// Track which nodes were written inside subgraphs
	written := make(map[string]bool)

	// Find root nodes
	var roots []string
	for _, n := range g.Nodes {
		if n.Parent == "" {
			roots = append(roots, n.ID)
		}
	}

	// Write nodes recursively
	for _, rid := range roots {
		writeGvNode(&b, rid, nodeIdx, childMap, written, iconTempFiles, 1)
	}

	b.WriteString("\n")

	// Write edges
	for _, e := range g.Edges {
		src := gvSanitize(e.Source)
		tgt := gvSanitize(e.Target)
		attrs := ""
		if e.Label != "" {
			attrs = fmt.Sprintf(" [xlabel=%q]", e.Label)
		}
		b.WriteString(fmt.Sprintf("  %q -> %q%s;\n", src, tgt, attrs))
	}

	b.WriteString("}\n")
	return b.String()
}

func writeGvNode(b *strings.Builder, id string, nodeIdx map[string]*sgraph.Node, childMap map[string][]string, written map[string]bool, iconTempFiles map[string]string, depth int) {
	n := nodeIdx[id]
	if n == nil {
		return
	}

	indent := strings.Repeat("  ", depth)
	kids := childMap[id]
	sanitized := gvSanitize(id)

	if n.Type == sgraph.NodeTypeGroup || len(kids) > 0 {
		// Subgraph cluster
		b.WriteString(fmt.Sprintf("%ssubgraph %q {\n", indent, "cluster_"+sanitized))

		// Label
		label := n.Label
		if n.Service != "" {
			label = n.Service + "\\n" + n.Label
		}
		b.WriteString(fmt.Sprintf("%s  label=%q;\n", indent, label))
		b.WriteString(fmt.Sprintf("%s  labeljust=l;\n", indent))
		b.WriteString(fmt.Sprintf("%s  fontname=\"Sans-Serif\";\n", indent))
		b.WriteString(fmt.Sprintf("%s  fontsize=13;\n", indent))
		b.WriteString(fmt.Sprintf("%s  penwidth=2;\n", indent))
		b.WriteString(fmt.Sprintf("%s  margin=16;\n", indent))

		// Container styling
		fill, stroke, style, fontcolor := gvClusterStyle(n)
		b.WriteString(fmt.Sprintf("%s  style=%q;\n", indent, style))
		b.WriteString(fmt.Sprintf("%s  color=%q;\n", indent, stroke))
		b.WriteString(fmt.Sprintf("%s  bgcolor=%q;\n", indent, fill))
		b.WriteString(fmt.Sprintf("%s  fontcolor=%q;\n", indent, fontcolor))

		written[id] = true

		// If no children, add an invisible anchor node so the cluster renders
		if len(kids) == 0 {
			b.WriteString(fmt.Sprintf("%s  %q [shape=none, label=\"\", width=0.5, height=0.3];\n", indent, sanitized+"_anchor"))
		}

		// Recurse into children
		for _, kid := range kids {
			writeGvNode(b, kid, nodeIdx, childMap, written, iconTempFiles, depth+1)
		}

		b.WriteString(fmt.Sprintf("%s}\n", indent))
	} else {
		// Leaf resource node
		written[id] = true

		// Try to get an icon
		iconPath := gvGetIconPath(n)
		tmpPath := ""
		if iconPath != "" {
			if cached, ok := iconTempFiles[iconPath]; ok {
				tmpPath = cached
			} else {
				var err error
				tmpPath, err = icons.WriteIconToTemp(iconPath)
				if err == nil {
					iconTempFiles[iconPath] = tmpPath
				} else {
					tmpPath = ""
				}
			}
		}

		label := n.Label
		if n.Service != "" {
			label = n.Service + "\\n" + n.Label
		}
		if n.Count > 1 {
			label += fmt.Sprintf("\\n(x%d)", n.Count)
		}

		if tmpPath != "" {
			// Icon-as-node: shape=none, image is the node visual
			b.WriteString(fmt.Sprintf("%s%q [shape=none, label=%q, labelloc=b, image=%q, imagescale=true, fixedsize=true, width=1.2, height=1.2];\n",
				indent, sanitized, label, tmpPath))
		} else {
			// Generic fallback — styled box
			style := "\"rounded,filled\""
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
			b.WriteString(fmt.Sprintf("%s%q [shape=box, style=%s, fillcolor=%q, color=%q, penwidth=1.5, label=%q];\n",
				indent, sanitized, style, fill, stroke, label))
		}
	}
}

func gvSanitize(id string) string {
	r := strings.ReplaceAll(id, ".", "_")
	r = strings.ReplaceAll(r, "[", "_")
	r = strings.ReplaceAll(r, "]", "")
	r = strings.ReplaceAll(r, "\"", "")
	return r
}

func gvClusterStyle(n *sgraph.Node) (fill, stroke, style, fontcolor string) {
	switch n.ResourceType {
	case "aws_vpc":
		return "#F8F4FF", "#8C4FFF", "filled", "#6B21A8"
	case "aws_subnet":
		return "#F2F7EE", "#7CB342", "filled", "#558B2F"
	case "aws_security_group":
		return "#FFF5F5", "#E53935", "filled,dashed", "#C62828"
	case "aws_ecs_cluster", "aws_eks_cluster":
		return "#FFF8E1", "#FF9900", "filled,dashed", "#E65100"
	case "azurerm_resource_group":
		return "#F0F8FF", "#0078D4", "filled,dashed", "#0078D4"
	case "azurerm_virtual_network":
		return "#E8F4FC", "#0078D4", "filled", "#0078D4"
	case "azurerm_subnet":
		return "#FFFFFF", "#CCCCCC", "filled", "#666666"
	case "google_compute_network":
		return "#E3F2FD", "#4285F4", "filled", "#1565C0"
	case "google_compute_subnetwork":
		return "#EDE7F6", "#7C4DFF", "filled", "#4527A0"
	default:
		return "#F8F9FA", "#ADB5BD", "filled,dashed", "#495057"
	}
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

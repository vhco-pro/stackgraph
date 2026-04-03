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
func RenderGraphvizSVG(g *sgraph.Graph) ([]byte, error) {
	if len(g.Nodes) == 0 {
		return []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="400" height="80">` +
			`<text x="200" y="40" text-anchor="middle" font-family="sans-serif" font-size="14" fill="#6c757d">No resources found</text></svg>`), nil
	}

	ctx := context.Background()
	gv, err := graphviz.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create graphviz: %w", err)
	}

	graph, err := gv.Graph()
	if err != nil {
		return nil, fmt.Errorf("failed to create graph: %w", err)
	}
	defer graph.Close()

	// Global graph attributes (Terravision-style)
	graph.SetRankDir(cgraph.TBRank)
	graph.Set("nodesep", "1.5")
	graph.Set("ranksep", "2.0")
	graph.Set("pad", "1.0")
	graph.Set("splines", "ortho")

	// Default node/edge attributes via graph-level Attr
	graph.Attr(int(cgraph.NODE), "fontname", "Sans-Serif")
	graph.Attr(int(cgraph.NODE), "fontsize", "12")
	graph.Attr(int(cgraph.NODE), "fontcolor", "#2D3436")
	graph.Attr(int(cgraph.EDGE), "color", "#7B8894")
	graph.Attr(int(cgraph.EDGE), "fontname", "Sans-Serif")
	graph.Attr(int(cgraph.EDGE), "fontsize", "10")

	// Build parent→children index
	childMap := make(map[string][]string)
	nodeIdx := make(map[string]*sgraph.Node)
	for _, n := range g.Nodes {
		nodeIdx[n.ID] = n
		if n.Parent != "" {
			childMap[n.Parent] = append(childMap[n.Parent], n.ID)
		}
	}

	// Write temp icon files (go-graphviz needs file paths)
	iconTempFiles := make(map[string]string)
	defer func() {
		for _, f := range iconTempFiles {
			os.Remove(f)
		}
	}()

	// Track graphviz nodes for edge creation
	gvNodes := make(map[string]*cgraph.Node)

	// Find root nodes
	var roots []string
	for _, n := range g.Nodes {
		if n.Parent == "" {
			roots = append(roots, n.ID)
		}
	}

	// Create nodes recursively
	for _, rid := range roots {
		createGvNodes(graph, graph, rid, nodeIdx, childMap, gvNodes, iconTempFiles)
	}

	// Create edges
	for _, e := range g.Edges {
		srcNode := gvNodes[e.Source]
		tgtNode := gvNodes[e.Target]
		if srcNode == nil || tgtNode == nil {
			continue
		}
		edge, edgeErr := graph.CreateEdgeByName(e.Source+"->"+e.Target, srcNode, tgtNode)
		if edgeErr != nil {
			continue
		}
		edge.SetColor("#7B8894")
		if e.Label != "" {
			edge.SafeSet("xlabel", e.Label, "")
		}
	}

	// Render to SVG
	var buf bytes.Buffer
	if err := gv.Render(ctx, graph, graphviz.SVG, &buf); err != nil {
		return nil, fmt.Errorf("failed to render graphviz SVG: %w", err)
	}

	return buf.Bytes(), nil
}

func createGvNodes(
	root *cgraph.Graph,
	parent *cgraph.Graph,
	id string,
	nodeIdx map[string]*sgraph.Node,
	childMap map[string][]string,
	gvNodes map[string]*cgraph.Node,
	iconTempFiles map[string]string,
) {
	n := nodeIdx[id]
	if n == nil {
		return
	}

	kids := childMap[id]

	if n.Type == sgraph.NodeTypeGroup || len(kids) > 0 {
		// Create subgraph cluster
		clusterName := "cluster_" + gvSanitize(id)
		sub, err := parent.CreateSubGraphByName(clusterName)
		if err != nil {
			return
		}

		// Container label
		label := n.Label
		if n.Service != "" {
			label = n.Service + "\\n" + n.Label
		}
		sub.Set("label", label)
		sub.Set("labeljust", "l")
		sub.Set("fontname", "Sans-Serif")
		sub.Set("fontsize", "14")
		sub.Set("margin", "20")

		// Container styling
		fill, stroke, style, fontcolor := gvClusterStyle(n)
		sub.Set("style", style)
		sub.Set("color", stroke)
		sub.Set("bgcolor", fill)
		sub.Set("fontcolor", fontcolor)
		sub.Set("penwidth", "2")

		// Recurse into children
		for _, kid := range kids {
			createGvNodes(root, sub, kid, nodeIdx, childMap, gvNodes, iconTempFiles)
		}
	} else {
		// Leaf resource node
		gvNode, err := parent.CreateNodeByName(gvSanitize(id))
		if err != nil {
			return
		}
		gvNodes[id] = gvNode

		// Try to get an icon
		iconPath := gvGetIconPath(n)
		tmpPath := ""
		if iconPath != "" {
			if cached, ok := iconTempFiles[iconPath]; ok {
				tmpPath = cached
			} else {
				tmpPath, err = icons.WriteIconToTemp(iconPath)
				if err == nil {
					iconTempFiles[iconPath] = tmpPath
				} else {
					tmpPath = ""
				}
			}
		}

		if tmpPath != "" {
			// Terravision-style: icon IS the node
			gvNode.SetShape("none")
			gvNode.SetImage(tmpPath)
			gvNode.SetImageScale(cgraph.ImageScaleTrue)
			gvNode.SetFixedSize(true)
			gvNode.SetWidth(1.4)
			gvNode.SetHeight(1.4)
		} else {
			// Generic fallback — styled box
			gvNode.SetShape(cgraph.BoxShape)
			gvNode.SetStyle(cgraph.RoundedNodeStyle)
			gvNode.SetFillColor("#FFFFFF")
			gvNode.SetColor("#DEE2E6")
			gvNode.SafeSet("penwidth", "1.5", "")
		}

		// Label below icon
		label := n.Label
		if n.Service != "" {
			label = n.Service + "\\n" + n.Label
		}
		if n.Count > 1 {
			label += fmt.Sprintf("\\n(x%d)", n.Count)
		}
		gvNode.SetLabel(label)
		gvNode.SafeSet("labelloc", "b", "")
		gvNode.SetFontName("Sans-Serif")
		gvNode.SetFontSize(12)
		gvNode.SetFontColor("#2D3436")
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
		return "#F8F4FF", "#8C4FFF", "solid", "#6B21A8"
	case "aws_subnet":
		return "#F2F7EE", "#7CB342", "solid", "#558B2F"
	case "aws_security_group":
		return "#FFF5F5", "#E53935", "dashed", "#C62828"
	case "aws_ecs_cluster", "aws_eks_cluster":
		return "#FFF8E1", "#FF9900", "dashed", "#E65100"
	case "aws_autoscaling_group":
		return "#DEEBF7", "#FF69B4", "dashed", "#C2185B"
	case "azurerm_resource_group":
		return "#F0F8FF", "#0078D4", "dashed", "#0078D4"
	case "azurerm_virtual_network":
		return "#E8F4FC", "#0078D4", "solid", "#0078D4"
	case "azurerm_subnet":
		return "#FFFFFF", "#CCCCCC", "solid", "#666666"
	case "google_compute_network":
		return "#E3F2FD", "#4285F4", "solid", "#1565C0"
	case "google_compute_subnetwork":
		return "#EDE7F6", "#7C4DFF", "solid", "#4527A0"
	default:
		return "#F8F9FA", "#ADB5BD", "dashed", "#495057"
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

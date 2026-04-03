package output

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/michielvha/stackgraph/pkg/graph"
	"github.com/michielvha/stackgraph/pkg/icons"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
	"oss.terrastruct.com/util-go/go2"
)

// RenderSVG produces an infrastructure diagram as SVG using D2.
func RenderSVG(g *graph.Graph) ([]byte, error) {
	if len(g.Nodes) == 0 {
		return []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="400" height="80">` +
			`<text x="200" y="40" text-anchor="middle" font-family="sans-serif" font-size="14" fill="#6c757d">No resources found</text></svg>`), nil
	}

	// Write embedded icons to temp files for D2 to read
	iconTempFiles := make(map[string]string)
	defer func() {
		for _, f := range iconTempFiles {
			os.Remove(f)
		}
	}()

	script := graphToD2(g, iconTempFiles)

	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return nil, fmt.Errorf("failed to create text ruler: %w", err)
	}

	layoutResolver := func(engine string) (d2graph.LayoutGraph, error) {
		return d2dagrelayout.DefaultLayout, nil
	}

	renderOpts := &d2svg.RenderOpts{
		Pad:     go2.Pointer(int64(20)),
		ThemeID: &d2themescatalog.CoolClassics.ID,
	}

	compileOpts := &d2lib.CompileOptions{
		LayoutResolver: layoutResolver,
		Ruler:          ruler,
	}

	ctx := log.WithDefault(context.Background())
	diagram, _, err := d2lib.Compile(ctx, script, compileOpts, renderOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to compile D2 diagram: %w", err)
	}

	svgBytes, err := d2svg.Render(diagram, renderOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to render SVG: %w", err)
	}

	return svgBytes, nil
}

// graphToD2 converts the internal graph representation to a D2 script string.
// iconTempFiles maps icon paths to temp file absolute paths for local embedding.
func graphToD2(g *graph.Graph, iconTempFiles map[string]string) string {
	var b strings.Builder

	// Build parent→children index
	childMap := make(map[string][]string)
	nodeIdx := make(map[string]*graph.Node)
	for _, n := range g.Nodes {
		nodeIdx[n.ID] = n
		if n.Parent != "" {
			childMap[n.Parent] = append(childMap[n.Parent], n.ID)
		}
	}

	// Build D2 path map: original node ID -> full D2 container path
	// e.g., "aws_instance.web_a" (parent: aws_subnet.public, grandparent: aws_vpc.main)
	// -> "aws_vpc_main.aws_subnet_public.aws_instance_web_a"
	d2Paths := make(map[string]string)
	var buildPath func(id string) string
	buildPath = func(id string) string {
		if cached, ok := d2Paths[id]; ok {
			return cached
		}
		safeID := d2SafeID(id)
		n := nodeIdx[id]
		if n == nil || n.Parent == "" {
			d2Paths[id] = safeID
			return safeID
		}
		parentPath := buildPath(n.Parent)
		fullPath := parentPath + "." + safeID
		d2Paths[id] = fullPath
		return fullPath
	}
	for _, n := range g.Nodes {
		buildPath(n.ID)
	}

	// Find root nodes
	var roots []string
	for _, n := range g.Nodes {
		if n.Parent == "" {
			roots = append(roots, n.ID)
		}
	}

	// Render nodes recursively
	for _, rid := range roots {
		writeD2Node(&b, rid, nodeIdx, childMap, 0, iconTempFiles)
	}

	b.WriteString("\n")

	// Render edges using full D2 container paths (prevents duplicate nodes)
	for _, e := range g.Edges {
		srcPath, srcOK := d2Paths[e.Source]
		tgtPath, tgtOK := d2Paths[e.Target]
		if !srcOK || !tgtOK {
			continue
		}
		if e.Label != "" {
			b.WriteString(fmt.Sprintf("%s -> %s: %s\n", srcPath, tgtPath, e.Label))
		} else {
			b.WriteString(fmt.Sprintf("%s -> %s\n", srcPath, tgtPath))
		}
	}

	return b.String()
}

// d2SafeID converts a resource address to a D2-safe identifier (no dots — D2 uses dots as hierarchy separators).
func d2SafeID(id string) string {
	safe := strings.ReplaceAll(id, ".", "_")
	safe = strings.ReplaceAll(safe, "[", "_")
	safe = strings.ReplaceAll(safe, "]", "")
	return safe
}

func writeD2Node(b *strings.Builder, id string, nodeIdx map[string]*graph.Node, childMap map[string][]string, depth int, iconTempFiles map[string]string) {
	n := nodeIdx[id]
	if n == nil {
		return
	}

	indent := strings.Repeat("  ", depth)
	d2id := d2SafeID(id)
	label := nodeLabel(n)

	kids := childMap[id]

	if n.Type == graph.NodeTypeGroup || len(kids) > 0 {
		// Container node
		b.WriteString(fmt.Sprintf("%s%s: %s {\n", indent, d2id, label))

		// Style for groups
		b.WriteString(fmt.Sprintf("%s  style.border-radius: 8\n", indent))

		fill, stroke := d2GroupColors(n)
		if fill != "" {
			b.WriteString(fmt.Sprintf("%s  style.fill: \"%s\"\n", indent, fill))
		}
		if stroke != "" {
			b.WriteString(fmt.Sprintf("%s  style.stroke: \"%s\"\n", indent, stroke))
		}

		// Icon
		icon := resolveD2Icon(n, iconTempFiles)
		if icon != "" {
			b.WriteString(fmt.Sprintf("%s  icon: %s\n", indent, icon))
		}

		b.WriteString("\n")
		for _, kid := range kids {
			writeD2Node(b, kid, nodeIdx, childMap, depth+1, iconTempFiles)
		}
		b.WriteString(fmt.Sprintf("%s}\n", indent))
	} else {
		// Leaf resource node
		b.WriteString(fmt.Sprintf("%s%s: %s", indent, d2id, label))

		icon := resolveD2Icon(n, iconTempFiles)
		attrs := d2NodeAttrs(n, icon)
		if attrs != "" {
			b.WriteString(fmt.Sprintf(" {\n%s%s%s}\n", indent, attrs, indent))
		} else {
			b.WriteString("\n")
		}
	}
}

func nodeLabel(n *graph.Node) string {
	label := n.Label
	if n.Service != "" {
		label = n.Service + " — " + n.Label
	}
	if n.Count > 1 {
		label += fmt.Sprintf(" (x%d)", n.Count)
	}
	return label
}

// d2ID converts a resource address to a valid D2 identifier.
func d2ID(id string) string {
	// D2 identifiers with dots/brackets need quoting
	if strings.ContainsAny(id, ".[]\"/ ") {
		return `"` + strings.ReplaceAll(id, `"`, `\"`) + `"`
	}
	return id
}

// resolveD2Icon returns the absolute path to a temp icon file for D2 to embed,
// or empty string if no icon is available. Uses the same icon mapping as the Graphviz renderer.
func resolveD2Icon(n *graph.Node, iconTempFiles map[string]string) string {
	iconPath := d2IconPath(n)
	if iconPath == "" {
		return ""
	}

	// Check if already written to temp
	if tmpPath, ok := iconTempFiles[iconPath]; ok {
		return tmpPath
	}

	// Write embedded icon to temp file
	tmpPath, err := icons.WriteIconToTemp(iconPath)
	if err != nil {
		return ""
	}
	iconTempFiles[iconPath] = tmpPath
	return tmpPath
}

// d2IconPath maps resource types to embedded icon paths (shared with Graphviz renderer).
func d2IconPath(n *graph.Node) string {
	switch n.ResourceType {
	case "aws_instance":
		return "aws/compute/ec2.svg"
	case "aws_lambda_function":
		return "aws/compute/lambda.svg"
	case "aws_vpc":
		return "aws/networking/vpc.svg"
	case "aws_subnet":
		return "aws/networking/subnet.svg"
	case "aws_security_group":
		return "aws/security/sg.svg"
	case "aws_lb":
		return "aws/networking/alb.svg"
	case "aws_internet_gateway":
		return "aws/networking/igw.svg"
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
	default:
		return ""
	}
}

func d2GroupColors(n *graph.Node) (fill, stroke string) {
	switch n.ResourceType {
	case "aws_vpc":
		return "#F8F4FF", "#8C4FFF"
	case "aws_subnet":
		return "#F2F7EE", "#7CB342"
	case "aws_security_group":
		return "#FFF5F5", "#E53935"
	case "azurerm_resource_group":
		return "#F0F8FF", "#0078D4"
	case "azurerm_virtual_network":
		return "#E8F4FC", "#0078D4"
	case "google_compute_network":
		return "#E3F2FD", "#4285F4"
	case "google_compute_subnetwork":
		return "#EDE7F6", "#7C4DFF"
	default:
		return "#F8F9FA", "#ADB5BD"
	}
}

func d2NodeAttrs(n *graph.Node, icon string) string {
	var parts []string

	if icon != "" {
		parts = append(parts, fmt.Sprintf("  icon: %s\n", icon))
	}

	// Shape based on type
	if n.Type == graph.NodeTypeData {
		parts = append(parts, "  shape: cylinder\n")
	}

	// Action styling
	switch n.Action {
	case graph.ActionCreate:
		parts = append(parts, "  style.fill: \"#E8F5E9\"\n  style.stroke: \"#43A047\"\n")
	case graph.ActionDelete:
		parts = append(parts, "  style.fill: \"#FFEBEE\"\n  style.stroke: \"#E53935\"\n")
	case graph.ActionUpdate:
		parts = append(parts, "  style.fill: \"#FFF8E1\"\n  style.stroke: \"#FB8C00\"\n")
	}

	return strings.Join(parts, "")
}

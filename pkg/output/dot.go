package output

import (
	"fmt"
	"strings"

	"github.com/michielvha/stackgraph/pkg/graph"
)

// RenderDOT converts the graph to Graphviz DOT format.
func RenderDOT(g *graph.Graph) ([]byte, error) {
	var b strings.Builder

	b.WriteString("digraph infrastructure {\n")
	b.WriteString("  rankdir=TB;\n")
	b.WriteString("  node [shape=box, style=rounded, fontname=\"Helvetica\"];\n")
	b.WriteString("  edge [fontname=\"Helvetica\", fontsize=10];\n\n")

	// Write group subgraphs
	groups := make(map[string]*graph.Node)
	for _, n := range g.Nodes {
		if n.Type == graph.NodeTypeGroup {
			groups[n.ID] = n
		}
	}

	// Track which nodes are written inside subgraphs
	written := make(map[string]bool)

	// Write group subgraphs (only top-level groups)
	for _, gn := range groups {
		if gn.Parent == "" {
			writeSubgraph(&b, gn, g, groups, written, 1)
		}
	}

	// Write remaining non-grouped nodes
	for _, n := range g.Nodes {
		if written[n.ID] {
			continue
		}
		writeNode(&b, n, 1)
	}

	b.WriteString("\n")

	// Write edges
	for _, e := range g.Edges {
		label := ""
		if e.Label != "" {
			label = fmt.Sprintf(" [label=%q]", e.Label)
		}
		fmt.Fprintf(&b, "  %q -> %q%s;\n", e.Source, e.Target, label)
	}

	b.WriteString("}\n")

	return []byte(b.String()), nil
}

func writeSubgraph(b *strings.Builder, gn *graph.Node, g *graph.Graph, groups map[string]*graph.Node, written map[string]bool, depth int) {
	indent := strings.Repeat("  ", depth)
	// Graphviz requires subgraph names to start with "cluster_"
	fmt.Fprintf(b, "%ssubgraph \"cluster_%s\" {\n", indent, gn.ID)
	fmt.Fprintf(b, "%s  label=%q;\n", indent, dotNodeLabel(gn))
	fmt.Fprintf(b, "%s  style=dashed;\n", indent)
	fmt.Fprintf(b, "%s  color=gray;\n", indent)

	written[gn.ID] = true

	for _, childID := range gn.Children {
		child := g.NodeByID(childID)
		if child == nil {
			continue
		}
		if childGroup, isGroup := groups[childID]; isGroup {
			writeSubgraph(b, childGroup, g, groups, written, depth+1)
		} else {
			writeNode(b, child, depth+1)
			written[childID] = true
		}
	}

	fmt.Fprintf(b, "%s}\n", indent)
}

func writeNode(b *strings.Builder, n *graph.Node, depth int) {
	indent := strings.Repeat("  ", depth)
	label := dotNodeLabel(n)
	shape := "box"
	if n.Type == graph.NodeTypeData {
		shape = "ellipse"
	}
	fmt.Fprintf(b, "%s%q [label=%q, shape=%s];\n", indent, n.ID, label, shape)
}

func dotNodeLabel(n *graph.Node) string {
	label := n.Label
	if n.Service != "" {
		label = n.Service + "\\n" + n.Label
	}
	if n.Count > 1 {
		label += fmt.Sprintf(" x%d", n.Count)
	}
	return label
}

package parser

import (
	"fmt"
	"strings"

	"github.com/awalterschulze/gographviz"
	"github.com/michielvha/stackgraph/pkg/graph"
)

// ParseDOT parses a DOT format graph (e.g., from `tofu graph`) and returns a graph.
func ParseDOT(data []byte) (*graph.Graph, error) {
	graphAst, err := gographviz.ParseString(string(data))
	if err != nil {
		return nil, fmt.Errorf("invalid DOT format: %w", err)
	}

	parsed := gographviz.NewGraph()
	if err := gographviz.Analyse(graphAst, parsed); err != nil {
		return nil, fmt.Errorf("failed to analyze DOT graph: %w", err)
	}

	g := graph.NewGraph("dot")

	// Add nodes
	for _, node := range parsed.Nodes.Nodes {
		name := cleanDOTName(node.Name)
		if name == "" {
			continue
		}

		resourceType, resourceName := parseDOTNodeName(name)
		provider := extractProviderFromType(resourceType)

		g.AddNode(&graph.Node{
			ID:           name,
			Type:         graph.NodeTypeResource,
			ResourceType: resourceType,
			Label:        resourceName,
			Provider:     provider,
		})
	}

	// Add edges
	for srcName, dstMap := range parsed.Edges.SrcToDsts {
		src := cleanDOTName(srcName)
		for dstName, edges := range dstMap {
			dst := cleanDOTName(dstName)
			if src == "" || dst == "" {
				continue
			}
			for range edges {
				g.AddEdge(&graph.Edge{
					Source: src,
					Target: dst,
					Type:   "dependency",
				})
			}
		}
	}

	return g, nil
}

// cleanDOTName removes DOT quoting and terraform graph prefixes.
func cleanDOTName(name string) string {
	// Remove quotes
	name = strings.Trim(name, "\"")

	// Remove [root] prefix from terraform graph output
	name = strings.TrimPrefix(name, "[root] ")

	// Remove (expand) and (close) suffixes
	if idx := strings.Index(name, " (expand)"); idx != -1 {
		name = name[:idx]
	}
	if idx := strings.Index(name, " (close)"); idx != -1 {
		name = name[:idx]
	}

	return strings.TrimSpace(name)
}

// parseDOTNodeName extracts the resource type and name from a DOT node name.
// e.g., "aws_instance.web" -> ("aws_instance", "web")
func parseDOTNodeName(name string) (string, string) {
	// Handle module prefix: module.foo.aws_instance.web
	parts := strings.Split(name, ".")
	if len(parts) >= 2 {
		// Find the resource type.name boundary
		// Resource types contain underscores, not dots
		for i := len(parts) - 2; i >= 0; i-- {
			if strings.Contains(parts[i], "_") || parts[i] == "data" {
				resourceType := parts[i]
				resourceName := parts[i+1]
				if parts[i] == "data" && i+2 < len(parts) {
					resourceType = parts[i+1]
					resourceName = parts[i+2]
				}
				return resourceType, resourceName
			}
		}
	}

	return name, name
}

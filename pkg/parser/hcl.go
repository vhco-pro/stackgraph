package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/michielvha/stackgraph/pkg/graph"
)

// ParseHCLDir parses all .tf files in a directory and returns a graph based on
// resource definitions and their cross-references.
func ParseHCLDir(dir string) (*graph.Graph, error) {
	g := graph.NewGraph("hcl")

	files, err := filepath.Glob(filepath.Join(dir, "*.tf"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob .tf files: %w", err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no .tf files found in %s", dir)
	}

	// Parse all files and collect resource blocks
	type resourceBlock struct {
		Type string
		Name string
		Body *hclsyntax.Body
	}
	var resources []resourceBlock

	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", file, err)
		}

		hclFile, diags := hclsyntax.ParseConfig(src, file, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return nil, fmt.Errorf("failed to parse %s: %w", file, diags)
		}

		body, ok := hclFile.Body.(*hclsyntax.Body)
		if !ok {
			continue
		}

		for _, block := range body.Blocks {
			switch block.Type {
			case "resource":
				if len(block.Labels) >= 2 {
					resources = append(resources, resourceBlock{
						Type: block.Labels[0],
						Name: block.Labels[1],
						Body: block.Body,
					})
				}
			case "data":
				if len(block.Labels) >= 2 {
					address := "data." + block.Labels[0] + "." + block.Labels[1]
					provider := extractProviderFromType(block.Labels[0])
					g.AddNode(&graph.Node{
						ID:           address,
						Type:         graph.NodeTypeData,
						ResourceType: block.Labels[0],
						Label:        block.Labels[1],
						Provider:     provider,
					})
				}
			case "module":
				if len(block.Labels) >= 1 {
					g.AddNode(&graph.Node{
						ID:           "module." + block.Labels[0],
						Type:         graph.NodeTypeResource,
						ResourceType: "module",
						Label:        block.Labels[0],
						Provider:     "terraform",
					})
				}
			}
		}
	}

	// Add resource nodes
	for _, r := range resources {
		address := r.Type + "." + r.Name
		provider := extractProviderFromType(r.Type)

		g.AddNode(&graph.Node{
			ID:           address,
			Type:         graph.NodeTypeResource,
			ResourceType: r.Type,
			Label:        r.Name,
			Provider:     provider,
		})
	}

	// Detect cross-references via expression variables
	for _, r := range resources {
		srcAddress := r.Type + "." + r.Name

		for _, attr := range r.Body.Attributes {
			vars := attr.Expr.Variables()
			for _, traversal := range vars {
				ref := buildReference(traversal)
				if ref != "" && ref != srcAddress {
					// Only add edge if target exists in graph
					if g.NodeByID(ref) != nil {
						g.AddEdge(&graph.Edge{
							Source: srcAddress,
							Target: ref,
							Type:   attr.Name,
							Label:  strings.TrimSuffix(strings.ReplaceAll(attr.Name, "_", " "), " id"),
						})
					}
				}
			}
		}
	}

	return g, nil
}

// buildReference extracts a resource address from an HCL traversal.
// e.g., aws_vpc.main.id -> "aws_vpc.main"
func buildReference(traversal hcl.Traversal) string {
	if len(traversal) < 2 {
		return ""
	}

	root := traversal[0].(hcl.TraverseRoot)
	rootName := root.Name

	// Skip var, local, each, count, path, terraform references
	switch rootName {
	case "var", "local", "each", "count", "path", "terraform", "self", "null":
		return ""
	}

	// For "data.type.name" references
	if rootName == "data" && len(traversal) >= 3 {
		if attr1, ok := traversal[1].(hcl.TraverseAttr); ok {
			if attr2, ok := traversal[2].(hcl.TraverseAttr); ok {
				return "data." + attr1.Name + "." + attr2.Name
			}
		}
		return ""
	}

	// For "module.name" references
	if rootName == "module" && len(traversal) >= 2 {
		if attr, ok := traversal[1].(hcl.TraverseAttr); ok {
			return "module." + attr.Name
		}
		return ""
	}

	// For "resource_type.name" references (e.g., aws_vpc.main)
	if len(traversal) >= 2 {
		if attr, ok := traversal[1].(hcl.TraverseAttr); ok {
			return rootName + "." + attr.Name
		}
	}

	return ""
}

// extractProviderFromType gets the provider prefix from a resource type.
// e.g., "aws_instance" -> "aws", "google_compute_instance" -> "google"
func extractProviderFromType(resourceType string) string {
	parts := strings.SplitN(resourceType, "_", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return resourceType
}

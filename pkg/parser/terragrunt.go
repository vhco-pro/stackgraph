package parser

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/michielvha/stackgraph/pkg/graph"
)

// ParseTerragruntDir discovers terragrunt.hcl files in a directory tree and
// produces a module-level dependency graph.
func ParseTerragruntDir(dir string) (*graph.Graph, error) {
	g := graph.NewGraph("terragrunt")

	// Find all terragrunt.hcl files
	var tgFiles []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == "terragrunt.hcl" {
			tgFiles = append(tgFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	if len(tgFiles) == 0 {
		return nil, fmt.Errorf("no terragrunt.hcl files found in %s", dir)
	}

	// Parse each file and extract dependency blocks
	for _, tgFile := range tgFiles {
		relPath, _ := filepath.Rel(dir, filepath.Dir(tgFile))
		moduleID := "module." + relPath

		g.AddNode(&graph.Node{
			ID:           moduleID,
			Type:         graph.NodeTypeResource,
			ResourceType: "terragrunt_module",
			Label:        relPath,
			Provider:     "terragrunt",
		})

		deps, err := parseTerragruntDeps(tgFile)
		if err != nil {
			continue // skip files with parse errors, don't fail the whole graph
		}

		for _, dep := range deps {
			// Resolve the dependency path relative to the project root
			depAbsPath := filepath.Join(filepath.Dir(tgFile), dep)
			depRelPath, err := filepath.Rel(dir, depAbsPath)
			if err != nil {
				continue
			}
			depModuleID := "module." + depRelPath

			g.AddEdge(&graph.Edge{
				Source: moduleID,
				Target: depModuleID,
				Type:   "dependency",
				Label:  "depends on",
			})
		}
	}

	return g, nil
}

// parseTerragruntDeps extracts dependency config_path values from a terragrunt.hcl file.
func parseTerragruntDeps(path string) ([]string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	file, diags := hclsyntax.ParseConfig(src, path, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse %s: %w", path, diags)
	}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, nil
	}

	var deps []string

	for _, block := range body.Blocks {
		if block.Type == "dependency" {
			if attr, exists := block.Body.Attributes["config_path"]; exists {
				val, diags := attr.Expr.Value(nil)
				if !diags.HasErrors() && val.Type().Equals(val.Type()) {
					deps = append(deps, val.AsString())
				}
			}
		}
	}

	return deps, nil
}

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/michielvha/stackgraph/pkg/graph"
	"github.com/michielvha/stackgraph/pkg/mapping"
	"github.com/michielvha/stackgraph/pkg/output"
	"github.com/michielvha/stackgraph/pkg/parser"
	"github.com/spf13/cobra"
)

func newGenerateCmd() *cobra.Command {
	var (
		inputFile  string
		sourceDir  string
		formatFlag string
		outputFile string
		inputType  string
		terragrunt bool
		renderer   string
	)

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate an infrastructure diagram",
		Long:  "Generate an infrastructure diagram from a state file, plan file, HCL source, or DOT graph input.",
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := mapping.LoadEmbedded()
			if err != nil {
				return fmt.Errorf("failed to load mappings: %w", err)
			}

			var g *graph.Graph

			switch {
			case sourceDir != "":
				if terragrunt {
					g, err = parser.ParseTerragruntDir(sourceDir)
				} else {
					g, err = parser.ParseHCLDir(sourceDir)
				}
			case inputFile != "":
				detectedType := inputType
				if detectedType == "" {
					detectedType, err = detectInputType(inputFile)
					if err != nil {
						return err
					}
				}
				data, readErr := os.ReadFile(inputFile)
				if readErr != nil {
					return fmt.Errorf("failed to read input file: %w", readErr)
				}
				g, err = parseByType(detectedType, data)
			default:
				// Try reading from stdin
				stat, _ := os.Stdin.Stat()
				if (stat.Mode() & os.ModeCharDevice) != 0 {
					return fmt.Errorf("no input provided: use --input, --source, or pipe data to stdin")
				}
				data, readErr := io.ReadAll(os.Stdin)
				if readErr != nil {
					return fmt.Errorf("failed to read stdin: %w", readErr)
				}
				detectedType := inputType
				if detectedType == "" {
					detectedType = detectContentType(data)
				}
				g, err = parseByType(detectedType, data)
			}

			if err != nil {
				return fmt.Errorf("failed to parse input: %w", err)
			}

			g.ApplyMappings(registry)
			g.ApplyGrouping()
			g.DetectImplicitEdges()
			g.CollapseCountInstances()
			g.FilterInternal()

			var out []byte
			switch formatFlag {
			case "json":
				out, err = output.RenderJSON(g)
			case "dot":
				out, err = output.RenderDOT(g)
			case "svg":
				switch renderer {
				case "graphviz":
					out, err = output.RenderGraphvizSVG(g)
				default:
					out, err = output.RenderSVG(g)
				}
			default:
				return fmt.Errorf("unsupported format: %s (supported: json, dot, svg)", formatFlag)
			}
			if err != nil {
				return fmt.Errorf("failed to render output: %w", err)
			}

			if outputFile == "" || outputFile == "-" {
				_, err = os.Stdout.Write(out)
				return err
			}
			return os.WriteFile(outputFile, out, 0o644)
		},
	}

	cmd.Flags().StringVarP(&inputFile, "input", "i", "", "Input file (state JSON, plan JSON, or DOT)")
	cmd.Flags().StringVarP(&sourceDir, "source", "s", "", "Source directory (HCL .tf files or Terragrunt project)")
	cmd.Flags().StringVarP(&formatFlag, "format", "f", "json", "Output format: json, dot, svg")
	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file (default: stdout)")
	cmd.Flags().StringVar(&inputType, "input-type", "", "Explicit input type: state, plan, dot (default: auto-detect)")
	cmd.Flags().BoolVar(&terragrunt, "terragrunt", false, "Parse source directory as a Terragrunt project")
	cmd.Flags().StringVar(&renderer, "renderer", "d2", "SVG renderer: d2 (default) or graphviz")

	return cmd
}

// detectInputType determines the input type from file extension and content.
func detectInputType(path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".dot" || ext == ".gv" {
		return "dot", nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file for type detection: %w", err)
	}

	return detectContentType(data), nil
}

// detectContentType sniffs JSON content to determine if it's a state or plan file.
func detectContentType(data []byte) string {
	trimmed := strings.TrimSpace(string(data))

	// DOT format starts with "digraph" or "graph"
	if strings.HasPrefix(trimmed, "digraph") || strings.HasPrefix(trimmed, "graph") {
		return "dot"
	}

	// Try to detect state vs plan from JSON structure
	var probe map[string]json.RawMessage
	if json.Unmarshal(data, &probe) == nil {
		if _, ok := probe["planned_values"]; ok {
			return "plan"
		}
		if _, ok := probe["values"]; ok {
			return "state"
		}
		// "resource_changes" is plan-only
		if _, ok := probe["resource_changes"]; ok {
			return "plan"
		}
	}

	return "state" // default fallback
}

func parseByType(inputType string, data []byte) (*graph.Graph, error) {
	switch inputType {
	case "state":
		return parser.ParseState(data)
	case "plan":
		return parser.ParsePlan(data)
	case "dot":
		return parser.ParseDOT(data)
	default:
		return nil, fmt.Errorf("unknown input type: %s", inputType)
	}
}

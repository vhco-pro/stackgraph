package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/vhco-pro/stackgraph/pkg/mapping"
)

func newMappingsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mappings",
		Short: "Manage resource type mappings",
	}

	cmd.AddCommand(newMappingsListCmd())
	cmd.AddCommand(newMappingsCoverageCmd())
	return cmd
}

// newMappingsCoverageCmd reports which resource types in a given state/plan
// file are mapped and which would render as generic boxes — turning unknowns
// from real usage into concrete mapping contributions.
func newMappingsCoverageCmd() *cobra.Command {
	var mappingsDir string

	cmd := &cobra.Command{
		Use:   "coverage --input state.json",
		Short: "Report mapping coverage for the resource types in a state or plan file",
		RunE: func(cmd *cobra.Command, args []string) error {
			inputFile, _ := cmd.Flags().GetString("input")
			if inputFile == "" {
				return fmt.Errorf("--input is required")
			}

			registry, err := loadRegistry(mappingsDir)
			if err != nil {
				return err
			}

			// Read once; sniff the content type from the bytes we already hold
			// (state files run to hundreds of MB — never read them twice).
			data, err := os.ReadFile(inputFile)
			if err != nil {
				return fmt.Errorf("failed to read input file: %w", err)
			}
			g, err := parseByType(detectContentType(data), data, registry.ParentRefAttrs())
			if err != nil {
				return fmt.Errorf("failed to parse input: %w", err)
			}

			counts := make(map[string]int)
			for _, n := range g.Nodes {
				counts[n.ResourceType]++
			}

			var mapped, unmapped []string
			for resourceType := range counts {
				if registry.Lookup(resourceType) != nil {
					mapped = append(mapped, resourceType)
				} else {
					unmapped = append(unmapped, resourceType)
				}
			}
			sort.Strings(mapped)
			sort.Strings(unmapped)

			out := cmd.OutOrStdout()
			total := len(mapped) + len(unmapped)
			_, _ = fmt.Fprintf(out, "%d resource types in %s: %d mapped, %d unmapped\n\n",
				total, inputFile, len(mapped), len(unmapped))

			if len(unmapped) > 0 {
				_, _ = fmt.Fprintln(out, "UNMAPPED (render as generic boxes):")
				w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				for _, rt := range unmapped {
					_, _ = fmt.Fprintf(w, "  %s\tx%d\n", rt, counts[rt])
				}
				_ = w.Flush()
				_, _ = fmt.Fprintln(out, "\nAdd mappings for these in a YAML directory and pass --mappings, or contribute them upstream.")
			} else {
				_, _ = fmt.Fprintln(out, "Full coverage — every resource type is mapped.")
			}
			return nil
		},
	}

	cmd.Flags().StringP("input", "i", "", "State or plan JSON file to analyze")
	cmd.Flags().StringVar(&mappingsDir, "mappings", "", "Directory of additional mapping YAML files")

	return cmd
}

func newMappingsListCmd() *cobra.Command {
	var (
		provider string
		category string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List supported resource type mappings",
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := mapping.LoadEmbedded()
			if err != nil {
				return fmt.Errorf("failed to load mappings: %w", err)
			}

			entries := registry.List(provider, category)
			if len(entries) == 0 {
				fmt.Println("No mappings found matching the given filters.")
				return nil
			}

			sort.Slice(entries, func(i, j int) bool {
				if entries[i].Provider != entries[j].Provider {
					return entries[i].Provider < entries[j].Provider
				}
				return entries[i].ResourceType < entries[j].ResourceType
			})

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "RESOURCE TYPE\tPROVIDER\tSERVICE\tCATEGORY")
			_, _ = fmt.Fprintln(w, strings.Repeat("-", 13)+"\t"+strings.Repeat("-", 8)+"\t"+strings.Repeat("-", 7)+"\t"+strings.Repeat("-", 8))
			for _, e := range entries {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.ResourceType, e.Provider, e.Service, e.Category)
			}
			return w.Flush()
		},
	}

	cmd.Flags().StringVarP(&provider, "provider", "p", "", "Filter by provider (e.g., aws, azure, gcp)")
	cmd.Flags().StringVarP(&category, "category", "c", "", "Filter by category (e.g., Compute, Networking)")

	return cmd
}

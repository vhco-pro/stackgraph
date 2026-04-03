package main

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/michielvha/stackgraph/pkg/mapping"
	"github.com/spf13/cobra"
)

func newMappingsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mappings",
		Short: "Manage resource type mappings",
	}

	cmd.AddCommand(newMappingsListCmd())
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
			fmt.Fprintln(w, "RESOURCE TYPE\tPROVIDER\tSERVICE\tCATEGORY")
			fmt.Fprintln(w, strings.Repeat("-", 13)+"\t"+strings.Repeat("-", 8)+"\t"+strings.Repeat("-", 7)+"\t"+strings.Repeat("-", 8))
			for _, e := range entries {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.ResourceType, e.Provider, e.Service, e.Category)
			}
			return w.Flush()
		},
	}

	cmd.Flags().StringVarP(&provider, "provider", "p", "", "Filter by provider (e.g., aws, azure, gcp)")
	cmd.Flags().StringVarP(&category, "category", "c", "", "Filter by category (e.g., Compute, Networking)")

	return cmd
}

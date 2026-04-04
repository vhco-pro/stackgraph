package main

import (
	"fmt"
	"os"

	"github.com/michielvha/logger"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	var verbose bool

	rootCmd := &cobra.Command{
		Use:     "stackgraph",
		Short:   "Infrastructure diagram generator for OpenTofu/Terraform",
		Long:    "stackgraph generates interactive infrastructure diagrams from OpenTofu/Terraform state files, plan JSON, and HCL source code.",
		Version: version,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if verbose {
				logger.Init("debug")
			}
		},
	}

	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable debug logging")

	rootCmd.AddCommand(newGenerateCmd())
	rootCmd.AddCommand(newMappingsCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

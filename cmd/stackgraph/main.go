package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:     "stackgraph",
		Short:   "Infrastructure diagram generator for OpenTofu/Terraform",
		Long:    "stackgraph generates interactive infrastructure diagrams from OpenTofu/Terraform state files, plan JSON, and HCL source code.",
		Version: version,
	}

	rootCmd.AddCommand(newGenerateCmd())
	rootCmd.AddCommand(newMappingsCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

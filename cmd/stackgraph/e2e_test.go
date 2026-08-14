package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runGenerate executes the real generate command end-to-end and returns the
// produced output file's contents.
func runGenerate(t *testing.T, args ...string) []byte {
	t.Helper()
	outFile := filepath.Join(t.TempDir(), "out")
	cmd := newGenerateCmd()
	cmd.SetArgs(append(args, "--output", outFile))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("generate %v failed: %v", args, err)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	return data
}

func TestGenerateEndToEnd_StateToSVG(t *testing.T) {
	svg := string(runGenerate(t, "--input", "../../testdata/state/aws/three-tier.json", "--format", "svg"))

	for _, want := range []string{"<svg", "</svg>", "VPC — main", "Subnet — web", "data:image"} {
		if !strings.Contains(svg, want) {
			t.Errorf("SVG output missing %q", want)
		}
	}
}

func TestGenerateEndToEnd_StateToJSON(t *testing.T) {
	out := string(runGenerate(t, "--input", "../../testdata/state/aws/two-vpc-realistic.json", "--format", "json"))

	for _, want := range []string{`"nodes"`, `"edges"`, `"aws_vpc.prod"`, `"parent"`} {
		if !strings.Contains(out, want) {
			t.Errorf("JSON output missing %q", want)
		}
	}
}

func TestGenerateEndToEnd_PlanToSVG(t *testing.T) {
	svg := string(runGenerate(t, "--input", "../../testdata/plan/aws/plan-changes.json", "--format", "svg"))

	// The destroyed bucket must render, carrying the delete accent color
	if !strings.Contains(svg, "old") {
		t.Error("destroyed resource missing from plan SVG")
	}
	if !strings.Contains(svg, "#E53935") {
		t.Error("delete accent color missing from plan SVG")
	}
}

func TestGenerateEndToEnd_DOT(t *testing.T) {
	out := string(runGenerate(t, "--input", "../../testdata/dot/simple.dot", "--format", "dot"))
	if !strings.Contains(out, "digraph") {
		t.Error("DOT output missing digraph")
	}
}

func TestGenerateEndToEnd_HCL(t *testing.T) {
	svg := string(runGenerate(t, "--source", "../../testdata/hcl/simple-aws", "--format", "svg"))
	if !strings.Contains(svg, "<svg") {
		t.Error("HCL source did not produce SVG")
	}
}

func TestGenerateEndToEnd_Terragrunt(t *testing.T) {
	out := string(runGenerate(t, "--source", "../../testdata/terragrunt/simple", "--terragrunt", "--format", "json"))
	if !strings.Contains(out, "module.app") || !strings.Contains(out, "module.vpc") {
		t.Error("terragrunt output missing module nodes")
	}
}

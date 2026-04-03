package parser

import (
	"testing"
)

func TestParseHCLDir_SimpleAWS(t *testing.T) {
	g, err := ParseHCLDir("../../testdata/hcl/simple-aws")
	if err != nil {
		t.Fatalf("ParseHCLDir failed: %v", err)
	}

	if g.Metadata.InputMode != "hcl" {
		t.Errorf("expected input mode 'hcl', got %q", g.Metadata.InputMode)
	}

	if len(g.Nodes) != 3 {
		t.Errorf("expected 3 resource nodes, got %d", len(g.Nodes))
	}

	// Check VPC exists
	vpc := g.NodeByID("aws_vpc.main")
	if vpc == nil {
		t.Fatal("expected aws_vpc.main node")
	}
	if vpc.Provider != "aws" {
		t.Errorf("expected provider aws, got %q", vpc.Provider)
	}

	// Check cross-references are detected as edges
	// aws_subnet.public references aws_vpc.main via aws_vpc.main.id
	var hasVPCRef bool
	for _, e := range g.Edges {
		if e.Source == "aws_subnet.public" && e.Target == "aws_vpc.main" {
			hasVPCRef = true
			break
		}
	}
	if !hasVPCRef {
		t.Error("expected edge from aws_subnet.public to aws_vpc.main via HCL variable reference")
	}

	// aws_instance.web references aws_subnet.public
	var hasSubnetRef bool
	for _, e := range g.Edges {
		if e.Source == "aws_instance.web" && e.Target == "aws_subnet.public" {
			hasSubnetRef = true
			break
		}
	}
	if !hasSubnetRef {
		t.Error("expected edge from aws_instance.web to aws_subnet.public via HCL variable reference")
	}
}

func TestParseHCLDir_NoTFFiles(t *testing.T) {
	_, err := ParseHCLDir("../../testdata/invalid")
	if err == nil {
		t.Fatal("expected error for directory with no .tf files, got nil")
	}
}

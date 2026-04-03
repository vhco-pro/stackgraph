package parser

import (
	"testing"
)

func TestParseDOT_Simple(t *testing.T) {
	data := readTestFile(t, "../../testdata/dot/simple.dot")
	g, err := ParseDOT(data)
	if err != nil {
		t.Fatalf("ParseDOT failed: %v", err)
	}

	if len(g.Nodes) == 0 {
		t.Fatal("expected nodes, got none")
	}
	if g.Metadata.InputMode != "dot" {
		t.Errorf("expected input mode 'dot', got %q", g.Metadata.InputMode)
	}

	// Check that terraform graph nodes are parsed
	instance := g.NodeByID("aws_instance.web")
	if instance == nil {
		t.Error("expected aws_instance.web node (after cleaning [root] prefix)")
	}

	vpc := g.NodeByID("aws_vpc.main")
	if vpc == nil {
		t.Error("expected aws_vpc.main node")
	}

	// Check edges exist
	if len(g.Edges) == 0 {
		t.Error("expected edges, got none")
	}
}

func TestParseDOT_Malformed(t *testing.T) {
	data := readTestFile(t, "../../testdata/invalid/malformed.dot")
	_, err := ParseDOT(data)
	if err == nil {
		t.Fatal("expected error for malformed DOT, got nil")
	}
}

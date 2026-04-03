package mapping

import (
	"testing"
)

func TestLoadEmbedded(t *testing.T) {
	r, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded failed: %v", err)
	}

	// Should have AWS mappings loaded
	entries := r.List("aws", "")
	if len(entries) == 0 {
		t.Fatal("expected AWS mappings, got none")
	}

	// Check a specific mapping
	result := r.Lookup("aws_instance")
	if result == nil {
		t.Fatal("expected mapping for aws_instance")
	}
	if result.Service != "EC2" {
		t.Errorf("expected service EC2, got %q", result.Service)
	}
	if result.Category != "Compute" {
		t.Errorf("expected category Compute, got %q", result.Category)
	}
}

func TestLookup_UnmappedReturnsNil(t *testing.T) {
	r, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded failed: %v", err)
	}

	result := r.Lookup("nonexistent_resource_type")
	if result != nil {
		t.Error("expected nil for unmapped resource type")
	}
}

func TestLookup_GroupNode(t *testing.T) {
	r, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded failed: %v", err)
	}

	result := r.Lookup("aws_vpc")
	if result == nil {
		t.Fatal("expected mapping for aws_vpc")
	}
	if !result.IsGroup {
		t.Error("expected aws_vpc to be a group node")
	}
	if result.GroupLevel != 1 {
		t.Errorf("expected group level 1, got %d", result.GroupLevel)
	}
}

func TestLookup_Variants(t *testing.T) {
	r, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded failed: %v", err)
	}

	result := r.Lookup("aws_ecs_service")
	if result == nil {
		t.Fatal("expected mapping for aws_ecs_service")
	}
	if len(result.Variants) == 0 {
		t.Fatal("expected variants for aws_ecs_service")
	}
	if result.Variants[0].MatchAttribute != "launch_type" {
		t.Errorf("expected variant match attribute 'launch_type', got %q", result.Variants[0].MatchAttribute)
	}
}

func TestListFilters(t *testing.T) {
	r, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded failed: %v", err)
	}

	// Filter by category
	compute := r.List("aws", "Compute")
	if len(compute) == 0 {
		t.Error("expected Compute resources for AWS")
	}
	for _, e := range compute {
		if e.Category != "Compute" {
			t.Errorf("expected category Compute, got %q for %s", e.Category, e.ResourceType)
		}
	}

	// Filter by non-existent provider
	empty := r.List("nonexistent", "")
	if len(empty) != 0 {
		t.Errorf("expected 0 results for nonexistent provider, got %d", len(empty))
	}
}

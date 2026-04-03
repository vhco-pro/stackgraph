package graph

import (
	"testing"
)

func TestCollapseCountInstances(t *testing.T) {
	g := NewGraph("state")

	// Add 3 indexed instances
	g.AddNode(&Node{ID: "aws_instance.web[0]", Type: NodeTypeResource, ResourceType: "aws_instance", Label: "web"})
	g.AddNode(&Node{ID: "aws_instance.web[1]", Type: NodeTypeResource, ResourceType: "aws_instance", Label: "web"})
	g.AddNode(&Node{ID: "aws_instance.web[2]", Type: NodeTypeResource, ResourceType: "aws_instance", Label: "web"})
	// Add a non-indexed resource
	g.AddNode(&Node{ID: "aws_vpc.main", Type: NodeTypeResource, ResourceType: "aws_vpc", Label: "main"})

	// Add edges from instances to VPC
	g.AddEdge(&Edge{Source: "aws_instance.web[0]", Target: "aws_vpc.main", Type: "depends_on"})
	g.AddEdge(&Edge{Source: "aws_instance.web[1]", Target: "aws_vpc.main", Type: "depends_on"})
	g.AddEdge(&Edge{Source: "aws_instance.web[2]", Target: "aws_vpc.main", Type: "depends_on"})

	g.CollapseCountInstances()

	// Should have 2 nodes: collapsed instance + VPC
	if len(g.Nodes) != 2 {
		t.Errorf("expected 2 nodes after collapse, got %d", len(g.Nodes))
	}

	// Check collapsed node
	collapsed := g.NodeByID("aws_instance.web")
	if collapsed == nil {
		t.Fatal("expected collapsed node aws_instance.web")
	}
	if collapsed.Count != 3 {
		t.Errorf("expected count 3, got %d", collapsed.Count)
	}

	// Edges should be deduplicated
	if len(g.Edges) != 1 {
		t.Errorf("expected 1 deduplicated edge after collapse, got %d", len(g.Edges))
	}
	if g.Edges[0].Source != "aws_instance.web" {
		t.Errorf("expected edge source aws_instance.web, got %q", g.Edges[0].Source)
	}
}

func TestFilterInternal(t *testing.T) {
	g := NewGraph("dot")

	g.AddNode(&Node{ID: "aws_instance.web", Type: NodeTypeResource, ResourceType: "aws_instance"})
	g.AddNode(&Node{ID: "provider[\"registry.terraform.io/hashicorp/aws\"]", Type: NodeTypeResource, ResourceType: "provider[\"registry.terraform.io/hashicorp/aws\"]"})
	g.AddNode(&Node{ID: "[root] root", Type: NodeTypeResource, ResourceType: "root"})

	g.AddEdge(&Edge{Source: "aws_instance.web", Target: "provider[\"registry.terraform.io/hashicorp/aws\"]"})
	g.AddEdge(&Edge{Source: "aws_instance.web", Target: "[root] root"})

	g.FilterInternal()

	if len(g.Nodes) != 1 {
		t.Errorf("expected 1 node after filtering, got %d", len(g.Nodes))
	}
	if g.Nodes[0].ID != "aws_instance.web" {
		t.Errorf("expected aws_instance.web to survive filtering, got %q", g.Nodes[0].ID)
	}
	if len(g.Edges) != 0 {
		t.Errorf("expected 0 edges after filtering, got %d", len(g.Edges))
	}
}

func TestGenericFallback(t *testing.T) {
	g := NewGraph("state")

	// Add a node with an unmapped resource type
	g.AddNode(&Node{
		ID:           "custom_resource.foo",
		Type:         NodeTypeResource,
		ResourceType: "custom_resource",
		Label:        "foo",
		Provider:     "custom",
	})

	// Node should exist with resource type as-is, no crash
	n := g.NodeByID("custom_resource.foo")
	if n == nil {
		t.Fatal("expected unmapped resource to exist in graph")
	}
	if n.Service != "" {
		t.Errorf("expected empty service for unmapped resource, got %q", n.Service)
	}
	if n.Label != "foo" {
		t.Errorf("expected label 'foo', got %q", n.Label)
	}
}

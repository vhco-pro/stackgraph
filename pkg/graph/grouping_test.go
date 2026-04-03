package graph

import (
	"testing"
)

func TestApplyGrouping_VPCSubnetInstance(t *testing.T) {
	g := NewGraph("state")

	vpc := &Node{
		ID: "aws_vpc.main", Type: NodeTypeGroup, ResourceType: "aws_vpc",
		Label: "main", IsGroupNode: true, GroupLevel: 1,
	}
	subnet := &Node{
		ID: "aws_subnet.public", Type: NodeTypeGroup, ResourceType: "aws_subnet",
		Label: "public", IsGroupNode: true, GroupLevel: 2, GroupParent: "aws_vpc",
	}
	instance := &Node{
		ID: "aws_instance.web", Type: NodeTypeResource, ResourceType: "aws_instance",
		Label: "web", GroupParent: "aws_subnet",
	}

	g.AddNode(vpc)
	g.AddNode(subnet)
	g.AddNode(instance)

	// Add dependency edges
	g.AddEdge(&Edge{Source: "aws_subnet.public", Target: "aws_vpc.main", Type: "depends_on"})
	g.AddEdge(&Edge{Source: "aws_instance.web", Target: "aws_subnet.public", Type: "depends_on"})

	g.ApplyGrouping()

	// Subnet should be inside VPC
	if subnet.Parent != "aws_vpc.main" {
		t.Errorf("expected subnet parent to be aws_vpc.main, got %q", subnet.Parent)
	}

	// VPC should have subnet as child
	if len(vpc.Children) == 0 || vpc.Children[0] != "aws_subnet.public" {
		t.Errorf("expected VPC to have subnet as child, got %v", vpc.Children)
	}

	// Instance should be inside subnet
	if instance.Parent != "aws_subnet.public" {
		t.Errorf("expected instance parent to be aws_subnet.public, got %q", instance.Parent)
	}
}

func TestDetectImplicitEdges(t *testing.T) {
	g := NewGraph("state")

	g.AddNode(&Node{
		ID: "aws_vpc.main", Type: NodeTypeResource, ResourceType: "aws_vpc",
		Attributes: map[string]interface{}{"id": "vpc-123"},
	})
	g.AddNode(&Node{
		ID: "aws_subnet.public", Type: NodeTypeResource, ResourceType: "aws_subnet",
		Attributes: map[string]interface{}{"id": "subnet-456", "vpc_id": "vpc-123"},
	})

	g.DetectImplicitEdges()

	// Should have an implicit edge from subnet to VPC via vpc_id
	var found bool
	for _, e := range g.Edges {
		if e.Source == "aws_subnet.public" && e.Target == "aws_vpc.main" && e.Type == "vpc_id" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected implicit edge from subnet to VPC via vpc_id")
	}
}

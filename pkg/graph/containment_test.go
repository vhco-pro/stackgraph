package graph

import "testing"

// buildTwoVPCGraph models a state with NO depends_on edges: two VPCs with a
// subnet and an instance each, plus one count-expanded instance group.
// Containment must come purely from parent_ref attribute resolution.
func buildTwoVPCGraph() *Graph {
	g := NewGraph("state")

	addVPC := func(env string) {
		g.AddNode(&Node{
			ID: "aws_vpc." + env, ResourceType: "aws_vpc", Label: env, Provider: "aws",
			IsGroupNode: true, GroupLevel: 1,
			Attributes: map[string]any{"id": "vpc-" + env},
		})
		g.AddNode(&Node{
			ID: "aws_subnet." + env, ResourceType: "aws_subnet", Label: env, Provider: "aws",
			IsGroupNode: true, GroupLevel: 2, GroupParent: "aws_vpc", ParentRef: "vpc_id",
			Attributes: map[string]any{"id": "subnet-" + env, "vpc_id": "vpc-" + env},
		})
		g.AddNode(&Node{
			ID: "aws_instance." + env, ResourceType: "aws_instance", Label: env, Provider: "aws",
			GroupParent: "aws_subnet", ParentRef: "subnet_id",
			Attributes: map[string]any{"id": "i-" + env, "subnet_id": "subnet-" + env},
		})
	}
	addVPC("prod")
	addVPC("staging")

	for i := 0; i < 3; i++ {
		g.AddNode(&Node{
			ID: "aws_instance.web[" + string(rune('0'+i)) + "]", ResourceType: "aws_instance",
			Label: "web", Provider: "aws", GroupParent: "aws_subnet", ParentRef: "subnet_id",
			Attributes: map[string]any{"id": "i-web" + string(rune('0'+i)), "subnet_id": "subnet-prod"},
		})
	}
	return g
}

func TestParentRefContainmentWithoutDependsOn(t *testing.T) {
	g := buildTwoVPCGraph()
	g.DetectImplicitEdges()
	g.ApplyGrouping()

	cases := map[string]string{
		"aws_subnet.prod":      "aws_vpc.prod",
		"aws_subnet.staging":   "aws_vpc.staging",
		"aws_instance.prod":    "aws_subnet.prod",
		"aws_instance.staging": "aws_subnet.staging",
		"aws_instance.web[0]":  "aws_subnet.prod",
	}
	for id, wantParent := range cases {
		n := g.NodeByID(id)
		if n == nil {
			t.Fatalf("node %s missing", id)
		}
		if n.Parent != wantParent {
			t.Errorf("node %s: parent = %q, want %q", id, n.Parent, wantParent)
		}
	}
}

func TestContainmentEdgesSuppressed(t *testing.T) {
	g := buildTwoVPCGraph()
	g.DetectImplicitEdges()
	g.ApplyGrouping()

	for _, e := range g.Edges {
		src, tgt := g.NodeByID(e.Source), g.NodeByID(e.Target)
		if src == nil || tgt == nil {
			continue
		}
		if src.Parent == tgt.ID || tgt.Parent == src.ID {
			t.Errorf("edge %s -> %s (%s) restates containment and should be suppressed",
				e.Source, e.Target, e.Type)
		}
	}
}

func TestCollapsePreservesContainment(t *testing.T) {
	g := buildTwoVPCGraph()
	g.DetectImplicitEdges()
	g.ApplyGrouping()
	g.CollapseCountInstances()

	web := g.NodeByID("aws_instance.web")
	if web == nil {
		t.Fatal("collapsed node aws_instance.web missing")
	}
	if web.Count != 3 {
		t.Errorf("collapsed count = %d, want 3", web.Count)
	}
	if web.Parent != "aws_subnet.prod" {
		t.Errorf("collapsed node parent = %q, want aws_subnet.prod", web.Parent)
	}

	subnet := g.NodeByID("aws_subnet.prod")
	found := false
	for _, c := range subnet.Children {
		if c == "aws_instance.web" {
			found = true
		}
		if g.NodeByID(c) == nil {
			t.Errorf("subnet child %q no longer exists (stale member reference)", c)
		}
	}
	if !found {
		t.Error("subnet children do not include the collapsed node")
	}
}

func TestNameBasedContainmentRequiresTypeFilter(t *testing.T) {
	g := NewGraph("state")
	g.AddNode(&Node{
		ID: "azurerm_resource_group.main", ResourceType: "azurerm_resource_group",
		Label: "main", Provider: "azurerm", IsGroupNode: true, GroupLevel: 0,
		Attributes: map[string]any{"id": "/subs/1/rg/prod-rg", "name": "prod-rg"},
	})
	// Decoy: a non-parent type sharing the same name must not match.
	g.AddNode(&Node{
		ID: "azurerm_storage_account.decoy", ResourceType: "azurerm_storage_account",
		Label: "decoy", Provider: "azurerm",
		Attributes: map[string]any{"id": "/subs/1/st/x", "name": "prod-rg"},
	})
	g.AddNode(&Node{
		ID: "azurerm_virtual_network.main", ResourceType: "azurerm_virtual_network",
		Label: "main", Provider: "azurerm", IsGroupNode: true, GroupLevel: 1,
		GroupParent: "azurerm_resource_group", ParentRef: "resource_group_name",
		Attributes: map[string]any{"id": "/subs/1/vnet/prod", "name": "prod-vnet", "resource_group_name": "prod-rg"},
	})

	g.ApplyGrouping()

	vnet := g.NodeByID("azurerm_virtual_network.main")
	if vnet.Parent != "azurerm_resource_group.main" {
		t.Errorf("vnet parent = %q, want azurerm_resource_group.main", vnet.Parent)
	}
}

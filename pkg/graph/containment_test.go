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

func TestCollapseSkipsMembersInDifferentParents(t *testing.T) {
	// for_each spread across AZ subnets: collapsing would misplace instances,
	// so groups with mixed parents must stay expanded.
	g := NewGraph("state")
	for _, az := range []string{"a", "b"} {
		g.AddNode(&Node{
			ID: "aws_subnet." + az, ResourceType: "aws_subnet", Label: az, Provider: "aws",
			IsGroupNode: true, GroupLevel: 2,
			Attributes: map[string]any{"id": "subnet-" + az},
		})
	}
	for i, az := range []string{"a", "b"} {
		g.AddNode(&Node{
			ID: "aws_instance.web[" + string(rune('0'+i)) + "]", ResourceType: "aws_instance",
			Label: "web", Provider: "aws", ParentRef: "subnet_id", GroupParent: "aws_subnet",
			Attributes: map[string]any{"id": "i-" + az, "subnet_id": "subnet-" + az},
		})
	}
	g.ApplyGrouping()
	g.CollapseCountInstances()

	if g.NodeByID("aws_instance.web") != nil {
		t.Fatal("mixed-parent count group must not collapse")
	}
	for i, az := range []string{"a", "b"} {
		n := g.NodeByID("aws_instance.web[" + string(rune('0'+i)) + "]")
		if n == nil {
			t.Fatalf("member %d missing", i)
		}
		if n.Parent != "aws_subnet."+az {
			t.Errorf("member %d parent = %q, want aws_subnet.%s", i, n.Parent, az)
		}
	}
}

func TestMemberRefContainment(t *testing.T) {
	g := NewGraph("state")
	g.AddNode(&Node{
		ID: "aws_subnet.web", ResourceType: "aws_subnet", Label: "web", Provider: "aws",
		IsGroupNode: true, GroupLevel: 2,
		Attributes: map[string]any{"id": "subnet-1"},
	})
	g.AddNode(&Node{
		ID: "aws_security_group.web", ResourceType: "aws_security_group", Label: "web",
		Provider: "aws", IsGroupNode: true, GroupLevel: 3,
		MemberRef:  "vpc_security_group_ids",
		Attributes: map[string]any{"id": "sg-1"},
	})
	g.AddNode(&Node{
		ID: "aws_instance.web", ResourceType: "aws_instance", Label: "web", Provider: "aws",
		ParentRef: "subnet_id", GroupParent: "aws_subnet",
		Attributes: map[string]any{"id": "i-1", "subnet_id": "subnet-1", "vpc_security_group_ids": []any{"sg-1"}},
	})

	g.ApplyGrouping()

	inst := g.NodeByID("aws_instance.web")
	sg := g.NodeByID("aws_security_group.web")
	if inst.Parent != "aws_security_group.web" {
		t.Errorf("instance parent = %q, want the security group", inst.Parent)
	}
	if sg.Parent != "aws_subnet.web" {
		t.Errorf("security group parent = %q, want the member's subnet", sg.Parent)
	}
}

func TestNetworkScopeBoundsFlowEdges(t *testing.T) {
	g := NewGraph("state")
	for _, env := range []string{"prod", "staging"} {
		g.AddNode(&Node{
			ID: "aws_vpc." + env, ResourceType: "aws_vpc", Label: env, Provider: "aws",
			IsGroupNode: true, GroupLevel: 1, NetworkScope: true,
			Attributes: map[string]any{"id": "vpc-" + env},
		})
		g.AddNode(&Node{
			ID: "aws_instance." + env, Type: NodeTypeResource, ResourceType: "aws_instance",
			Label: env, Provider: "aws", ParentRef: "vpc_id", GroupParent: "aws_vpc",
			Attributes: map[string]any{"id": "i-" + env, "vpc_id": "vpc-" + env},
		})
		g.AddNode(&Node{
			ID: "aws_db_instance." + env, Type: NodeTypeResource, ResourceType: "aws_db_instance",
			Label: env, Provider: "aws", ParentRef: "vpc_id", GroupParent: "aws_vpc",
			Attributes: map[string]any{"id": "db-" + env, "vpc_id": "vpc-" + env},
		})
	}
	g.ApplyGrouping()
	g.InferFlowEdges([][2]string{{"aws_instance", "aws_db_instance"}})

	for _, e := range g.Edges {
		if e.Type != "flow" {
			continue
		}
		srcEnv := e.Source[len(e.Source)-4:]
		tgtEnv := e.Target[len(e.Target)-4:]
		if srcEnv != tgtEnv {
			t.Errorf("flow edge crosses network scopes: %s -> %s", e.Source, e.Target)
		}
	}
	flowCount := 0
	for _, e := range g.Edges {
		if e.Type == "flow" {
			flowCount++
		}
	}
	if flowCount != 2 {
		t.Errorf("expected 2 same-scope flow edges, got %d", flowCount)
	}
}

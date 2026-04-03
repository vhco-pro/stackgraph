package parser

import (
	"os"
	"testing"
)

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read test file %s: %v", path, err)
	}
	return data
}

func TestParseState_AWSSimpleVPC(t *testing.T) {
	data := readTestFile(t, "../../testdata/state/aws/simple-vpc.json")
	g, err := ParseState(data)
	if err != nil {
		t.Fatalf("ParseState failed: %v", err)
	}

	if len(g.Nodes) == 0 {
		t.Fatal("expected nodes, got none")
	}
	if g.Metadata.InputMode != "state" {
		t.Errorf("expected input mode 'state', got %q", g.Metadata.InputMode)
	}

	// Check VPC node exists
	vpc := g.NodeByID("aws_vpc.main")
	if vpc == nil {
		t.Fatal("expected aws_vpc.main node")
	}
	if vpc.ResourceType != "aws_vpc" {
		t.Errorf("expected resource type aws_vpc, got %q", vpc.ResourceType)
	}
	if vpc.Provider != "aws" {
		t.Errorf("expected provider aws, got %q", vpc.Provider)
	}

	// Check attributes are curated (id should be kept)
	if vpc.Attributes == nil {
		t.Fatal("expected attributes on VPC node")
	}
	if _, ok := vpc.Attributes["id"]; !ok {
		t.Error("expected 'id' in curated attributes")
	}
	if _, ok := vpc.Attributes["cidr_block"]; !ok {
		t.Error("expected 'cidr_block' in curated attributes")
	}

	// Check subnet has depends_on edge to VPC
	var hasVPCEdge bool
	for _, e := range g.Edges {
		if e.Source == "aws_subnet.public" && e.Target == "aws_vpc.main" {
			hasVPCEdge = true
			break
		}
	}
	if !hasVPCEdge {
		t.Error("expected edge from aws_subnet.public to aws_vpc.main")
	}
}

func TestParseState_AWSCountForEach(t *testing.T) {
	data := readTestFile(t, "../../testdata/state/aws/count-foreach.json")
	g, err := ParseState(data)
	if err != nil {
		t.Fatalf("ParseState failed: %v", err)
	}

	// Should have 7 resources (1 VPC + 3 subnets + 3 instances)
	if len(g.Nodes) != 7 {
		t.Errorf("expected 7 nodes, got %d", len(g.Nodes))
	}

	// Check indexed resources exist
	sub0 := g.NodeByID("aws_subnet.main[0]")
	if sub0 == nil {
		t.Error("expected aws_subnet.main[0] node")
	}

	inst2 := g.NodeByID("aws_instance.web[2]")
	if inst2 == nil {
		t.Error("expected aws_instance.web[2] node")
	}
}

func TestParseState_Proxmox(t *testing.T) {
	data := readTestFile(t, "../../testdata/state/proxmox/basic.json")
	g, err := ParseState(data)
	if err != nil {
		t.Fatalf("ParseState failed: %v", err)
	}

	if len(g.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(g.Nodes))
	}

	vm := g.NodeByID("proxmox_vm_qemu.web")
	if vm == nil {
		t.Fatal("expected proxmox_vm_qemu.web node")
	}
	if vm.Provider != "proxmox" {
		t.Errorf("expected provider proxmox, got %q", vm.Provider)
	}
}

func TestParseState_Azure(t *testing.T) {
	data := readTestFile(t, "../../testdata/state/azure/simple-rg.json")
	g, err := ParseState(data)
	if err != nil {
		t.Fatalf("ParseState failed: %v", err)
	}

	if len(g.Nodes) != 4 {
		t.Errorf("expected 4 nodes, got %d", len(g.Nodes))
	}

	rg := g.NodeByID("azurerm_resource_group.main")
	if rg == nil {
		t.Fatal("expected azurerm_resource_group.main node")
	}
	if rg.Provider != "azurerm" {
		t.Errorf("expected provider azurerm, got %q", rg.Provider)
	}

	vm := g.NodeByID("azurerm_virtual_machine.web")
	if vm == nil {
		t.Fatal("expected azurerm_virtual_machine.web node")
	}
}

func TestParseState_GCP(t *testing.T) {
	data := readTestFile(t, "../../testdata/state/gcp/simple-vpc.json")
	g, err := ParseState(data)
	if err != nil {
		t.Fatalf("ParseState failed: %v", err)
	}

	if len(g.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(g.Nodes))
	}

	vpc := g.NodeByID("google_compute_network.main")
	if vpc == nil {
		t.Fatal("expected google_compute_network.main node")
	}
	if vpc.Provider != "google" {
		t.Errorf("expected provider google, got %q", vpc.Provider)
	}

	instance := g.NodeByID("google_compute_instance.web")
	if instance == nil {
		t.Fatal("expected google_compute_instance.web node")
	}
}

func TestParseState_EmptyState(t *testing.T) {
	data := readTestFile(t, "../../testdata/invalid/empty-state.json")
	g, err := ParseState(data)
	if err != nil {
		t.Fatalf("ParseState failed: %v", err)
	}
	if len(g.Nodes) != 0 {
		t.Errorf("expected 0 nodes for empty state, got %d", len(g.Nodes))
	}
}

func TestParseState_MalformedJSON(t *testing.T) {
	data := readTestFile(t, "../../testdata/invalid/malformed-state.json")
	_, err := ParseState(data)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

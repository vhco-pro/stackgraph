package output

import (
	"strings"
	"testing"

	"github.com/vhco-pro/stackgraph/pkg/graph"
)

const ec2Icon = "aws/Arch_Compute/Arch_Amazon-EC2_64.png"

func TestRenderSVG(t *testing.T) {
	g := graph.NewGraph("state")
	g.AddNode(&graph.Node{
		ID: "aws_instance.web", Type: graph.NodeTypeResource,
		ResourceType: "aws_instance", Label: "web", Service: "EC2",
		Icon: ec2Icon,
	})
	g.AddNode(&graph.Node{
		ID: "aws_vpc.main", Type: graph.NodeTypeGroup,
		ResourceType: "aws_vpc", Label: "main", Service: "VPC",
		IsGroupNode: true, GroupLevel: 1,
		Style:    &graph.NodeStyle{Fill: "#F8F4FF", Stroke: "#8C4FFF", Label: "#6B21A8", Role: "vpc"},
		Children: []string{"aws_instance.web"},
	})
	g.Nodes[0].Parent = "aws_vpc.main"

	out, err := RenderSVG(g)
	if err != nil {
		t.Fatalf("RenderSVG failed: %v", err)
	}

	svg := string(out)

	if !strings.Contains(svg, "<svg") {
		t.Error("expected <svg tag in output")
	}
	if !strings.Contains(svg, "</svg>") {
		t.Error("expected closing </svg> tag")
	}
	if !strings.Contains(svg, "EC2") {
		t.Error("expected EC2 service label in SVG")
	}
	if !strings.Contains(svg, "prefers-color-scheme") {
		t.Error("expected dark mode CSS media query")
	}
	if !strings.Contains(svg, "data:image") {
		t.Error("expected base64 embedded icon")
	}
	if !strings.Contains(svg, "sg-container-vpc") {
		t.Error("expected mapping-declared container role class in SVG")
	}
	if !strings.Contains(svg, "#8C4FFF") {
		t.Error("expected mapping-declared container stroke color in SVG")
	}
}

func TestRenderSVG_Empty(t *testing.T) {
	g := graph.NewGraph("state")

	out, err := RenderSVG(g)
	if err != nil {
		t.Fatalf("RenderSVG failed: %v", err)
	}

	if !strings.Contains(string(out), "No resources found") {
		t.Error("expected 'No resources found' message in empty SVG")
	}
}

func TestRenderSVG_CountBadge(t *testing.T) {
	g := graph.NewGraph("state")
	g.AddNode(&graph.Node{
		ID: "aws_instance.web", Type: graph.NodeTypeResource,
		ResourceType: "aws_instance", Label: "web", Count: 3,
		Icon: ec2Icon,
	})

	out, err := RenderSVG(g)
	if err != nil {
		t.Fatalf("RenderSVG failed: %v", err)
	}

	if !strings.Contains(string(out), "x3") {
		t.Error("expected count badge 'x3' in SVG")
	}
}

func TestRenderSVG_SelfContained(t *testing.T) {
	g := graph.NewGraph("state")
	g.AddNode(&graph.Node{
		ID: "aws_instance.web", Type: graph.NodeTypeResource,
		ResourceType: "aws_instance", Label: "web", Service: "EC2",
		Icon: ec2Icon,
	})

	out, err := RenderSVG(g)
	if err != nil {
		t.Fatalf("RenderSVG failed: %v", err)
	}

	svg := string(out)

	// Should not reference external files
	if strings.Contains(svg, "xlink:href=\"/") || strings.Contains(svg, "href=\"/tmp") {
		t.Error("SVG contains external file reference — should be self-contained")
	}
}

func TestRenderSVG_UnmappedBoxNode(t *testing.T) {
	g := graph.NewGraph("state")
	g.AddNode(&graph.Node{
		ID: "proxmox_vm_qemu.web", Type: graph.NodeTypeResource,
		ResourceType: "proxmox_vm_qemu", Label: "a-rather-long-instance-name",
	})

	out, err := RenderSVG(g)
	if err != nil {
		t.Fatalf("RenderSVG failed: %v", err)
	}

	svg := string(out)
	// Full label must survive — no truncation/ellipsis
	if !strings.Contains(svg, "a-rather-long-instance-name") {
		t.Error("expected full label in SVG, found truncation")
	}
	if strings.Contains(svg, "...") {
		t.Error("box labels must not be ellipsized")
	}
}

func TestRenderSVG_PlanActionAccent(t *testing.T) {
	g := graph.NewGraph("plan")
	g.AddNode(&graph.Node{
		ID: "aws_instance.web", Type: graph.NodeTypeResource,
		ResourceType: "aws_instance", Label: "web", Service: "EC2",
		Icon: ec2Icon, Action: graph.ActionCreate,
	})

	out, err := RenderSVG(g)
	if err != nil {
		t.Fatalf("RenderSVG failed: %v", err)
	}

	// Icon nodes must carry the action accent ring (create = green)
	if !strings.Contains(string(out), "#43A047") {
		t.Error("expected create-action accent color on icon node")
	}
}

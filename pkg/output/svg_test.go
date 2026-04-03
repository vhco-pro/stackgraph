package output

import (
	"strings"
	"testing"

	"github.com/michielvha/stackgraph/pkg/graph"
)

func TestRenderSVG(t *testing.T) {
	g := graph.NewGraph("state")
	g.AddNode(&graph.Node{
		ID: "aws_instance.web", Type: graph.NodeTypeResource,
		ResourceType: "aws_instance", Label: "web", Service: "EC2",
	})
	g.AddNode(&graph.Node{
		ID: "aws_vpc.main", Type: graph.NodeTypeGroup,
		ResourceType: "aws_vpc", Label: "main", Service: "VPC",
	})

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
	// D2 renders labels as text in the SVG
	if !strings.Contains(svg, "EC2") {
		t.Error("expected EC2 service label in SVG")
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
	})

	out, err := RenderSVG(g)
	if err != nil {
		t.Fatalf("RenderSVG failed: %v", err)
	}

	if !strings.Contains(string(out), "x3") {
		t.Error("expected count badge 'x3' in SVG")
	}
}

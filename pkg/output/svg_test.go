package output

import (
	"strings"
	"testing"

	"github.com/michielvha/stackgraph/pkg/graph"
)

func TestRenderDagreSVG(t *testing.T) {
	g := graph.NewGraph("state")
	g.AddNode(&graph.Node{
		ID: "aws_instance.web", Type: graph.NodeTypeResource,
		ResourceType: "aws_instance", Label: "web", Service: "EC2",
	})
	g.AddNode(&graph.Node{
		ID: "aws_vpc.main", Type: graph.NodeTypeGroup,
		ResourceType: "aws_vpc", Label: "main", Service: "VPC",
		IsGroupNode: true, GroupLevel: 1,
		Children: []string{"aws_instance.web"},
	})
	g.Nodes[0].Parent = "aws_vpc.main"

	out, err := RenderDagreSVG(g)
	if err != nil {
		t.Fatalf("RenderDagreSVG failed: %v", err)
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
}

func TestRenderDagreSVG_Empty(t *testing.T) {
	g := graph.NewGraph("state")

	out, err := RenderDagreSVG(g)
	if err != nil {
		t.Fatalf("RenderDagreSVG failed: %v", err)
	}

	if !strings.Contains(string(out), "No resources found") {
		t.Error("expected 'No resources found' message in empty SVG")
	}
}

func TestRenderDagreSVG_CountBadge(t *testing.T) {
	g := graph.NewGraph("state")
	g.AddNode(&graph.Node{
		ID: "aws_instance.web", Type: graph.NodeTypeResource,
		ResourceType: "aws_instance", Label: "web", Count: 3,
	})

	out, err := RenderDagreSVG(g)
	if err != nil {
		t.Fatalf("RenderDagreSVG failed: %v", err)
	}

	if !strings.Contains(string(out), "x3") {
		t.Error("expected count badge 'x3' in SVG")
	}
}

func TestRenderDagreSVG_SelfContained(t *testing.T) {
	g := graph.NewGraph("state")
	g.AddNode(&graph.Node{
		ID: "aws_instance.web", Type: graph.NodeTypeResource,
		ResourceType: "aws_instance", Label: "web", Service: "EC2",
	})

	out, err := RenderDagreSVG(g)
	if err != nil {
		t.Fatalf("RenderDagreSVG failed: %v", err)
	}

	svg := string(out)

	// Should not reference external files
	if strings.Contains(svg, "xlink:href=\"/") || strings.Contains(svg, "href=\"/tmp") {
		t.Error("SVG contains external file reference — should be self-contained")
	}
}

package output

import (
	"strings"
	"testing"

	"github.com/michielvha/stackgraph/pkg/graph"
)

func TestRenderDOT(t *testing.T) {
	g := graph.NewGraph("state")
	g.AddNode(&graph.Node{
		ID: "aws_instance.web", Type: graph.NodeTypeResource,
		ResourceType: "aws_instance", Label: "web", Service: "EC2",
	})
	g.AddNode(&graph.Node{
		ID: "aws_vpc.main", Type: graph.NodeTypeGroup,
		ResourceType: "aws_vpc", Label: "main", Service: "VPC",
		Children: []string{"aws_instance.web"},
	})
	g.Nodes[0].Parent = "aws_vpc.main"

	g.AddEdge(&graph.Edge{Source: "aws_instance.web", Target: "aws_vpc.main", Type: "depends_on"})

	out, err := RenderDOT(g)
	if err != nil {
		t.Fatalf("RenderDOT failed: %v", err)
	}

	dot := string(out)

	if !strings.Contains(dot, "digraph infrastructure") {
		t.Error("expected 'digraph infrastructure' in DOT output")
	}
	if !strings.Contains(dot, "cluster_aws_vpc.main") {
		t.Error("expected subgraph cluster for VPC")
	}
	if !strings.Contains(dot, "aws_instance.web") {
		t.Error("expected instance node in DOT output")
	}
}

package output

import (
	"os"
	"testing"

	"github.com/vhco-pro/stackgraph/pkg/graph"
	"github.com/vhco-pro/stackgraph/pkg/mapping"
	"github.com/vhco-pro/stackgraph/pkg/parser"
)

// runPipeline parses a state fixture and runs the full processing pipeline,
// mirroring cmd/stackgraph/generate.go.
func runPipeline(t *testing.T, fixture string) *graph.Graph {
	t.Helper()
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	g, err := parser.ParseState(data)
	if err != nil {
		t.Fatalf("parse state: %v", err)
	}
	registry, err := mapping.LoadEmbedded()
	if err != nil {
		t.Fatalf("load mappings: %v", err)
	}
	g.ApplyMappings(registry)
	g.DetectImplicitEdges()
	g.ApplyGrouping()
	g.ApplyMappings(registry)
	g.CollapseCountInstances()
	g.FilterInternal()
	return g
}

// TestLayoutStructuralInvariants asserts, against real ELK layout output, the
// properties whose violation produced the original broken renders:
//  1. every child's bounding box lies inside its parent's bounding box
//  2. no group node renders as an empty box (a group with no children must not
//     be classified/rendered as a container)
//  3. every graph node received a layout position
func TestLayoutStructuralInvariants(t *testing.T) {
	fixtures := []string{
		"../../testdata/state/aws/three-tier.json",
		"../../testdata/state/aws/multi-service.json",
		"../../testdata/state/aws/count-foreach.json",
		"../../testdata/state/aws/two-vpc-realistic.json",
		"../../testdata/state/azure/multi-service.json",
		"../../testdata/state/gcp/multi-service.json",
	}

	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			g := runPipeline(t, fixture)

			elkGraph := buildElkGraph(g)
			positions, _, err := runElkLayout(elkGraph, g)
			if err != nil {
				t.Fatalf("ELK layout failed: %v", err)
			}

			// 3. every node has a position
			for _, n := range g.Nodes {
				if _, ok := positions[n.ID]; !ok {
					t.Errorf("node %s has no layout position", n.ID)
				}
			}

			// 1. child bbox inside parent bbox (with 1px tolerance)
			const tol = 1.0
			for _, n := range g.Nodes {
				if n.Parent == "" {
					continue
				}
				child, cok := positions[n.ID]
				parent, pok := positions[n.Parent]
				if !cok || !pok {
					continue
				}
				cl, cr := child.X-child.Width/2, child.X+child.Width/2
				ct, cb := child.Y-child.Height/2, child.Y+child.Height/2
				pl, pr := parent.X-parent.Width/2, parent.X+parent.Width/2
				pt, pb := parent.Y-parent.Height/2, parent.Y+parent.Height/2
				if cl < pl-tol || cr > pr+tol || ct < pt-tol || cb > pb+tol {
					t.Errorf("node %s bbox [%f,%f,%f,%f] escapes parent %s bbox [%f,%f,%f,%f]",
						n.ID, cl, ct, cr, cb, n.Parent, pl, pt, pr, pb)
				}
			}

			// 2. every rendered container has at least one child
			childCount := make(map[string]int)
			for _, n := range g.Nodes {
				if n.Parent != "" {
					childCount[n.Parent]++
				}
			}
			for _, n := range g.Nodes {
				if (n.Type == graph.NodeTypeGroup || len(n.Children) > 0) && childCount[n.ID] == 0 {
					t.Errorf("group node %s renders as an empty container", n.ID)
				}
			}
		})
	}
}

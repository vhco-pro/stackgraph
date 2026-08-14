package output

import (
	"os"
	"testing"

	"github.com/vhco-pro/stackgraph/pkg/graph"
	"github.com/vhco-pro/stackgraph/pkg/mapping"
	"github.com/vhco-pro/stackgraph/pkg/parser"
)

// runPipeline parses a state fixture and runs the exact processing pipeline
// the CLI uses — graph.Process — so the invariants validate real output.
func runPipeline(t *testing.T, fixture string) *graph.Graph {
	t.Helper()
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	registry, err := mapping.LoadEmbedded()
	if err != nil {
		t.Fatalf("load mappings: %v", err)
	}
	g, err := parser.ParseState(data, registry.RefAttrs()...)
	if err != nil {
		t.Fatalf("parse state: %v", err)
	}
	graph.Process(g, registry, graph.ProcessOptions{})
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

			elkGraph, _ := buildElkGraph(g)
			positions, _, err := runElkLayout(elkGraph)
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

// TestPipelineEdgeContract pins the edge semantics on the realistic two-VPC
// fixture, end to end through graph.Process — the properties the 2026-08
// review established:
//  1. evidence edges (typed references) are never deleted
//  2. --infer-flows is purely additive
//  3. flows never cross network scopes and are unambiguous at global scope
func TestPipelineEdgeContract(t *testing.T) {
	const fixture = "../../testdata/state/aws/two-vpc-realistic.json"

	load := func(inferFlows bool) *graph.Graph {
		t.Helper()
		data, err := os.ReadFile(fixture)
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		registry, err := mapping.LoadEmbedded()
		if err != nil {
			t.Fatalf("load mappings: %v", err)
		}
		g, err := parser.ParseState(data, registry.RefAttrs()...)
		if err != nil {
			t.Fatalf("parse state: %v", err)
		}
		graph.Process(g, registry, graph.ProcessOptions{
			InferFlows:   inferFlows,
			FlowPatterns: registry.FlowPatterns(),
		})
		return g
	}

	edgeSet := func(g *graph.Graph) map[string]string {
		set := make(map[string]string)
		for _, e := range g.Edges {
			set[e.Source+"->"+e.Target] = e.Type
		}
		return set
	}

	plain := edgeSet(load(false))
	flows := edgeSet(load(true))

	// 1. Evidence must be present in BOTH runs — enabling a heuristic must
	// never delete state-proven arrows.
	for _, evidence := range []string{
		"aws_lb.prod_alb->aws_security_group.prod_web",
		"aws_lb.staging_alb->aws_security_group.staging_web",
		"aws_instance.prod_web->aws_security_group.prod_web",
		"aws_route53_record.www->aws_route53_zone.main",
	} {
		if _, ok := plain[evidence]; !ok {
			t.Errorf("evidence edge missing without flows: %s", evidence)
		}
		if _, ok := flows[evidence]; !ok {
			t.Errorf("evidence edge deleted by --infer-flows: %s", evidence)
		}
	}

	// 2. Purely additive: every default-run edge survives with flows on.
	for pair := range plain {
		if _, ok := flows[pair]; !ok {
			t.Errorf("--infer-flows removed edge %s", pair)
		}
	}

	// 3. No cross-environment or ambiguous global-scope flows.
	for _, fabricated := range []string{
		"aws_ecs_service.api->aws_db_instance.prod_db",
		"aws_ecs_service.api->aws_db_instance.staging_db",
		"aws_lambda_function.worker->aws_s3_bucket.assets",
		"aws_instance.prod_web->aws_elasticache_cluster.staging_redis",
	} {
		if _, ok := flows[fabricated]; ok {
			t.Errorf("fabricated flow edge present: %s", fabricated)
		}
	}

	// Scoped flows that SHOULD exist (same VPC / unambiguous global target).
	for _, want := range []string{
		"aws_lb.prod_alb->aws_instance.prod_web",
		"aws_instance.staging_app->aws_elasticache_cluster.staging_redis",
		"aws_lambda_function.worker->aws_sqs_queue.jobs",
	} {
		if typ, ok := flows[want]; !ok || typ != "flow" {
			t.Errorf("expected flow edge %s (got type %q, present=%v)", want, typ, ok)
		}
	}

	if len(flows) != 16 || len(plain) != 5 {
		t.Errorf("edge counts drifted: plain=%d (want 5), flows=%d (want 16) — update deliberately if semantics changed",
			len(plain), len(flows))
	}
}

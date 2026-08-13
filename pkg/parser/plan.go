package parser

import (
	"encoding/json"
	"fmt"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/vhco-pro/stackgraph/pkg/graph"
)

// ParsePlan parses a terraform/tofu show -json plan output and returns a graph
// with action annotations (create/update/delete) on each node. extraKeepAttrs
// behaves as in ParseState.
func ParsePlan(data []byte, extraKeepAttrs ...string) (*graph.Graph, error) {
	var plan tfjson.Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("invalid plan JSON: %w", err)
	}

	g := graph.NewGraph("plan")

	// Build action map from resource changes
	actions := make(map[string]graph.Action)
	for _, rc := range plan.ResourceChanges {
		if rc.Change == nil || len(rc.Change.Actions) == 0 {
			continue
		}

		switch {
		case rc.Change.Actions.Replace():
			actions[rc.Address] = graph.ActionReplace
		case rc.Change.Actions.Create():
			actions[rc.Address] = graph.ActionCreate
		case rc.Change.Actions.Delete():
			actions[rc.Address] = graph.ActionDelete
		case rc.Change.Actions.Update():
			actions[rc.Address] = graph.ActionUpdate
		case rc.Change.Actions.NoOp() || rc.Change.Actions.Read():
			actions[rc.Address] = graph.ActionNoOp
		}
	}

	// Use planned values for the graph (same structure as state)
	if plan.PlannedValues != nil && plan.PlannedValues.RootModule != nil {
		providers := make(map[string]int)
		addModuleResources(g, plan.PlannedValues.RootModule, providers, extraKeepAttrs)

		if len(providers) > 0 {
			var topProvider string
			var topCount int
			for p, c := range providers {
				if c > topCount {
					topProvider = p
					topCount = c
				}
			}
			g.Metadata.Provider = topProvider
		}
	}

	// Resources being destroyed exist only in resource_changes (planned_values
	// excludes them) — add them from the change's "before" values so deletes
	// are visible in the diagram.
	for _, rc := range plan.ResourceChanges {
		if actions[rc.Address] != graph.ActionDelete || g.NodeByID(rc.Address) != nil {
			continue
		}
		var before map[string]any
		if rc.Change != nil {
			before, _ = rc.Change.Before.(map[string]any)
		}
		g.AddNode(&graph.Node{
			ID:           rc.Address,
			Type:         graph.NodeTypeResource,
			ResourceType: rc.Type,
			Label:        rc.Name,
			Provider:     extractProvider(rc.ProviderName),
			Attributes:   curateAttributes(rc.Type, before, extraKeepAttrs),
			Module:       rc.ModuleAddress,
		})
	}

	// Apply action annotations
	for _, n := range g.Nodes {
		if action, ok := actions[n.ID]; ok {
			n.Action = action
		}
	}

	return g, nil
}

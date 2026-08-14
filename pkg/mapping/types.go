// Package mapping provides the resource type to cloud service mapping system.
package mapping

import "github.com/vhco-pro/stackgraph/pkg/graph"

// ProviderMappings represents all resource mappings for a single provider.
type ProviderMappings struct {
	Resources   map[string]*ResourceMapping `yaml:"resources"`
	Flows       []FlowPattern               `yaml:"flows,omitempty"`
	Consolidate []ConsolidationRule         `yaml:"consolidate,omitempty"`
}

// FlowPattern declares a heuristic data-flow relationship between two resource
// types (used by --infer-flows, scoped to a single network container).
type FlowPattern struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// ResourceMapping maps a Terraform/OpenTofu resource type to its cloud service metadata.
type ResourceMapping struct {
	Service     string `yaml:"service"`
	Category    string `yaml:"category"`
	Icon        string `yaml:"icon"`
	IsGroup     bool   `yaml:"is_group,omitempty"`
	GroupLevel  int    `yaml:"group_level,omitempty"`
	GroupParent string `yaml:"group_parent,omitempty"`
	ParentRef   string `yaml:"parent_ref,omitempty"`
	// MemberRef names a list attribute on OTHER resources that references this
	// container's id (e.g. vpc_security_group_ids for aws_security_group):
	// resources listing this container become its children.
	MemberRef string `yaml:"member_ref,omitempty"`
	// NetworkScope marks this group as a network boundary (VPC, VNet, …) —
	// heuristic flow edges never cross it.
	NetworkScope bool             `yaml:"network_scope,omitempty"`
	Style        *graph.NodeStyle `yaml:"style,omitempty"`
	Variants     []VariantMapping `yaml:"variants,omitempty"`
}

// VariantMapping selects an alternative icon/service based on a resource attribute value.
type VariantMapping struct {
	Match   VariantMatch `yaml:"match"`
	Icon    string       `yaml:"icon,omitempty"`
	Service string       `yaml:"service,omitempty"`
}

// VariantMatch defines the condition for a variant.
type VariantMatch struct {
	Attribute string `yaml:"attribute"`
	Value     string `yaml:"value"`
}

// ConsolidationRule defines how multiple related resource types should be merged into one node.
type ConsolidationRule struct {
	Target  string   `yaml:"target"`
	Sources []string `yaml:"sources"`
	Label   string   `yaml:"label"`
}

// ListEntry is a flattened view of a mapping for the `mappings list` CLI command.
type ListEntry struct {
	ResourceType string
	Provider     string
	Service      string
	Category     string
	Icon         string
}

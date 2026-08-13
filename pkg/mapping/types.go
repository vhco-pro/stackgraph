// Package mapping provides the resource type to cloud service mapping system.
package mapping

// ProviderMappings represents all resource mappings for a single provider.
type ProviderMappings struct {
	Resources   map[string]*ResourceMapping `yaml:"resources"`
	Consolidate []ConsolidationRule         `yaml:"consolidate,omitempty"`
}

// ResourceMapping maps a Terraform/OpenTofu resource type to its cloud service metadata.
type ResourceMapping struct {
	Service     string           `yaml:"service"`
	Category    string           `yaml:"category"`
	Icon        string           `yaml:"icon"`
	IsGroup     bool             `yaml:"is_group,omitempty"`
	GroupLevel  int              `yaml:"group_level,omitempty"`
	GroupParent string           `yaml:"group_parent,omitempty"`
	ParentRef   string           `yaml:"parent_ref,omitempty"`
	Style       *StyleMapping    `yaml:"style,omitempty"`
	Variants    []VariantMapping `yaml:"variants,omitempty"`
}

// StyleMapping declares how a group/container renders. Colors are hex strings;
// role selects the dark-mode CSS class family (cloud, vpc, subnet, sg, default).
type StyleMapping struct {
	Fill   string `yaml:"fill,omitempty"`
	Stroke string `yaml:"stroke,omitempty"`
	Label  string `yaml:"label,omitempty"`
	Role   string `yaml:"role,omitempty"`
	Dashed bool   `yaml:"dashed,omitempty"`
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

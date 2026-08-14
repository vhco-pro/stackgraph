package mapping

import (
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/vhco-pro/stackgraph/pkg/graph"
	"gopkg.in/yaml.v3"
)

// Registry holds all loaded resource type mappings indexed by resource type.
type Registry struct {
	mappings map[string]*registryEntry
	flows    [][2]string // declared flow patterns: [from, to] resource types
}

type registryEntry struct {
	provider string
	mapping  *ResourceMapping
}

// LoadFromFS loads all mapping YAML files from the given filesystem at the given root.
func LoadFromFS(fsys fs.FS, dir string) (*Registry, error) {
	r := &Registry{
		mappings: make(map[string]*registryEntry),
	}

	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read mappings directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		data, err := fs.ReadFile(fsys, dir+"/"+entry.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to read mapping file %s: %w", entry.Name(), err)
		}

		var pm ProviderMappings
		if err := yaml.Unmarshal(data, &pm); err != nil {
			return nil, fmt.Errorf("failed to parse mapping file %s: %w", entry.Name(), err)
		}

		provider := strings.TrimSuffix(entry.Name(), ".yaml")

		for resourceType, m := range pm.Resources {
			r.mappings[resourceType] = &registryEntry{
				provider: provider,
				mapping:  m,
			}
		}
		for _, f := range pm.Flows {
			if f.From != "" && f.To != "" {
				r.flows = append(r.flows, [2]string{f.From, f.To})
			}
		}
	}

	return r, nil
}

// FlowPatterns returns the declared heuristic flow patterns as [from, to] pairs.
func (r *Registry) FlowPatterns() [][2]string {
	return r.flows
}

// LoadEmbedded loads all mapping YAML files from the embedded filesystem.
func LoadEmbedded() (*Registry, error) {
	return LoadFromFS(embeddedMappings, "mappings")
}

// LoadDir merges mapping YAML files from a directory on disk into the registry.
// Overlay entries PATCH embedded mappings with the same resource type: fields
// the overlay leaves empty inherit from the base entry, so restyling an icon
// cannot silently strip member_ref/parent_ref and break containment. Fields
// the overlay sets win.
func (r *Registry) LoadDir(dir string) error {
	overlay, err := LoadFromFS(os.DirFS(dir), ".")
	if err != nil {
		return fmt.Errorf("failed to load mapping overlay from %s: %w", dir, err)
	}
	for resourceType, entry := range overlay.mappings {
		if base, ok := r.mappings[resourceType]; ok {
			mergeMapping(entry.mapping, base.mapping)
		}
		r.mappings[resourceType] = entry
	}
	r.flows = append(r.flows, overlay.flows...)
	return nil
}

// mergeMapping fills m's empty fields from base (overlay-as-patch semantics).
func mergeMapping(m, base *ResourceMapping) {
	if m.Service == "" {
		m.Service = base.Service
	}
	if m.Category == "" {
		m.Category = base.Category
	}
	if m.Icon == "" {
		m.Icon = base.Icon
	}
	if !m.IsGroup {
		m.IsGroup = base.IsGroup
	}
	if m.GroupLevel == 0 {
		m.GroupLevel = base.GroupLevel
	}
	if m.GroupParent == "" {
		m.GroupParent = base.GroupParent
	}
	if m.ParentRef == "" {
		m.ParentRef = base.ParentRef
	}
	if m.MemberRef == "" {
		m.MemberRef = base.MemberRef
	}
	if !m.NetworkScope {
		m.NetworkScope = base.NetworkScope
	}
	if m.Style == nil {
		m.Style = base.Style
	}
	if len(m.Variants) == 0 {
		m.Variants = base.Variants
	}
}

// Lookup returns the mapping result for a given resource type, or nil if not found.
func (r *Registry) Lookup(resourceType string) *graph.MappingResult {
	entry, ok := r.mappings[resourceType]
	if !ok {
		return nil
	}

	result := &graph.MappingResult{
		Service:      entry.mapping.Service,
		Category:     entry.mapping.Category,
		Icon:         entry.mapping.Icon,
		IsGroup:      entry.mapping.IsGroup,
		GroupLevel:   entry.mapping.GroupLevel,
		GroupParent:  entry.mapping.GroupParent,
		ParentRef:    entry.mapping.ParentRef,
		MemberRef:    entry.mapping.MemberRef,
		NetworkScope: entry.mapping.NetworkScope,
	}
	// ApplyMappings deep-copies the style per node, so sharing the pointer is safe.
	result.Style = entry.mapping.Style

	for _, v := range entry.mapping.Variants {
		result.Variants = append(result.Variants, graph.MappingVariant{
			MatchAttribute: v.Match.Attribute,
			MatchValue:     v.Match.Value,
			Icon:           v.Icon,
			Service:        v.Service,
		})
	}

	return result
}

// RefAttrs returns every attribute name any mapping uses for containment —
// parent_ref and member_ref — so parsers keep those attributes through
// curation regardless of their shape.
func (r *Registry) RefAttrs() []string {
	seen := make(map[string]bool)
	var attrs []string
	add := func(ref string) {
		if ref != "" && !seen[ref] {
			seen[ref] = true
			attrs = append(attrs, ref)
		}
	}
	for _, entry := range r.mappings {
		add(entry.mapping.ParentRef)
		add(entry.mapping.MemberRef)
	}
	return attrs
}

// ParentRefAttrs is a deprecated alias for RefAttrs.
func (r *Registry) ParentRefAttrs() []string { return r.RefAttrs() }

// List returns all mappings matching the given filters.
func (r *Registry) List(provider, category string) []ListEntry {
	var result []ListEntry

	for resourceType, entry := range r.mappings {
		if provider != "" && !strings.EqualFold(entry.provider, provider) {
			continue
		}
		if category != "" && !strings.EqualFold(entry.mapping.Category, category) {
			continue
		}
		result = append(result, ListEntry{
			ResourceType: resourceType,
			Provider:     entry.provider,
			Service:      entry.mapping.Service,
			Category:     entry.mapping.Category,
			Icon:         entry.mapping.Icon,
		})
	}

	return result
}

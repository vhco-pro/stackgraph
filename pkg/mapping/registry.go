package mapping

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/michielvha/stackgraph/pkg/graph"
	"gopkg.in/yaml.v3"
)

// Registry holds all loaded resource type mappings indexed by resource type.
type Registry struct {
	mappings map[string]*registryEntry
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
	}

	return r, nil
}

// LoadEmbedded loads all mapping YAML files from the embedded filesystem.
func LoadEmbedded() (*Registry, error) {
	return LoadFromFS(embeddedMappings, "mappings")
}

// Lookup returns the mapping result for a given resource type, or nil if not found.
func (r *Registry) Lookup(resourceType string) *graph.MappingResult {
	entry, ok := r.mappings[resourceType]
	if !ok {
		return nil
	}

	result := &graph.MappingResult{
		Service:     entry.mapping.Service,
		Category:    entry.mapping.Category,
		Icon:        entry.mapping.Icon,
		IsGroup:     entry.mapping.IsGroup,
		GroupLevel:  entry.mapping.GroupLevel,
		GroupParent: entry.mapping.GroupParent,
	}

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

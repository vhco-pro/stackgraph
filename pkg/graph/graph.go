// Package graph provides the internal graph representation for infrastructure diagrams.
package graph

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// NodeType distinguishes between regular resources and group containers.
type NodeType string

const (
	NodeTypeResource NodeType = "resource"
	NodeTypeGroup    NodeType = "group"
	NodeTypeData     NodeType = "data"
)

// Action represents a planned change action for plan-mode visualization.
type Action string

const (
	ActionNone   Action = ""
	ActionCreate Action = "create"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
	ActionNoOp   Action = "no-op"
)

// Node represents a single resource or group in the infrastructure graph.
type Node struct {
	ID           string                 `json:"id"`
	Type         NodeType               `json:"type"`
	ResourceType string                 `json:"resource_type"`
	Label        string                 `json:"label"`
	Provider     string                 `json:"provider"`
	Service      string                 `json:"service,omitempty"`
	Category     string                 `json:"category,omitempty"`
	Icon         string                 `json:"icon,omitempty"`
	Parent       string                 `json:"parent,omitempty"`
	Children     []string               `json:"children,omitempty"`
	Attributes   map[string]interface{} `json:"attributes,omitempty"`
	Module       string                 `json:"module,omitempty"`
	Count        int                    `json:"count,omitempty"`
	Action       Action                 `json:"action,omitempty"`
	IsGroupNode  bool                   `json:"-"` // internal: determined by mapping
	GroupLevel   int                    `json:"-"` // internal: nesting depth
	GroupParent  string                 `json:"-"` // internal: expected parent resource type
}

// Edge represents a dependency or relationship between two nodes.
type Edge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type,omitempty"`
	Label  string `json:"label,omitempty"`
}

// Metadata holds information about the graph generation context.
type Metadata struct {
	Provider         string `json:"provider,omitempty"`
	ResourceCount    int    `json:"resource_count"`
	GeneratedAt      string `json:"generated_at,omitempty"`
	GeneratorVersion string `json:"generator_version"`
	InputMode        string `json:"input_mode"`
}

// Graph is the top-level container for the infrastructure diagram.
type Graph struct {
	Nodes    []*Node   `json:"nodes"`
	Edges    []*Edge   `json:"edges"`
	Metadata *Metadata `json:"metadata"`

	// internal indexes built lazily
	nodeIndex map[string]*Node
}

// NewGraph creates an empty graph with the given input mode.
func NewGraph(inputMode string) *Graph {
	return &Graph{
		Nodes: make([]*Node, 0),
		Edges: make([]*Edge, 0),
		Metadata: &Metadata{
			InputMode:        inputMode,
			GeneratorVersion: "0.1.0",
		},
	}
}

// AddNode adds a node to the graph.
func (g *Graph) AddNode(n *Node) {
	g.Nodes = append(g.Nodes, n)
	g.nodeIndex = nil // invalidate index
}

// AddEdge adds an edge to the graph.
func (g *Graph) AddEdge(e *Edge) {
	g.Edges = append(g.Edges, e)
}

// NodeByID returns the node with the given ID, or nil if not found.
func (g *Graph) NodeByID(id string) *Node {
	g.ensureIndex()
	return g.nodeIndex[id]
}

func (g *Graph) ensureIndex() {
	if g.nodeIndex != nil {
		return
	}
	g.nodeIndex = make(map[string]*Node, len(g.Nodes))
	for _, n := range g.Nodes {
		g.nodeIndex[n.ID] = n
	}
}

// ApplyMappings enriches nodes with service/category/icon metadata from the mapping registry.
func (g *Graph) ApplyMappings(registry interface{ Lookup(string) *MappingResult }) {
	for _, n := range g.Nodes {
		result := registry.Lookup(n.ResourceType)
		if result == nil {
			continue
		}
		n.Service = result.Service
		n.Category = result.Category
		n.Icon = result.Icon
		n.IsGroupNode = result.IsGroup
		n.GroupLevel = result.GroupLevel
		n.GroupParent = result.GroupParent

		if n.IsGroupNode {
			n.Type = NodeTypeGroup
		}

		// Check variants
		if len(result.Variants) > 0 && n.Attributes != nil {
			for _, v := range result.Variants {
				if val, ok := n.Attributes[v.MatchAttribute]; ok {
					valStr := fmt.Sprintf("%v", val)
					if strings.EqualFold(valStr, v.MatchValue) {
						if v.Icon != "" {
							n.Icon = v.Icon
						}
						if v.Service != "" {
							n.Service = v.Service
						}
						break
					}
				}
			}
		}
	}
}

// MappingResult is the result of looking up a resource type in the mapping registry.
type MappingResult struct {
	Service     string
	Category    string
	Icon        string
	IsGroup     bool
	GroupLevel  int
	GroupParent string
	Variants    []MappingVariant
}

// MappingVariant represents an alternative icon/service based on a resource attribute value.
type MappingVariant struct {
	MatchAttribute string
	MatchValue     string
	Icon           string
	Service        string
}

// countIndexRegex matches resource addresses with count/for_each indices like [0], ["key"].
var countIndexRegex = regexp.MustCompile(`^(.+)\[.+\]$`)

// CollapseCountInstances merges resources created via count/for_each into a single node with a count badge.
func (g *Graph) CollapseCountInstances() {
	// Group nodes by their base address (without index)
	baseGroups := make(map[string][]*Node)
	var nonIndexed []*Node

	for _, n := range g.Nodes {
		matches := countIndexRegex.FindStringSubmatch(n.ID)
		if matches != nil {
			base := matches[1]
			baseGroups[base] = append(baseGroups[base], n)
		} else {
			nonIndexed = append(nonIndexed, n)
		}
	}

	// Replace indexed groups with a single collapsed node
	collapsed := make(map[string]bool)
	for base, nodes := range baseGroups {
		if len(nodes) < 1 {
			continue
		}

		// Use the first node as the representative
		rep := *nodes[0]
		rep.ID = base
		rep.Label = nodes[0].Label
		// Remove index suffix from label if present
		if matches := countIndexRegex.FindStringSubmatch(rep.Label); matches != nil {
			rep.Label = matches[1]
		}
		rep.Count = len(nodes)
		nonIndexed = append(nonIndexed, &rep)

		for _, n := range nodes {
			collapsed[n.ID] = true
		}
	}

	g.Nodes = nonIndexed

	// Update edges: repoint collapsed node references to their base address
	var updatedEdges []*Edge
	for _, e := range g.Edges {
		src := e.Source
		tgt := e.Target
		if matches := countIndexRegex.FindStringSubmatch(src); matches != nil && collapsed[src] {
			src = matches[1]
		}
		if matches := countIndexRegex.FindStringSubmatch(tgt); matches != nil && collapsed[tgt] {
			tgt = matches[1]
		}
		// Deduplicate edges after collapsing
		updatedEdges = append(updatedEdges, &Edge{
			Source: src,
			Target: tgt,
			Type:   e.Type,
			Label:  e.Label,
		})
	}
	if updatedEdges == nil {
		updatedEdges = make([]*Edge, 0)
	}
	g.Edges = deduplicateEdges(updatedEdges)
	g.nodeIndex = nil // invalidate index
}

func deduplicateEdges(edges []*Edge) []*Edge {
	seen := make(map[string]bool)
	var result []*Edge
	for _, e := range edges {
		key := e.Source + "->" + e.Target + ":" + e.Type
		if !seen[key] {
			seen[key] = true
			result = append(result, e)
		}
	}
	return result
}

// FilterInternal removes internal/meta nodes that aren't useful for visualization
// (provider configurations, variable nodes, output nodes from DOT graphs).
func (g *Graph) FilterInternal() {
	var filtered []*Node
	removedIDs := make(map[string]bool)

	for _, n := range g.Nodes {
		// Filter out provider configuration nodes (handles escaped quotes from DOT parsing)
		if strings.Contains(n.ID, "provider[") || strings.Contains(n.ResourceType, "provider[") {
			removedIDs[n.ID] = true
			continue
		}
		// Filter out terraform internal nodes from DOT parsing
		if n.ID == "[root] root" || n.ID == "root" {
			removedIDs[n.ID] = true
			continue
		}
		filtered = append(filtered, n)
	}

	g.Nodes = filtered

	// Remove edges that reference removed nodes
	var filteredEdges []*Edge
	for _, e := range g.Edges {
		if !removedIDs[e.Source] && !removedIDs[e.Target] {
			filteredEdges = append(filteredEdges, e)
		}
	}
	g.Edges = filteredEdges
	g.nodeIndex = nil
}

// ToJSON serializes the graph to its JSON representation.
func (g *Graph) ToJSON() ([]byte, error) {
	g.Metadata.ResourceCount = len(g.Nodes)
	if g.Edges == nil {
		g.Edges = make([]*Edge, 0)
	}
	return json.MarshalIndent(g, "", "  ")
}

// Package graph provides the internal graph representation for infrastructure diagrams.
package graph

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
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
	ActionNone    Action = ""
	ActionCreate  Action = "create"
	ActionUpdate  Action = "update"
	ActionReplace Action = "replace"
	ActionDelete  Action = "delete"
	ActionNoOp    Action = "no-op"
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
	Style        *NodeStyle             `json:"style,omitempty"`
	Parent       string                 `json:"parent,omitempty"`
	Children     []string               `json:"children,omitempty"`
	Attributes   map[string]interface{} `json:"attributes,omitempty"`
	Module       string                 `json:"module,omitempty"`
	Count        int                    `json:"count,omitempty"`
	Action       Action                 `json:"action,omitempty"`
	IsGroupNode  bool                   `json:"-"` // internal: determined by mapping
	GroupLevel   int                    `json:"-"` // internal: nesting depth
	GroupParent  string                 `json:"-"` // internal: expected parent resource type
	ParentRef    string                 `json:"-"` // internal: attribute naming the containing resource
}

// NodeStyle carries container rendering colors resolved from the mapping registry.
// Role selects the dark-mode CSS class family (cloud, vpc, subnet, sg, default).
// The yaml tags let mapping files unmarshal directly into this type — one
// struct from YAML through the registry to the renderer and the JSON contract.
type NodeStyle struct {
	Fill   string `json:"fill,omitempty"   yaml:"fill,omitempty"`
	Stroke string `json:"stroke,omitempty" yaml:"stroke,omitempty"`
	Label  string `json:"label,omitempty"  yaml:"label,omitempty"`
	Role   string `json:"role,omitempty"   yaml:"role,omitempty"`
	Dashed bool   `json:"dashed,omitempty" yaml:"dashed,omitempty"`
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
		n.ParentRef = result.ParentRef
		if result.Style != nil {
			style := *result.Style
			n.Style = &style
		}

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
	ParentRef   string
	Style       *NodeStyle
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

	// Replace indexed groups with a single collapsed node. Bases are processed
	// in sorted order — map iteration order must never leak into node order,
	// or identical inputs produce different JSON/layout bytes run to run.
	bases := make([]string, 0, len(baseGroups))
	for base := range baseGroups {
		bases = append(bases, base)
	}
	sort.Strings(bases)

	// The member→base mapping is recorded so later reference rewriting is a
	// map lookup, not a second regex pass that could drift from baseGroups.
	collapsed := make(map[string]string) // member ID -> base ID
	for _, base := range bases {
		nodes := baseGroups[base]

		// Members spread across different parents (for_each over AZ subnets)
		// cannot be represented by one node in one container — collapsing
		// would draw instances in subnet B as if they lived in subnet A.
		// Keep such groups expanded.
		mixedParents := false
		for _, n := range nodes[1:] {
			if n.Parent != nodes[0].Parent {
				mixedParents = true
				break
			}
		}
		if mixedParents {
			nonIndexed = append(nonIndexed, nodes...)
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
		rep.Children = nil
		nonIndexed = append(nonIndexed, &rep)

		for _, n := range nodes {
			collapsed[n.ID] = base
		}
	}

	g.Nodes = nonIndexed
	g.nodeIndex = nil // invalidate index

	collapseRef := func(id string) string {
		if base, ok := collapsed[id]; ok {
			return base
		}
		return id
	}
	for _, n := range g.Nodes {
		n.Parent = collapseRef(n.Parent)
	}

	// Rebuild every Children list from the (rewritten) Parent pointers —
	// the parent pointer is canonical, and this restores the invariant for
	// collapsed representatives (whose copied Children were cleared) and for
	// containers that held collapsed members.
	for _, n := range g.Nodes {
		n.Children = nil
	}
	g.ensureIndex()
	for _, n := range g.Nodes {
		if n.Parent == "" {
			continue
		}
		if p := g.nodeIndex[n.Parent]; p != nil {
			p.Children = append(p.Children, n.ID)
		}
	}

	// Update edges: repoint collapsed node references to their base address,
	// dropping self-edges created by merging members of the same base.
	var updatedEdges []*Edge
	for _, e := range g.Edges {
		src := collapseRef(e.Source)
		tgt := collapseRef(e.Target)
		if src == tgt {
			continue
		}
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
}

// deduplicateEdges keeps one edge per source→target pair. The key deliberately
// ignores edge type: a depends_on edge plus a subnet_id edge between the same
// two nodes would otherwise draw as two overlapping arrows. When duplicates
// carry different labels, the labeled (more informative) edge wins so the
// survivor doesn't depend on slice order.
func deduplicateEdges(edges []*Edge) []*Edge {
	seen := make(map[string]*Edge)
	var result []*Edge
	for _, e := range edges {
		key := e.Source + "->" + e.Target
		if kept, ok := seen[key]; ok {
			if kept.Label == "" && e.Label != "" {
				kept.Label = e.Label
				kept.Type = e.Type
			}
			continue
		}
		seen[key] = e
		result = append(result, e)
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

// ProcessOptions configures the standard processing pipeline.
type ProcessOptions struct {
	// InferFlows adds heuristic data-flow edges (ALB→EC2, …) scoped to a
	// single network container. Off by default: these edges are guesses.
	InferFlows bool
}

// Process runs the standard pipeline every consumer (CLI, library, tests)
// should use. The ordering is load-bearing: reference edges must exist before
// grouping so the edge-based containment fallback works, and mappings are
// applied a second time so the cloud-boundary nodes grouping synthesizes get
// their icon and style.
func Process(g *Graph, registry interface{ Lookup(string) *MappingResult }, opts ProcessOptions) {
	g.ApplyMappings(registry)
	g.DetectImplicitEdges()
	g.ApplyGrouping()
	g.ApplyMappings(registry)
	if opts.InferFlows {
		g.InferFlowEdges()
	}
	g.CollapseCountInstances()
	g.FilterInternal()
}

// FilterDanglingEdges removes edges whose source or target node doesn't exist.
func (g *Graph) FilterDanglingEdges() {
	g.ensureIndex()
	var kept []*Edge
	for _, e := range g.Edges {
		if g.nodeIndex[e.Source] != nil && g.nodeIndex[e.Target] != nil {
			kept = append(kept, e)
		}
	}
	if kept == nil {
		kept = make([]*Edge, 0)
	}
	g.Edges = kept
}

// ToJSON serializes the graph to its JSON representation.
func (g *Graph) ToJSON() ([]byte, error) {
	g.Metadata.ResourceCount = len(g.Nodes)
	if g.Edges == nil {
		g.Edges = make([]*Edge, 0)
	}
	return json.MarshalIndent(g, "", "  ")
}

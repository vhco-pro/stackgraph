package graph

import (
	"sort"
	"strings"
)

// ApplyGrouping assigns parent-child relationships between group nodes (VPC, subnet, etc.)
// and their contained resources. Containment is resolved primarily from declarative
// parent_ref attribute lookups (deterministic, works without depends_on), with
// edge-based inference as a fallback for parsers that produce no attributes (DOT, HCL).
// Callers must run DetectImplicitEdges BEFORE this so the edge fallback sees real edges.
func (g *Graph) ApplyGrouping() {
	g.ensureIndex()

	// Sort group nodes by level (outermost first) so nesting is applied top-down
	var groupNodes []*Node
	for _, n := range g.Nodes {
		if n.IsGroupNode {
			groupNodes = append(groupNodes, n)
		}
	}
	sort.Slice(groupNodes, func(i, j int) bool {
		return groupNodes[i].GroupLevel < groupNodes[j].GroupLevel
	})

	refIndex := g.buildRefIndex()

	// Phase 1: parent_ref containment — group nodes first (outermost first,
	// so children of nested groups resolve against already-placed parents),
	// then regular resources.
	for _, gn := range groupNodes {
		g.assignParentByRef(gn, refIndex)
	}
	for _, n := range g.Nodes {
		if !n.IsGroupNode {
			g.assignParentByRef(n, refIndex)
		}
	}

	// Phase 2 (fallback): edge-based nesting for nodes parent_ref could not place.
	for _, gn := range groupNodes {
		if gn.GroupParent == "" || gn.Parent != "" {
			continue
		}

		// Find a group node of the expected parent type that has an edge to this group
		for _, candidate := range groupNodes {
			if candidate.ResourceType != gn.GroupParent || candidate.ID == gn.ID {
				continue
			}

			if g.hasEdgeBetween(gn.ID, candidate.ID) {
				g.setParent(gn, candidate)
				break
			}
		}
	}
	for _, n := range g.Nodes {
		if n.IsGroupNode || n.Parent != "" {
			continue
		}

		bestGroup := g.findBestGroup(n, groupNodes)
		if bestGroup != nil {
			g.setParent(n, bestGroup)
		}
	}

	// Phase 3: Resolve nested parents — push resources to most specific container
	g.resolveNestedParents()

	// Phase 4: member-list containment (security groups and any container
	// declaring member_ref). Runs AFTER subnet assignment so the container
	// inherits the member's subnet context.
	g.assignMemberContainment()

	// Phase 5: Drop edges that merely restate containment — nesting already
	// shows them, and drawn arrows into container walls read as noise.
	g.suppressContainmentEdges()

	// Phase 6: Add cloud provider boundary around all top-level provider resources
	g.addCloudBoundary()
}

// refIndex resolves attribute reference values to nodes: by "id"/"arn" globally
// (those are unique), and by (resource type, "name") for name-typed references
// like azurerm's resource_group_name, which are only unique within a type.
type refIndex struct {
	byID   map[string]*Node
	byName map[string]*Node // key: resourceType + "\x00" + name
}

func (g *Graph) buildRefIndex() *refIndex {
	idx := &refIndex{
		byID:   make(map[string]*Node),
		byName: make(map[string]*Node),
	}
	for _, n := range g.Nodes {
		if n.Attributes == nil {
			continue
		}
		for _, key := range []string{"id", "arn", "self_link"} {
			if v, ok := n.Attributes[key].(string); ok && v != "" {
				idx.byID[v] = n
			}
		}
		if name, ok := n.Attributes["name"].(string); ok && name != "" {
			key := n.ResourceType + "\x00" + name
			// Two same-type nodes sharing a name (e.g. equally named resource
			// groups in different subscriptions) make the name ambiguous —
			// record the collision so nothing nests under the wrong namesake.
			if _, exists := idx.byName[key]; exists {
				idx.byName[key] = nil
			} else {
				idx.byName[key] = n
			}
		}
	}
	return idx
}

// assignParentByRef places a node inside the container its parent_ref attribute
// points at. Reference values may be a scalar string or a list (first resolvable
// entry wins, e.g. aws_lb.subnets). Matches resolve by id/arn first; a name match
// is only accepted for the mapping's declared group_parent type.
func (g *Graph) assignParentByRef(n *Node, idx *refIndex) {
	if n.Parent != "" || n.ParentRef == "" || n.Attributes == nil {
		return
	}

	raw, ok := n.Attributes[n.ParentRef]
	if !ok {
		return
	}

	for _, ref := range attrStringValues(raw) {
		if ref == "" {
			continue
		}
		target := idx.byID[ref]
		if target == nil && n.GroupParent != "" {
			target = idx.byName[n.GroupParent+"\x00"+ref]
		}
		if target == nil || target.ID == n.ID || !target.IsGroupNode {
			continue
		}
		g.setParent(n, target)
		return
	}
}

// attrStringValues extracts the string values a reference-shaped attribute can
// carry: a scalar string or a list of strings. Both containment resolution and
// implicit edge detection go through this one helper so they never drift.
func attrStringValues(raw interface{}) []string {
	switch v := raw.(type) {
	case string:
		return []string{v}
	case []interface{}:
		var values []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				values = append(values, s)
			}
		}
		return values
	}
	return nil
}

// setParent links child under parent, guarding against containment cycles.
func (g *Graph) setParent(child, parent *Node) {
	for anc, hops := parent, 0; anc != nil && hops < len(g.Nodes)+1; hops++ {
		if anc.ID == child.ID {
			return // would create a cycle
		}
		anc = g.NodeByID(anc.Parent)
	}
	child.Parent = parent.ID
	parent.Children = appendUnique(parent.Children, child.ID)
}

// suppressContainmentEdges removes edges whose relationship is already expressed
// by nesting: an edge between a node and one of its ancestor containers.
func (g *Graph) suppressContainmentEdges() {
	g.ensureIndex()

	isAncestor := func(anc, node *Node) bool {
		for cur, hops := node, 0; cur != nil && hops < len(g.Nodes)+1; hops++ {
			if cur.Parent == "" {
				return false
			}
			if cur.Parent == anc.ID {
				return true
			}
			cur = g.NodeByID(cur.Parent)
		}
		return false
	}

	var kept []*Edge
	for _, e := range g.Edges {
		src := g.NodeByID(e.Source)
		tgt := g.NodeByID(e.Target)
		if src != nil && tgt != nil &&
			((tgt.IsGroupNode && isAncestor(tgt, src)) || (src.IsGroupNode && isAncestor(src, tgt))) {
			continue
		}
		kept = append(kept, e)
	}
	if kept == nil {
		kept = make([]*Edge, 0)
	}
	g.Edges = kept
}

// InferFlowEdges adds synthetic data-flow edges between resources based on
// common architectural patterns (ALB → EC2, EC2 → RDS, …). These edges are
// heuristic, not evidence from the state, so this is opt-in (--infer-flows)
// and scoped: a pair only connects when both nodes live under the same
// network container (VPC/VNet), or both live outside any network container.
// Without that scope a prod instance would get a drawn edge to a staging
// database purely because the types match.
// Patterns come from the mapping registry's flows: sections, so providers
// added purely via --mappings can declare their own.
func (g *Graph) InferFlowEdges(patterns [][2]string) {
	g.ensureIndex()

	// Collect resources by type for pairing
	byType := make(map[string][]*Node)
	for _, n := range g.Nodes {
		if n.Type == NodeTypeResource || n.Type == NodeTypeData {
			byType[n.ResourceType] = append(byType[n.ResourceType], n)
		}
	}

	existingEdges := make(map[string]bool)
	for _, e := range g.Edges {
		existingEdges[e.Source+"->"+e.Target] = true
		existingEdges[e.Target+"->"+e.Source] = true
	}

	for _, p := range patterns {
		srcs := byType[p[0]]
		tgts := byType[p[1]]
		if len(srcs) == 0 || len(tgts) == 0 {
			continue
		}

		for _, src := range srcs {
			srcAncestor := g.networkAncestor(src)
			for _, tgt := range tgts {
				if srcAncestor != g.networkAncestor(tgt) {
					continue
				}
				key := src.ID + "->" + tgt.ID
				if existingEdges[key] {
					continue
				}
				g.AddEdge(&Edge{
					Source: src.ID,
					Target: tgt.ID,
					Type:   "flow",
				})
				existingEdges[key] = true
				existingEdges[tgt.ID+"->"+src.ID] = true
			}
		}
	}
}

// networkAncestor returns the ID of the network container (VPC, VNet, …)
// enclosing the node, or "" when the node lives outside one. Network scope is
// declared in mappings (network_scope: true); when no ancestor declares it,
// group level 1 is the fallback so undeclared hierarchies keep working.
func (g *Graph) networkAncestor(n *Node) string {
	fallback := ""
	for cur, hops := n, 0; cur != nil && hops < len(g.Nodes)+1; hops++ {
		if cur.IsGroupNode {
			if cur.NetworkScope {
				return cur.ID
			}
			if cur.GroupLevel == 1 && fallback == "" {
				fallback = cur.ID
			}
		}
		cur = g.NodeByID(cur.Parent)
	}
	return fallback
}

// assignMemberContainment implements member-list containment: a container type
// declares member_ref (e.g. aws_security_group: vpc_security_group_ids), and
// any resource whose member_ref attribute lists the container's id becomes its
// child. The container itself moves into the member's parent (subnet) so the
// SG → Subnet → VPC hierarchy holds. Runs AFTER parent_ref assignment so
// member parents are known.
func (g *Graph) assignMemberContainment() {
	g.ensureIndex()

	// container type -> member attribute; container id value -> container node
	memberAttrByType := make(map[string]string)
	containerByID := make(map[string]*Node)
	for _, n := range g.Nodes {
		if !n.IsGroupNode || n.MemberRef == "" {
			continue
		}
		memberAttrByType[n.ResourceType] = n.MemberRef
		if n.Attributes != nil {
			if id, ok := n.Attributes["id"].(string); ok && id != "" {
				containerByID[id] = n
			}
		}
	}

	if len(containerByID) == 0 {
		return
	}

	// Iterate member attributes in sorted order for deterministic placement
	memberAttrs := make([]string, 0, len(memberAttrByType))
	seenAttr := make(map[string]bool)
	for _, attr := range memberAttrByType {
		if !seenAttr[attr] {
			seenAttr[attr] = true
			memberAttrs = append(memberAttrs, attr)
		}
	}
	sort.Strings(memberAttrs)

	for _, n := range g.Nodes {
		if n.IsGroupNode || n.Attributes == nil {
			continue
		}

		placed := false
		for _, attr := range memberAttrs {
			if placed {
				break
			}
			raw, ok := n.Attributes[attr]
			if !ok {
				continue
			}

			for _, idStr := range attrStringValues(raw) {
				container, found := containerByID[idStr]
				if !found {
					continue
				}

				// Move the container into the same parent as the member it
				// contains, when the member's parent is more specific (or the
				// container isn't placed anywhere yet).
				memberParent := n.Parent
				if memberParent != "" && memberParent != container.ID {
					containerParent := g.NodeByID(container.Parent)
					memberParentNode := g.NodeByID(memberParent)

					if memberParentNode != nil &&
						(containerParent == nil || memberParentNode.GroupLevel > containerParent.GroupLevel) {
						if containerParent != nil {
							containerParent.Children = removeString(containerParent.Children, container.ID)
						}
						container.Parent = ""
						g.setParent(container, memberParentNode)
					}
				}

				// Re-home the member inside the container
				if oldParent := g.NodeByID(n.Parent); oldParent != nil {
					oldParent.Children = removeString(oldParent.Children, n.ID)
				}
				n.Parent = ""
				g.setParent(n, container)
				placed = true
				break // use first matching container
			}
		}
	}
}

// addCloudBoundary adds a synthetic provider boundary node (e.g., "AWS Cloud")
// wrapping all top-level resources of the same provider. The boundary's
// resource type is "<provider>_cloud" (aws→aws_cloud, azurerm→azure_cloud,
// google→gcp_cloud, anything else literal) — a matching mapping entry supplies
// its label, icon, and style via the post-grouping ApplyMappings pass, so any
// provider added purely through mapping YAML gets a styled boundary too.
func (g *Graph) addCloudBoundary() {
	// Collect providers among top-level nodes, in deterministic order
	seen := make(map[string]bool)
	var providers []string
	for _, n := range g.Nodes {
		if n.Parent == "" && n.Provider != "" && !seen[n.Provider] {
			seen[n.Provider] = true
			providers = append(providers, n.Provider)
		}
	}
	sort.Strings(providers)

	for _, provider := range providers {
		boundaryType := provider + "_cloud"
		boundaryLabel := strings.ToUpper(provider[:1]) + provider[1:]
		switch provider {
		case "aws":
			boundaryLabel = "AWS Cloud"
		case "azurerm":
			boundaryType = "azure_cloud"
			boundaryLabel = "Azure"
		case "google":
			boundaryType = "gcp_cloud"
			boundaryLabel = "Google Cloud"
		}

		boundaryID := boundaryType + ".boundary"
		boundary := &Node{
			ID:           boundaryID,
			Type:         NodeTypeGroup,
			ResourceType: boundaryType,
			Label:        boundaryLabel,
			Provider:     provider,
			IsGroupNode:  true,
			GroupLevel:   0, // outermost
		}
		g.AddNode(boundary)

		// Re-parent all top-level nodes of this provider under the boundary
		for _, n := range g.Nodes {
			if n.ID == boundaryID {
				continue
			}
			if n.Parent == "" && n.Provider == provider {
				n.Parent = boundaryID
				boundary.Children = appendUnique(boundary.Children, n.ID)
			}
		}
	}
}

// findBestGroup finds the most specific (highest GroupLevel) group node
// that is connected to the given node via an edge.
func (g *Graph) findBestGroup(n *Node, groupNodes []*Node) *Node {
	var best *Node

	for _, gn := range groupNodes {
		if !g.hasEdgeBetween(n.ID, gn.ID) {
			continue
		}

		// Check if this group's resource type matches the node's group_parent
		if n.GroupParent != "" && gn.ResourceType != n.GroupParent {
			continue
		}

		if best == nil || gn.GroupLevel > best.GroupLevel {
			best = gn
		}
	}

	return best
}

// hasEdgeBetween checks if there's an edge between two nodes in either direction.
func (g *Graph) hasEdgeBetween(a, b string) bool {
	for _, e := range g.Edges {
		if (e.Source == a && e.Target == b) || (e.Source == b && e.Target == a) {
			return true
		}
	}
	return false
}

// resolveNestedParents ensures that resources are parented to the most specific
// group container (e.g., instance inside subnet, not directly inside VPC).
func (g *Graph) resolveNestedParents() {
	g.ensureIndex()

	for _, n := range g.Nodes {
		if n.Parent == "" || n.IsGroupNode {
			continue
		}

		parent := g.NodeByID(n.Parent)
		if parent == nil {
			continue
		}

		// Check if any of the parent's child groups also has an edge to this node
		for _, childID := range parent.Children {
			child := g.NodeByID(childID)
			if child == nil || !child.IsGroupNode || child.ID == n.ID {
				continue
			}

			if g.hasEdgeBetween(n.ID, child.ID) && child.GroupLevel > parent.GroupLevel {
				// Move to the more specific group
				parent.Children = removeString(parent.Children, n.ID)
				n.Parent = child.ID
				child.Children = appendUnique(child.Children, n.ID)
				break
			}
		}
	}
}

// DetectImplicitEdges scans resource attribute values for references to other resource IDs,
// creating edges for relationships not captured by explicit depends_on.
func (g *Graph) DetectImplicitEdges() {
	g.ensureIndex()

	// Build index: resource attribute "id" value -> node ID
	idIndex := make(map[string]string)
	for _, n := range g.Nodes {
		if n.Attributes == nil {
			continue
		}
		if id, ok := n.Attributes["id"]; ok {
			idStr, isStr := id.(string)
			if isStr && idStr != "" {
				idIndex[idStr] = n.ID
			}
		}
	}

	// Build set of existing edges for dedup
	existingEdges := make(map[string]bool)
	for _, e := range g.Edges {
		existingEdges[e.Source+"->"+e.Target] = true
		existingEdges[e.Target+"->"+e.Source] = true
	}

	// Scan all attributes for references to known IDs. Attribute names are
	// visited in sorted order so the surviving edge's type/label is stable
	// when two attributes reference the same target (maps iterate randomly).
	for _, n := range g.Nodes {
		if n.Attributes == nil {
			continue
		}
		attrNames := make([]string, 0, len(n.Attributes))
		for attrName := range n.Attributes {
			if attrName != "id" { // skip the node's own "id" attribute
				attrNames = append(attrNames, attrName)
			}
		}
		sort.Strings(attrNames)

		for _, attrName := range attrNames {
			// References appear as scalar strings or lists of strings
			// (e.g. subnets, security_groups, route53 records).
			for _, valStr := range attrStringValues(n.Attributes[attrName]) {
				if valStr == "" {
					continue
				}

				targetNodeID, found := idIndex[valStr]
				if !found || targetNodeID == n.ID {
					continue
				}

				edgeKey := n.ID + "->" + targetNodeID
				if existingEdges[edgeKey] {
					continue
				}

				g.AddEdge(&Edge{
					Source: n.ID,
					Target: targetNodeID,
					Type:   attrName,
					Label:  formatEdgeLabel(attrName),
				})
				existingEdges[edgeKey] = true
				existingEdges[targetNodeID+"->"+n.ID] = true
			}
		}
	}
}

// formatEdgeLabel creates a human-readable label from an attribute name.
func formatEdgeLabel(attrName string) string {
	label := strings.TrimSuffix(attrName, "_id")
	label = strings.TrimSuffix(label, "_ids")
	return strings.ReplaceAll(label, "_", " ")
}

func appendUnique(slice []string, s string) []string {
	for _, existing := range slice {
		if existing == s {
			return slice
		}
	}
	return append(slice, s)
}

func removeString(slice []string, s string) []string {
	var result []string
	for _, existing := range slice {
		if existing != s {
			result = append(result, existing)
		}
	}
	return result
}

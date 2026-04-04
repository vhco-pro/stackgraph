package graph

import (
	"sort"
	"strings"
)

// ApplyGrouping assigns parent-child relationships between group nodes (VPC, subnet, etc.)
// and their contained resources based on mapping metadata, implicit edge detection,
// and attribute-based containment (e.g., security group membership).
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

	// Phase 1: Nest group nodes inside each other based on group_parent mapping
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
				gn.Parent = candidate.ID
				candidate.Children = appendUnique(candidate.Children, gn.ID)
				break
			}
		}
	}

	// Phase 2: For non-group resources, find closest group parent via edges
	for _, n := range g.Nodes {
		if n.IsGroupNode || n.Parent != "" {
			continue
		}

		bestGroup := g.findBestGroup(n, groupNodes)
		if bestGroup != nil {
			n.Parent = bestGroup.ID
			bestGroup.Children = appendUnique(bestGroup.Children, n.ID)
		}
	}

	// Phase 3: Resolve nested parents — push resources to most specific container
	g.resolveNestedParents()

	// Phase 4: Assign resources to security groups via attribute matching.
	// This runs AFTER subnet assignment so SG inherits the subnet context.
	// If a resource has vpc_security_group_ids referencing an SG, move the SG
	// to the same parent as the resource and put the resource inside the SG.
	g.assignSecurityGroupContainment()

	// Phase 5: Infer data flow edges based on resource type relationships.
	// Terraform depends_on shows build dependencies, not runtime data flow.
	// We add synthetic edges for common patterns (ALB → EC2, EC2 → RDS, etc.)
	g.inferFlowEdges()

	// Phase 6: Add cloud provider boundary around all top-level provider resources
	g.addCloudBoundary()
}

// inferFlowEdges adds synthetic data-flow edges between resources based on
// common architectural patterns. These are the edges that make diagrams useful.
func (g *Graph) inferFlowEdges() {
	g.ensureIndex()

	// Collect resources by type for pairing
	byType := make(map[string][]*Node)
	for _, n := range g.Nodes {
		if n.Type == NodeTypeResource || n.Type == NodeTypeData {
			byType[n.ResourceType] = append(byType[n.ResourceType], n)
		}
	}

	// Common flow patterns: source type → target type
	// Direction represents data/traffic flow, not dependency
	patterns := []struct {
		srcType string
		tgtType string
		label   string
	}{
		// Web tier flows
		{"aws_lb", "aws_instance", ""},
		{"aws_lb", "aws_ecs_service", ""},
		{"aws_cloudfront_distribution", "aws_lb", ""},
		{"aws_cloudfront_distribution", "aws_s3_bucket", ""},
		{"aws_route53_record", "aws_lb", ""},
		{"aws_route53_record", "aws_cloudfront_distribution", ""},
		{"aws_api_gateway_rest_api", "aws_lambda_function", ""},
		{"aws_apigatewayv2_api", "aws_lambda_function", ""},

		// App → Database flows
		{"aws_instance", "aws_db_instance", ""},
		{"aws_instance", "aws_rds_cluster", ""},
		{"aws_instance", "aws_dynamodb_table", ""},
		{"aws_instance", "aws_elasticache_cluster", ""},
		{"aws_ecs_service", "aws_db_instance", ""},
		{"aws_ecs_service", "aws_rds_cluster", ""},
		{"aws_lambda_function", "aws_dynamodb_table", ""},
		{"aws_lambda_function", "aws_db_instance", ""},

		// Storage flows
		{"aws_instance", "aws_s3_bucket", ""},
		{"aws_lambda_function", "aws_s3_bucket", ""},

		// Messaging flows
		{"aws_lambda_function", "aws_sqs_queue", ""},
		{"aws_lambda_function", "aws_sns_topic", ""},
		{"aws_instance", "aws_sqs_queue", ""},

		// Azure flows
		{"azurerm_linux_virtual_machine", "azurerm_mssql_server", ""},
		{"azurerm_linux_virtual_machine", "azurerm_mssql_database", ""},
		{"azurerm_linux_virtual_machine", "azurerm_storage_account", ""},
		{"azurerm_windows_virtual_machine", "azurerm_mssql_server", ""},
		{"azurerm_virtual_machine", "azurerm_mssql_server", ""},
		{"azurerm_virtual_machine", "azurerm_storage_account", ""},
		{"azurerm_kubernetes_cluster", "azurerm_mssql_server", ""},
		{"azurerm_kubernetes_cluster", "azurerm_storage_account", ""},
		{"azurerm_kubernetes_cluster", "azurerm_cosmosdb_account", ""},
		{"azurerm_application_gateway", "azurerm_linux_virtual_machine", ""},
		{"azurerm_application_gateway", "azurerm_kubernetes_cluster", ""},

		// GCP flows
		{"google_compute_instance", "google_sql_database_instance", ""},
		{"google_compute_instance", "google_storage_bucket", ""},
		{"google_container_cluster", "google_sql_database_instance", ""},
		{"google_container_cluster", "google_storage_bucket", ""},
		{"google_cloudfunctions_function", "google_sql_database_instance", ""},
		{"google_cloudfunctions_function", "google_storage_bucket", ""},
		{"google_cloudfunctions2_function", "google_sql_database_instance", ""},
		{"google_cloudfunctions2_function", "google_storage_bucket", ""},
	}

	existingEdges := make(map[string]bool)
	for _, e := range g.Edges {
		existingEdges[e.Source+"->"+e.Target] = true
		existingEdges[e.Target+"->"+e.Source] = true
	}

	for _, p := range patterns {
		srcs := byType[p.srcType]
		tgts := byType[p.tgtType]
		if len(srcs) == 0 || len(tgts) == 0 {
			continue
		}

		for _, src := range srcs {
			for _, tgt := range tgts {
				key := src.ID + "->" + tgt.ID
				if existingEdges[key] {
					continue
				}
				g.AddEdge(&Edge{
					Source: src.ID,
					Target: tgt.ID,
					Type:   "flow",
					Label:  p.label,
				})
				existingEdges[key] = true
				existingEdges[tgt.ID+"->"+src.ID] = true
			}
		}
	}
}

// assignSecurityGroupContainment moves resources inside their security groups.
// Runs AFTER subnet assignment so we know where resources are placed.
// The SG is moved to the same subnet as the resources it contains.
func (g *Graph) assignSecurityGroupContainment() {
	g.ensureIndex()

	// Build SG ID value -> SG node index
	sgByID := make(map[string]*Node)
	for _, n := range g.Nodes {
		if n.ResourceType != "aws_security_group" {
			continue
		}
		if n.Attributes != nil {
			if sgID, ok := n.Attributes["id"].(string); ok {
				sgByID[sgID] = n
			}
		}
	}

	if len(sgByID) == 0 {
		return
	}

	// For each resource with vpc_security_group_ids, move it inside the SG.
	// Also move the SG to the resource's current parent (subnet) if not already placed.
	for _, n := range g.Nodes {
		if n.IsGroupNode {
			continue
		}
		if n.Attributes == nil {
			continue
		}

		sgIDs, ok := n.Attributes["vpc_security_group_ids"]
		if !ok {
			continue
		}

		sgList, ok := sgIDs.([]interface{})
		if !ok {
			continue
		}

		for _, sgIDRaw := range sgList {
			sgIDStr, ok := sgIDRaw.(string)
			if !ok {
				continue
			}
			sgNode, found := sgByID[sgIDStr]
			if !found {
				continue
			}

			// Move the SG into the same subnet as the resource it contains.
			// This ensures SG → Subnet → VPC nesting hierarchy.
			resourceParent := n.Parent
			if resourceParent != "" && resourceParent != sgNode.ID {
				sgCurrentParent := g.NodeByID(sgNode.Parent)
				resourceParentNode := g.NodeByID(resourceParent)

				// Only move if the resource's parent is more specific (deeper) than the SG's
				if resourceParentNode != nil && sgCurrentParent != nil &&
					resourceParentNode.GroupLevel > sgCurrentParent.GroupLevel {
					// Remove SG from its current parent
					sgCurrentParent.Children = removeString(sgCurrentParent.Children, sgNode.ID)
					// Place SG in the resource's parent (subnet)
					sgNode.Parent = resourceParent
					resourceParentNode.Children = appendUnique(resourceParentNode.Children, sgNode.ID)
				}
			}

			// Remove resource from its current parent's children list
			if oldParent := g.NodeByID(n.Parent); oldParent != nil {
				oldParent.Children = removeString(oldParent.Children, n.ID)
			}

			// Place resource inside the SG
			n.Parent = sgNode.ID
			sgNode.Children = appendUnique(sgNode.Children, n.ID)
			break // use first matching SG
		}
	}
}

// addCloudBoundary adds a synthetic cloud provider boundary node (e.g., "AWS Cloud")
// wrapping all top-level resources of the same provider.
func (g *Graph) addCloudBoundary() {
	// Count providers among top-level nodes
	providerCounts := make(map[string]int)
	for _, n := range g.Nodes {
		if n.Parent == "" {
			providerCounts[n.Provider]++
		}
	}

	for provider, count := range providerCounts {
		if count < 1 || provider == "" {
			continue
		}

		// Create cloud boundary node
		boundaryType := ""
		boundaryLabel := ""
		switch provider {
		case "aws":
			boundaryType = "aws_cloud"
			boundaryLabel = "AWS Cloud"
		case "azurerm":
			boundaryType = "azure_cloud"
			boundaryLabel = "Azure"
		case "google":
			boundaryType = "gcp_cloud"
			boundaryLabel = "Google Cloud"
		default:
			continue // only add boundaries for known cloud providers
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

	// Scan all attributes for references to known IDs
	for _, n := range g.Nodes {
		if n.Attributes == nil {
			continue
		}
		for attrName, attrValue := range n.Attributes {
			// Skip the node's own "id" attribute
			if attrName == "id" {
				continue
			}

			valStr, ok := attrValue.(string)
			if !ok || valStr == "" {
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

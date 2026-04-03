// Package parser provides input parsers for various infrastructure-as-code formats.
package parser

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/michielvha/stackgraph/pkg/graph"

	tfjson "github.com/hashicorp/terraform-json"
)

// ParseState parses a terraform/tofu show -json state output and returns a graph.
func ParseState(data []byte) (*graph.Graph, error) {
	var state tfjson.State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("invalid state JSON: %w", err)
	}

	g := graph.NewGraph("state")

	if state.Values == nil {
		return g, nil // empty state
	}

	// Detect primary provider
	providers := make(map[string]int)

	// Process root module resources
	if state.Values.RootModule != nil {
		addModuleResources(g, state.Values.RootModule, "", providers)
	}

	// Set primary provider in metadata
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

	return g, nil
}

func addModuleResources(g *graph.Graph, module *tfjson.StateModule, modulePrefix string, providers map[string]int) {
	for _, r := range module.Resources {
		address := r.Address
		if modulePrefix != "" {
			address = modulePrefix + "." + address
		}

		provider := extractProvider(r.ProviderName)
		providers[provider]++

		// Extract curated attributes for display
		attrs := curateAttributes(r.Type, r.AttributeValues)

		node := &graph.Node{
			ID:           address,
			Type:         graph.NodeTypeResource,
			ResourceType: r.Type,
			Label:        r.Name,
			Provider:     provider,
			Attributes:   attrs,
			Module:       modulePrefix,
		}

		if r.Mode == tfjson.DataResourceMode {
			node.Type = graph.NodeTypeData
		}

		// Add depends_on edges
		for _, dep := range r.DependsOn {
			g.AddEdge(&graph.Edge{
				Source: address,
				Target: dep,
				Type:   "depends_on",
			})
		}

		g.AddNode(node)
	}

	// Recurse into child modules
	for _, child := range module.ChildModules {
		childPrefix := child.Address
		addModuleResources(g, child, childPrefix, providers)
	}
}

// extractProvider returns the short provider name from a full provider address.
// e.g., "registry.terraform.io/hashicorp/aws" -> "aws"
func extractProvider(providerName string) string {
	parts := strings.Split(providerName, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return providerName
}

// curateAttributes extracts a subset of resource attributes useful for display.
// Keeps attributes needed for variant detection, tooltips, and grouping.
func curateAttributes(resourceType string, values map[string]any) map[string]any {
	if values == nil {
		return nil
	}

	// Always include these universal attributes
	keep := map[string]bool{
		"id": true, "name": true, "arn": true, "tags": true,
	}

	// Resource-type-specific attributes worth keeping for display
	typeSpecific := map[string][]string{
		"aws_instance":            {"instance_type", "ami", "availability_zone", "vpc_security_group_ids", "subnet_id"},
		"aws_vpc":                 {"cidr_block", "enable_dns_support", "enable_dns_hostnames"},
		"aws_subnet":             {"cidr_block", "availability_zone", "vpc_id", "map_public_ip_on_launch"},
		"aws_security_group":     {"vpc_id", "description"},
		"aws_lb":                 {"load_balancer_type", "internal", "subnets"},
		"aws_lambda_function":    {"runtime", "handler", "memory_size", "timeout"},
		"aws_s3_bucket":          {"bucket", "region"},
		"aws_db_instance":        {"engine", "engine_version", "instance_class", "allocated_storage"},
		"aws_ecs_service":        {"launch_type", "desired_count", "cluster"},
		"aws_ecs_cluster":        {},
		"aws_rds_cluster":        {"engine", "engine_version", "database_name"},
		"aws_route53_record":     {"type", "ttl", "zone_id"},
		"aws_cloudfront_distribution": {"enabled", "default_root_object"},

		"azurerm_virtual_machine":      {"vm_size", "location"},
		"azurerm_resource_group":       {"location"},
		"azurerm_virtual_network":      {"address_space", "location"},
		"azurerm_subnet":              {"address_prefixes", "virtual_network_name"},
		"azurerm_kubernetes_cluster":   {"dns_prefix", "kubernetes_version", "location"},

		"google_compute_instance":      {"machine_type", "zone"},
		"google_compute_network":       {"auto_create_subnetworks"},
		"google_compute_subnetwork":    {"ip_cidr_range", "region", "network"},
		"google_container_cluster":     {"location", "initial_node_count"},

		"proxmox_vm_qemu":             {"target_node", "cores", "memory", "disk_size"},
		"proxmox_lxc":                 {"target_node", "cores", "memory", "rootfs_size"},
	}

	// Always keep attributes ending in _id (used for implicit edge detection)
	result := make(map[string]any)
	for k, v := range values {
		if keep[k] || strings.HasSuffix(k, "_id") {
			result[k] = v
			continue
		}
		if specific, ok := typeSpecific[resourceType]; ok {
			for _, s := range specific {
				if k == s {
					result[k] = v
					break
				}
			}
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

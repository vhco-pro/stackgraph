package mapping

import (
	"strings"
	"testing"

	"github.com/vhco-pro/stackgraph/pkg/icons"
)

// TestAllIconPathsExist walks every embedded mapping and asserts each declared
// icon resolves to a real embedded file. A dangling icon path silently degrades
// a node to a plain box (and ships a broken path to the frontend JSON), so this
// is enforced at test time.
func TestAllIconPathsExist(t *testing.T) {
	r, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded failed: %v", err)
	}

	checked := 0
	for _, entry := range r.List("", "") {
		if entry.Icon == "" {
			continue
		}
		if _, err := icons.GetIconBytes(entry.Icon); err != nil {
			t.Errorf("%s: icon %q does not exist in embedded icon set", entry.ResourceType, entry.Icon)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no icons checked — registry appears empty")
	}
	t.Logf("validated %d icon paths", checked)

	// Variant icons too
	for resourceType, entry := range r.mappings {
		for _, v := range entry.mapping.Variants {
			if v.Icon == "" {
				continue
			}
			if _, err := icons.GetIconBytes(v.Icon); err != nil {
				t.Errorf("%s variant: icon %q does not exist in embedded icon set", resourceType, v.Icon)
			}
		}
	}
}

// TestCategoryVocabulary keeps the category names consistent across providers
// so `mappings list --category` filters behave predictably: no two categories
// may differ only by case or by a trailing "s" (Database vs Databases).
func TestCategoryVocabulary(t *testing.T) {
	r, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded failed: %v", err)
	}

	seen := map[string]string{} // normalized -> first-seen spelling
	for _, entry := range r.List("", "") {
		if entry.Category == "" {
			continue
		}
		norm := strings.TrimSuffix(strings.ToLower(entry.Category), "s")
		if first, ok := seen[norm]; ok {
			if first != entry.Category {
				t.Errorf("category spelling conflict: %q vs %q (resource %s)", first, entry.Category, entry.ResourceType)
			}
			continue
		}
		seen[norm] = entry.Category
	}
}

// TestFlowPatternsLoaded asserts the embedded YAML flows sections reach the
// registry — the renderer's heuristic edges depend on them.
func TestFlowPatternsLoaded(t *testing.T) {
	r, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded failed: %v", err)
	}
	patterns := r.FlowPatterns()
	if len(patterns) < 30 {
		t.Fatalf("expected 30+ flow patterns from embedded YAML, got %d", len(patterns))
	}
	found := false
	for _, p := range patterns {
		if p[0] == "aws_lb" && p[1] == "aws_instance" {
			found = true
		}
	}
	if !found {
		t.Error("expected aws_lb -> aws_instance flow pattern")
	}
}

// TestContainmentDeclarationsLoaded asserts member_ref and network_scope
// survive the YAML round-trip into lookup results.
func TestContainmentDeclarationsLoaded(t *testing.T) {
	r, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded failed: %v", err)
	}
	sg := r.Lookup("aws_security_group")
	if sg == nil || sg.MemberRef != "vpc_security_group_ids" {
		t.Error("aws_security_group must declare member_ref vpc_security_group_ids")
	}
	for _, network := range []string{"aws_vpc", "azurerm_virtual_network", "google_compute_network"} {
		m := r.Lookup(network)
		if m == nil || !m.NetworkScope {
			t.Errorf("%s must declare network_scope", network)
		}
	}
}

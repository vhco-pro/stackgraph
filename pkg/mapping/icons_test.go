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

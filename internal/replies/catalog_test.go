package replies

import "testing"

func TestEmbeddedCatalogCounts(t *testing.T) {
	t.Parallel()

	catalog, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded() error = %v", err)
	}

	if got := catalog.Len(); got != 1000 {
		t.Fatalf("Len() = %d, want 1000", got)
	}
	wants := map[Tier]int{
		TierSharp:  340,
		TierHarsh:  330,
		TierSavage: 330,
	}
	for tier, want := range wants {
		if got := catalog.Count(tier); got != want {
			t.Errorf("Count(%d) = %d, want %d", tier, got, want)
		}
	}
}

func TestEmbeddedCatalogHasUniqueIDsAndText(t *testing.T) {
	t.Parallel()

	catalog, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded() error = %v", err)
	}

	ids := make(map[string]struct{}, catalog.Len())
	texts := make(map[string]struct{}, catalog.Len())
	for _, entry := range catalog.entries {
		if _, exists := ids[entry.ID]; exists {
			t.Errorf("duplicate ID %q", entry.ID)
		}
		if _, exists := texts[entry.Text]; exists {
			t.Errorf("duplicate text for %q", entry.ID)
		}
		ids[entry.ID] = struct{}{}
		texts[entry.Text] = struct{}{}
	}
}

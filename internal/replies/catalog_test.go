package replies

import (
	"fmt"
	"strings"
	"testing"
)

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

func TestNewCatalogRequiresMoreRepliesThanRecentWindow(t *testing.T) {
	t.Parallel()

	entries := make([]Reply, 0, minimumTierReplies*3-1)
	for tier := TierSharp; tier <= TierSavage; tier++ {
		count := minimumTierReplies
		if tier == TierHarsh {
			count--
		}
		for index := range count {
			entries = append(entries, Reply{
				ID:       fmt.Sprintf("tier-%d-%02d", tier, index),
				Locale:   "pl",
				Category: "generic",
				Tier:     tier,
				Text:     fmt.Sprintf("Unikalna odpowiedź %d-%02d.", tier, index),
			})
		}
	}

	_, err := NewCatalog(entries)
	if err == nil {
		t.Fatal("NewCatalog() error = nil, want tier capacity error")
	}
	if want := "tier 2 has 20 replies, need at least 21"; !strings.Contains(err.Error(), want) {
		t.Fatalf("NewCatalog() error = %q, want text %q", err, want)
	}
}

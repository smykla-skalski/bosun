package replies

import (
	"errors"
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

	if got := catalog.Len(); got != 6400 {
		t.Fatalf("Len() = %d, want 6400", got)
	}
	wants := map[Tier]int{
		TierSharp:  2140,
		TierHarsh:  2130,
		TierSavage: 2130,
	}
	for tier, want := range wants {
		if got := catalog.Count(tier); got != want {
			t.Errorf("Count(%d) = %d, want %d", tier, got, want)
		}
	}
	for _, category := range categories[1:] {
		for tier := TierSharp; tier <= TierSavage; tier++ {
			if got := catalog.CountCategory(category, tier); got != 100 {
				t.Errorf("CountCategory(%q, %d) = %d, want 100", category, tier, got)
			}
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
		if entry.OpeningVariant == 0 || entry.PunchlineVariant == 0 {
			t.Errorf("reply %q has missing composition variants", entry.ID)
		}
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

func TestNewCatalogAcceptsBatchCapacity(t *testing.T) {
	t.Parallel()

	catalog, err := NewCatalog(minimalCatalogEntries(minimumTierReplies))
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	for tier := TierSharp; tier <= TierSavage; tier++ {
		if got := catalog.CountCategory(CategoryGeneric, tier); got != minimumTierReplies {
			t.Errorf("CountCategory(%q, %d) = %d, want %d", CategoryGeneric, tier, got, minimumTierReplies)
		}
	}
}

func TestNewCatalogRequiresBatchCapacity(t *testing.T) {
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
	if want := `category "generic" tier 2 has 27 replies, need at least 28`; !strings.Contains(err.Error(), want) {
		t.Fatalf("NewCatalog() error = %q, want text %q", err, want)
	}
}

func TestNewCatalogRejectsUnknownCategory(t *testing.T) {
	t.Parallel()

	entries := make([]Reply, 0, minimumTierReplies*3)
	for tier := TierSharp; tier <= TierSavage; tier++ {
		for index := range minimumTierReplies {
			category := CategoryGeneric
			if tier == TierSavage && index == minimumTierReplies-1 {
				category = Category("unknown")
			}
			entries = append(entries, Reply{
				ID:       fmt.Sprintf("tier-%d-%02d", tier, index),
				Locale:   "pl",
				Category: category,
				Tier:     tier,
				Text:     fmt.Sprintf("Inna odpowiedź %d-%02d.", tier, index),
			})
		}
	}

	_, err := NewCatalog(entries)
	if err == nil || !strings.Contains(err.Error(), `category "unknown" is invalid`) {
		t.Fatalf("NewCatalog() error = %v, want unknown category error", err)
	}
}

func TestNewCatalogRequiresEveryTierForPresentCategory(t *testing.T) {
	t.Parallel()

	entries := make([]Reply, 0, minimumTierReplies*4)
	for tier := TierSharp; tier <= TierSavage; tier++ {
		for index := range minimumTierReplies {
			entries = append(entries, Reply{
				ID:       fmt.Sprintf("generic-%d-%02d", tier, index),
				Locale:   "pl",
				Category: CategoryGeneric,
				Tier:     tier,
				Text:     fmt.Sprintf("Ogólna odpowiedź %d-%02d.", tier, index),
			})
		}
	}
	for index := range minimumTierReplies {
		entries = append(entries, Reply{
			ID:       fmt.Sprintf("greeting-1-%02d", index),
			Locale:   "pl",
			Category: CategoryGreeting,
			Tier:     TierSharp,
			Text:     fmt.Sprintf("Powitanie numer %02d.", index),
		})
	}

	_, err := NewCatalog(entries)
	if err == nil || !strings.Contains(err.Error(), `category "greeting" tier 2 has 0 replies`) {
		t.Fatalf("NewCatalog() error = %v, want missing category tier error", err)
	}
}

func TestValidateModelCandidate(t *testing.T) {
	t.Parallel()

	unsafe := []string{
		"Sprawdź https://example.com.",
		"Sprawdź example.com.",
		"Hej <@123456789>.",
		"Napisz do jan@example.com.",
		"Kod to 1234567.",
		"Bosun odpowiada.",
		"Nazwa Bosunem też odpada.",
		"Discord odpowiada.",
		"Configured-server odpowiada.",
		"OpenRouter odpowiada.",
	}
	for _, text := range unsafe {
		text := text
		t.Run(text, func(t *testing.T) {
			t.Parallel()
			err := ValidateModelCandidate(text)
			if !errors.Is(err, ErrUnsafeModelCandidate) {
				t.Fatalf("ValidateModelCandidate() error = %v, want ErrUnsafeModelCandidate", err)
			}
		})
	}

	if err := ValidateModelCandidate("To jest bezpieczna odpowiedź bez danych."); err != nil {
		t.Fatalf("ValidateModelCandidate() error = %v", err)
	}
}

func TestValidateModelCandidateRejectsDynamicForbiddenValue(t *testing.T) {
	t.Parallel()

	text := "Mniej dymu, więcej współrzędnych, kapitanie."
	err := ValidateModelCandidate(text, "  KAPITAN  ")
	if !errors.Is(err, ErrUnsafeModelCandidate) {
		t.Fatalf("ValidateModelCandidate() error = %v, want ErrUnsafeModelCandidate", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "kapitan") {
		t.Fatal("ValidateModelCandidate() error contains the forbidden value")
	}
	if err := ValidateModelCandidate(text, "", "   "); err != nil {
		t.Fatalf("ValidateModelCandidate() empty forbidden value error = %v", err)
	}
}

func TestNewCatalogRejectsUnsafeModelCandidate(t *testing.T) {
	t.Parallel()

	_, err := NewCatalog([]Reply{{
		ID:       "unsafe-001",
		Locale:   "pl",
		Category: CategoryGeneric,
		Tier:     TierSharp,
		Text:     "Odwiedź example.com.",
	}})
	if !errors.Is(err, ErrUnsafeModelCandidate) {
		t.Fatalf("NewCatalog() error = %v, want ErrUnsafeModelCandidate", err)
	}
}

func minimalCatalogEntries(repliesPerTier int) []Reply {
	entries := make([]Reply, 0, repliesPerTier*3)
	for tier := TierSharp; tier <= TierSavage; tier++ {
		for index := range repliesPerTier {
			entries = append(entries, Reply{
				ID:       fmt.Sprintf("minimal-%d-%02d", tier, index),
				Locale:   "pl",
				Category: CategoryGeneric,
				Tier:     tier,
				Text:     fmt.Sprintf("Minimalna odpowiedź %d-%02d.", tier, index),
			})
		}
	}
	return entries
}

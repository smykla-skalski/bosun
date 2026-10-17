package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestGenerateContracts(t *testing.T) {
	t.Parallel()

	entries, err := generate()
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}
	expectedCount := 1000 + len(typedSets)*300
	if len(entries) != expectedCount {
		t.Fatalf("generate() count = %d, want %d", len(entries), expectedCount)
	}

	ids := make(map[string]struct{}, len(entries))
	texts := make(map[string]struct{}, len(entries))
	counts := make(map[string]int, len(typedSets)*3)
	tierCounts := make(map[int]int, 3)
	for _, entry := range entries {
		if entry.OpeningVariant < 1 || entry.PunchlineVariant < 1 {
			t.Fatalf("reply %q has invalid variants %d/%d", entry.ID, entry.OpeningVariant, entry.PunchlineVariant)
		}
		if _, exists := ids[entry.ID]; exists {
			t.Fatalf("duplicate ID %q", entry.ID)
		}
		if _, exists := texts[entry.Text]; exists {
			t.Fatalf("duplicate text for %q", entry.ID)
		}
		ids[entry.ID] = struct{}{}
		texts[entry.Text] = struct{}{}
		counts[fmt.Sprintf("%s-%d", entry.Category, entry.Tier)]++
		tierCounts[entry.Tier]++
	}

	for _, set := range typedSets {
		for tier := 1; tier <= 3; tier++ {
			if got := counts[fmt.Sprintf("%s-%d", set.category, tier)]; got != 100 {
				t.Errorf("category %s tier %d count = %d, want 100", set.category, tier, got)
			}
		}
	}

	wantTierCounts := map[int]int{1: 2140, 2: 2130, 3: 2130}
	for tier, want := range wantTierCounts {
		if got := tierCounts[tier]; got != want {
			t.Errorf("tier %d count = %d, want %d", tier, got, want)
		}
	}
}

func TestPunchlinesDoNotRequestMoreContext(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"dodaj ",
		"dopisz ",
		"określ ",
		"podaj ",
		"pokaż ",
		"przynieś ",
		"wyjaśnij ",
		"wróć z ",
		"kontekst",
		"szczegół",
		"wymagan",
	}
	check := func(group string, punchlines []string) {
		t.Helper()
		for _, punchline := range punchlines {
			lower := strings.ToLower(punchline)
			for _, phrase := range forbidden {
				if strings.Contains(lower, phrase) {
					t.Errorf("%s punchline %q requests more context", group, punchline)
				}
			}
		}
	}

	for _, set := range sets {
		check(set.name, set.punchlines)
	}
	for _, set := range typedSets {
		check(set.category, set.punchlines)
	}
}

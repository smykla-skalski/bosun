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
	if len(entries) != 4000 {
		t.Fatalf("generate() count = %d, want 4000", len(entries))
	}

	ids := make(map[string]struct{}, len(entries))
	texts := make(map[string]struct{}, len(entries))
	counts := make(map[string]int, 33)
	tierCounts := make(map[int]int, 3)
	for _, entry := range entries {
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

	for _, category := range []string{"greeting", "question", "request", "complaint", "boast", "insult", "joke", "praise", "thanks", "challenge"} {
		for tier := 1; tier <= 3; tier++ {
			if got := counts[fmt.Sprintf("%s-%d", category, tier)]; got != 100 {
				t.Errorf("category %s tier %d count = %d, want 100", category, tier, got)
			}
		}
	}

	wantTierCounts := map[int]int{1: 1340, 2: 1330, 3: 1330}
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
		check(set.name, set.closings)
	}
	for _, set := range typedSets {
		check(set.category, set.closings)
	}
}

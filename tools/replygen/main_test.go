package main

import (
	"fmt"
	"strings"
	"testing"
	"unicode"
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
		if strings.ContainsAny(entry.Text, ".!?…") {
			t.Fatalf("reply %q is not one sentence without terminal punctuation: %q", entry.ID, entry.Text)
		}
		if strings.TrimSpace(entry.Text) != entry.Text {
			t.Fatalf("reply %q has outer whitespace: %q", entry.ID, entry.Text)
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

func TestComposeReplyCreatesOneDashLinkedSentence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		opening   string
		punchline string
		want      string
	}{
		{name: "addition", opening: "Pomysł uciekł.", punchline: "A publiczność zna drogę.", want: "Pomysł uciekł - publiczność zna drogę"},
		{name: "consequence", opening: "Logika zatonęła!", punchline: "Więc ratownik odmówił skoku?", want: "Logika zatonęła - ratownik odmówił skoku"},
		{name: "acronym", opening: "Diagnoza jest prosta…", punchline: "EKG odmówiło komentarza.", want: "Diagnoza jest prosta - EKG odmówiło komentarza"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := composeReply(test.opening, test.punchline); got != test.want {
				t.Fatalf("composeReply() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCatalogPunchlinesAreDependentFinishers(t *testing.T) {
	t.Parallel()

	starters := []string{"a ", "aż ", "bo ", "choć ", "jakby ", "więc "}
	check := func(group string, punchlines []string) {
		t.Helper()
		for _, punchline := range punchlines {
			lower := strings.ToLower(punchline)
			valid := false
			for _, starter := range starters {
				if strings.HasPrefix(lower, starter) {
					valid = true
					break
				}
			}
			if !valid {
				t.Errorf("%s punchline %q is not a dependent finisher", group, punchline)
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

func TestTemplatePairsDoNotRepeatCoreIdea(t *testing.T) {
	t.Parallel()

	stopWords := map[string]struct{}{
		"albo": {}, "całej": {}, "całym": {}, "cały": {}, "choć": {}, "czeka": {}, "dla": {}, "gdzie": {}, "jeszcze": {},
		"jakby": {}, "jedna": {}, "jeden": {}, "jednak": {}, "kiedy": {}, "kartonu": {}, "lecz": {}, "nadal": {}, "nawet": {},
		"oraz": {}, "pieprzony": {}, "pozostaje": {}, "przed": {}, "przez": {}, "siebie": {}, "sobie": {}, "swoich": {},
		"swoim": {}, "swoje": {}, "temu": {}, "tego": {}, "tutaj": {}, "twoich": {}, "twoim": {}, "twoja": {}, "twoją": {},
		"twoje": {}, "twojego": {}, "twój": {}, "właśnie": {}, "własna": {}, "własne": {}, "własny": {}, "wygląda": {},
		"znowu": {}, "została": {}, "zostało": {}, "zostały": {}, "żeby": {},
	}
	contentWords := func(text string) map[string]struct{} {
		result := make(map[string]struct{})
		for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) }) {
			if len([]rune(word)) < 5 {
				continue
			}
			if _, ignored := stopWords[word]; !ignored {
				result[word] = struct{}{}
			}
		}
		return result
	}
	coreRoots := []string{
		"argument", "awari", "ciekaw", "diagno", "dowcip", "komplement", "logik", "meldun", "obelg",
		"ocen", "pewno", "potwierd", "prośb", "puent", "pytan", "raport", "sens", "sprzeciw",
		"status", "treś", "uzasad", "wdzięcz", "wymaga", "wniosk", "wyzwan", "żart",
	}
	check := func(group string, openings []string, punchlines []string) {
		t.Helper()
		for _, opening := range openings {
			opening = strings.ToLower(opening)
			openingWords := contentWords(opening)
			for _, punchline := range punchlines {
				punchline = strings.ToLower(punchline)
				punchlineWords := contentWords(punchline)
				for word := range openingWords {
					if _, repeated := punchlineWords[word]; repeated {
						t.Errorf("%s repeats content word %q across %q and %q", group, word, opening, punchline)
					}
				}
				for _, root := range coreRoots {
					if strings.Contains(opening, root) && strings.Contains(punchline, root) {
						t.Errorf("%s repeats core idea %q across %q and %q", group, root, opening, punchline)
					}
				}
			}
		}
	}

	for _, set := range sets {
		check(set.name, set.openings, set.punchlines)
	}
	for _, set := range typedSets {
		for tier, openings := range set.openings {
			check(fmt.Sprintf("%s tier %d", set.category, tier+1), openings, set.punchlines)
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

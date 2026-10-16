package replies

import (
	"testing"

	"github.com/smykla-skalski/bosun/internal/messageprofile"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want Category
	}{
		{name: "generic", text: "Bosun, statek płynie spokojnie.", want: CategoryGeneric},
		{name: "greeting", text: "Hej Bosun", want: CategoryGreeting},
		{name: "question", text: "Jak to działa?", want: CategoryQuestion},
		{name: "request", text: "Czy możesz to napisać?", want: CategoryRequest},
		{name: "complaint", text: "Mam problem z tym błędem.", want: CategoryComplaint},
		{name: "negative complaint", text: "To nie działa.", want: CategoryComplaint},
		{name: "boast", text: "Wygrałem bez żadnej pomocy.", want: CategoryBoast},
		{name: "self praise", text: "Jestem najlepszy.", want: CategoryBoast},
		{name: "insult", text: "Ale z ciebie kretyn.", want: CategoryInsult},
		{name: "joke", text: "Haha, to był dobry żart.", want: CategoryJoke},
		{name: "praise", text: "To jest super.", want: CategoryPraise},
		{name: "direct praise", text: "Jesteś najlepszy.", want: CategoryPraise},
		{name: "thanks", text: "Dzięki za pomoc.", want: CategoryThanks},
		{name: "challenge", text: "Spróbuj mnie pokonać.", want: CategoryChallenge},
		{name: "ability challenge", text: "Dasz radę?", want: CategoryChallenge},
		{name: "uppercase", text: "DZIĘKUJĘ!", want: CategoryThanks},
		{name: "question over greeting", text: "Hej, gdzie jest raport?", want: CategoryQuestion},
		{name: "request over question", text: "Czy możesz pokazać raport?", want: CategoryRequest},
		{name: "insult over thanks", text: "Dzięki, idioto.", want: CategoryInsult},
		{name: "complaint over joke", text: "Haha, ale awaria.", want: CategoryComplaint},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := Classify(test.text); got != test.want {
				t.Fatalf("Classify(%q) = %q, want %q", test.text, got, test.want)
			}
		})
	}
}

func TestClassifyIgnoresExternalAndStructuredText(t *testing.T) {
	t.Parallel()

	text := "<@123> <@&456> <:idiota:789> https://example.com/debil www.example.com/problem DISCORD.COM/INVITE/idiota `kretyn` ```awaria``` 12345 $%^"
	if got := Classify(text); got != CategoryGeneric {
		t.Fatalf("Classify() = %q, want %q", got, CategoryGeneric)
	}
}

func TestClassifyHandlesNegatedInsults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want Category
	}{
		{name: "negated", text: "Bosun, nie jesteś głupi.", want: CategoryGeneric},
		{name: "uppercase negated", text: "BOSUN, NIE JESTEŚ GŁUPI!", want: CategoryGeneric},
		{name: "long negated", text: "Bosun, nie jesteś wcale taki głupi.", want: CategoryGeneric},
		{name: "direct", text: "Bosun, jesteś głupi.", want: CategoryInsult},
		{name: "uppercase direct", text: "Bosun, TY GŁUPI!", want: CategoryInsult},
		{name: "punctuation ends negation", text: "Bosun, nie. Jesteś GŁUPI!", want: CategoryInsult},
		{name: "later direct insult", text: "Nie jesteś głupi, ale jesteś debilem.", want: CategoryInsult},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := Classify(test.text); got != test.want {
				t.Fatalf("Classify(%q) = %q, want %q", test.text, got, test.want)
			}
		})
	}
}

func TestClassifyHandlesNegatedPraise(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want Category
	}{
		{name: "adjective", text: "Bosun, to nie jest super.", want: CategoryGeneric},
		{name: "superlative", text: "Bosun, nie jesteś najlepszy.", want: CategoryGeneric},
		{name: "uppercase superlative", text: "BOSUN, NIE JESTEŚ NAJLEPSZY!", want: CategoryGeneric},
		{name: "direct praise", text: "Bosun, jesteś najlepszy!", want: CategoryPraise},
		{name: "direct boast", text: "Bosun, jestem najlepszy!", want: CategoryBoast},
		{name: "punctuation ends negation", text: "Bosun, nie. Jesteś najlepszy!", want: CategoryPraise},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := Classify(test.text); got != test.want {
				t.Fatalf("Classify(%q) = %q, want %q", test.text, got, test.want)
			}
		})
	}
}

func TestClassifyHandlesNegatedJokes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want Category
	}{
		{name: "negated", text: "Bosun, nie żartuję.", want: CategoryGeneric},
		{name: "uppercase negated", text: "BOSUN: NIE ŻARTUJĘ!", want: CategoryGeneric},
		{name: "direct", text: "Bosun, żartuję.", want: CategoryJoke},
		{name: "uppercase direct", text: "BOSUN: ŻARTUJĘ!", want: CategoryJoke},
		{name: "punctuation ends negation", text: "Bosun, nie. Żartuję!", want: CategoryJoke},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := Classify(test.text); got != test.want {
				t.Fatalf("Classify(%q) = %q, want %q", test.text, got, test.want)
			}
		})
	}
}

func TestClassifyHandlesOtherNegatedCategories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want Category
	}{
		{name: "thanks", text: "Bosun, nie dziękuję.", want: CategoryGeneric},
		{name: "uppercase thanks", text: "BOSUN: NIE DZIĘKUJĘ!", want: CategoryGeneric},
		{name: "direct thanks", text: "Bosun, dziękuję!", want: CategoryThanks},
		{name: "thanks after boundary", text: "Bosun, nie. Dziękuję!", want: CategoryThanks},
		{name: "complaint", text: "Bosun, to nie awaria.", want: CategoryGeneric},
		{name: "uppercase complaint", text: "BOSUN: TO NIE AWARIA!", want: CategoryGeneric},
		{name: "direct complaint", text: "Bosun, to awaria!", want: CategoryComplaint},
		{name: "complaint after boundary", text: "Bosun, nie. To awaria!", want: CategoryComplaint},
		{name: "negative failure", text: "Bosun, to nie działa.", want: CategoryComplaint},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := Classify(test.text); got != test.want {
				t.Fatalf("Classify(%q) = %q, want %q", test.text, got, test.want)
			}
		})
	}
}

func TestClassifyHandlesBoundedPolishNegation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want Category
	}{
		{name: "masculine quantifier", text: "Bosun, to żaden problem.", want: CategoryGeneric},
		{name: "feminine quantifier", text: "Bosun, to żadna awaria.", want: CategoryGeneric},
		{name: "ascii quantifier", text: "Bosun, to zaden problem.", want: CategoryGeneric},
		{name: "uppercase quantifier", text: "BOSUN, TO ŻADNA AWARIA!", want: CategoryGeneric},
		{name: "without", text: "Bosun, bez problem.", want: CategoryGeneric},
		{name: "never", text: "Bosun, nigdy awaria.", want: CategoryGeneric},
		{name: "bounded scope", text: "Bosun, żaden jeden dwa trzy cztery problem.", want: CategoryComplaint},
		{name: "contrast ends scope", text: "Bosun, żaden problem ale awaria.", want: CategoryComplaint},
		{name: "punctuation ends scope", text: "Bosun, żaden problem. Awaria!", want: CategoryComplaint},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := Classify(test.text); got != test.want {
				t.Fatalf("Classify(%q) = %q, want %q", test.text, got, test.want)
			}
		})
	}
}

func TestClassifyDistinguishesNegativeFailureFromNegatedProblem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want Category
	}{
		{name: "qualified failure", text: "Bosun, to nie zawsze działa.", want: CategoryComplaint},
		{name: "qualified failure without accents", text: "Bosun, to nie zawsze dziala.", want: CategoryComplaint},
		{name: "failure with two qualifiers", text: "Bosun, to nie zawsze poprawnie działa.", want: CategoryComplaint},
		{name: "failure outside scope", text: "Bosun, nie jeden dwa trzy cztery działa.", want: CategoryGeneric},
		{name: "negated problem", text: "Bosun, to żaden problem.", want: CategoryGeneric},
		{name: "qualified negated problem", text: "Bosun, to żaden duży problem.", want: CategoryGeneric},
		{name: "negated failure noun", text: "Bosun, to żadna poważna awaria.", want: CategoryGeneric},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := Classify(test.text); got != test.want {
				t.Fatalf("Classify(%q) = %q, want %q", test.text, got, test.want)
			}
		})
	}
}

func TestNegatedProblemProducesNeutralProfile(t *testing.T) {
	t.Parallel()

	const text = "Bosun, to żaden problem."
	category := Classify(text)
	if category != CategoryGeneric {
		t.Fatalf("Classify(%q) = %q, want %q", text, category, CategoryGeneric)
	}
	profile, err := messageprofile.Build(text, "bosun", "mention", string(category), "harsh")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Tone != "neutral" {
		t.Fatalf("profile tone = %q, want neutral", profile.Tone)
	}
}

func TestClassifyTreatsUnclosedCodeAsIgnored(t *testing.T) {
	t.Parallel()

	if got := Classify("Bosun `idiota bez końca"); got != CategoryGeneric {
		t.Fatalf("Classify() = %q, want %q", got, CategoryGeneric)
	}
}

func TestClassifyDoesNotAllocate(t *testing.T) {
	allocations := testing.AllocsPerRun(1000, func() {
		if got := Classify("Hej Bosun, czy możesz napisać dobry żart?"); got != CategoryRequest {
			panic(got)
		}
	})
	if allocations != 0 {
		t.Fatalf("Classify() allocations = %.2f, want 0", allocations)
	}
}

func BenchmarkClassify(b *testing.B) {
	const text = "Hej <@123>, czy możesz napisać dobry żart o statku bez https://example.com/problem?"
	b.ReportAllocs()
	for range b.N {
		if got := Classify(text); got != CategoryRequest {
			b.Fatal(got)
		}
	}
}

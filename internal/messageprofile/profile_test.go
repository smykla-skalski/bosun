package messageprofile

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestBuildClassifiesMessageShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		text     string
		category string
		want     Profile
	}{
		{
			name:     "angry direct question",
			text:     "CZY TY TO W OGÓLE ZROBISZ?! KURWA",
			category: "question",
			want:     Profile{Version: Version, Trigger: "mention", Category: "question", ConversationRole: "direct_address", QuestionKind: "information", ResponseMode: "answer_then_roast", Form: "question", Structure: "simple", LengthBucket: "short", Punctuation: "mixed", Tone: "angry", Humor: "none", Directness: "direct", Casing: "upper", ProfanityStrength: "strong", Spice: "savage"},
		},
		{
			name:     "happy playful fragment",
			text:     "super haha 😂",
			category: "joke",
			want:     Profile{Version: Version, Trigger: "mention", Category: "joke", ConversationRole: "direct_address", QuestionKind: "none", ResponseMode: "counter_roast", Form: "fragment", Structure: "minimal", LengthBucket: "tiny", Punctuation: "none", Tone: "joking", Humor: "playful", Directness: "indirect", Casing: "lower", ProfanityStrength: "none", Spice: "savage"},
		},
		{
			name:     "sad compound statement",
			text:     "Jest mi smutno, ale jutro będzie lepiej.",
			category: "generic",
			want:     Profile{Version: Version, Trigger: "mention", Category: "generic", ConversationRole: "direct_address", QuestionKind: "none", ResponseMode: "counter_roast", Form: "statement", Structure: "compound", LengthBucket: "short", Punctuation: "period", Tone: "sad", Humor: "none", Directness: "indirect", Casing: "mixed", ProfanityStrength: "none", Spice: "savage"},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := Build(test.text, "Bosun", "mention", test.category, "savage")
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Build() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestProfileRejectsInconsistentConversationContext(t *testing.T) {
	t.Parallel()

	valid, err := Build("anonimowa treść", "Bosun", "mention", "presence", "harsh")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	tests := []struct {
		name string
		edit func(*Profile)
	}{
		{name: "conversation role", edit: func(profile *Profile) { profile.ConversationRole = "continuation" }},
		{name: "question kind", edit: func(profile *Profile) { profile.QuestionKind = "information" }},
		{name: "response mode", edit: func(profile *Profile) { profile.ResponseMode = "counter_roast" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := valid
			test.edit(&profile)
			if profile.Validate() == nil {
				t.Fatal("Validate() accepted inconsistent conversation labels")
			}
		})
	}
}

func TestProfileJSONHasExactKeysAndNoCanaryData(t *testing.T) {
	t.Parallel()

	canary := "Jan Kowalski @captain 123456789 https://private.example/secret DiScOrD.Gg/private-invite DiScOrD.CoM/InViTe/private-invite report.pdf Discord Bosun configured-server"
	profile, err := Build(canary, "Bosun", "ask", "request", "harsh")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	encoded, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var fields map[string]string
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	wantKeys := []string{
		"version", "trigger", "category", "conversation_role", "question_kind", "response_mode", "form", "structure", "length_bucket",
		"punctuation", "tone", "humor", "directness", "casing", "profanity_strength", "spice",
	}
	if len(fields) != len(wantKeys) {
		t.Fatalf("profile has %d fields, want %d", len(fields), len(wantKeys))
	}
	for _, key := range wantKeys {
		if _, ok := fields[key]; !ok {
			t.Errorf("profile lacks %q", key)
		}
	}
	lower := strings.ToLower(string(encoded))
	for _, forbidden := range []string{"jan", "kowalski", "captain", "123456789", "private.example", "report.pdf", "discord", "bosun", "configured-server"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("profile contains canary %q", forbidden)
		}
	}
}

func TestBuildAddsAnonymousConversationContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		trigger      string
		category     string
		wantRole     string
		wantQuestion string
		wantMode     string
	}{
		{name: "presence", trigger: "mention", category: "presence", wantRole: "direct_address", wantQuestion: "presence", wantMode: "social_banter"},
		{name: "reaction", trigger: "reply", category: "reaction", wantRole: "continuation", wantQuestion: "none", wantMode: "reaction_banter"},
		{name: "opinion command", trigger: "ask", category: "opinion", wantRole: "explicit_request", wantQuestion: "opinion", wantMode: "answer_then_roast"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			profile, err := Build("anonimowa treść", "Bosun", test.trigger, test.category, "harsh")
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if profile.ConversationRole != test.wantRole || profile.QuestionKind != test.wantQuestion || profile.ResponseMode != test.wantMode {
				t.Fatalf("conversation context = %#v", profile)
			}
		})
	}
}

func TestExcludedContentDoesNotChangeAnyProfileFeature(t *testing.T) {
	t.Parallel()

	base := "To jest spokojna wiadomość."
	want, err := Build(base, "Bosun", "mention", "generic", "harsh")
	if err != nil {
		t.Fatalf("Build() base error = %v", err)
	}
	canaries := []string{
		"<@123456789>", "<@!123456789>", "<@&123456789>", "<#123456789>",
		"<:KURWA:123456789>", "<a:HAHA:123456789>", "</WŚCIEKŁY:123456789>", ":KURWA:",
		"<t:123456789:R>", "<t:123456789>",
		"https://WSCIEKLY.example/KURWA???", "WWW.HAHA.example/TAK!!!",
		"DiScOrD.Gg/KURWA???", "DiScOrD.CoM/InViTe/KURWA???",
		"`KURWA??? HAHA`", "```\nKURWA???\nHAHA!!!\n```", "123456789", "BOSUN",
		" \t\n\r ",
	}
	for _, canary := range canaries {
		canary := canary
		t.Run(canary, func(t *testing.T) {
			t.Parallel()
			text := base + strings.Repeat(" "+canary, 128)
			got, buildErr := Build(text, "Bosun", "mention", "generic", "harsh")
			if buildErr != nil {
				t.Fatalf("Build() error = %v", buildErr)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("excluded content changed profile: got %#v, want %#v", got, want)
			}
		})
	}
}

func TestBareInviteURLUsesAWordBoundary(t *testing.T) {
	t.Parallel()

	for _, invite := range []string{"DiScOrD.Gg/private-invite", "DiScOrD.CoM/InViTe/private-invite"} {
		if cleaned := Clean(invite, "Bosun"); cleaned != "" {
			t.Fatalf("Clean() kept a bare invite URL: %q", cleaned)
		}
	}
	for _, nonURL := range []string{"prefixdiscord.gg/private-invite", "prefixdiscord.com/invite/private-invite"} {
		if cleaned := Clean(nonURL, "Bosun"); cleaned != nonURL {
			t.Fatalf("Clean() changed non-URL text: %q", cleaned)
		}
	}
}

func TestNumberedListUsesOneLocalShapeMarker(t *testing.T) {
	t.Parallel()

	base := "1. a\n2. b"
	repeated := base + strings.Repeat("\n3. ", 128)
	baseProfile, err := Build(base, "Bosun", "mention", "generic", "harsh")
	if err != nil {
		t.Fatalf("Build() base error = %v", err)
	}
	repeatedProfile, err := Build(repeated, "Bosun", "mention", "generic", "harsh")
	if err != nil {
		t.Fatalf("Build() repeated error = %v", err)
	}
	cleaned := Clean(repeated, "Bosun")
	if baseProfile.Structure != "list" || !reflect.DeepEqual(repeatedProfile, baseProfile) {
		t.Fatalf("list profiles = %#v and %#v", baseProfile, repeatedProfile)
	}
	if strings.Count(cleaned, listShapeMarker) != 1 || strings.ContainsAny(cleaned, "123") {
		t.Fatalf("cleaned list = %q", cleaned)
	}
}

func TestListShapeRequiresTwoLineMarkers(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"1. a", "v1. a\nv2. b", "tekst 1. a 2. b"} {
		profile, err := Build(text, "Bosun", "mention", "generic", "harsh")
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if profile.Structure == "list" {
			t.Fatalf("Build(%q) classified a non-list as a list", text)
		}
	}
}

func TestTermSignalsRespectNegationBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		text          string
		wantTone      string
		wantHumor     string
		wantProfanity string
	}{
		{name: "happy negated", text: "To nie jest super.", wantTone: "neutral", wantHumor: "none", wantProfanity: "none"},
		{name: "angry negated", text: "Nie jestem zły.", wantTone: "neutral", wantHumor: "none", wantProfanity: "none"},
		{name: "sad negated", text: "Nie jestem smutny.", wantTone: "neutral", wantHumor: "none", wantProfanity: "none"},
		{name: "joke negated", text: "Nie żartuję.", wantTone: "neutral", wantHumor: "none", wantProfanity: "none"},
		{name: "sarcasm negated", text: "To nie jest oczywiście.", wantTone: "neutral", wantHumor: "none", wantProfanity: "none"},
		{name: "profanity negated", text: "To nie jest kurwa.", wantTone: "neutral", wantHumor: "none", wantProfanity: "none"},
		{name: "direct happy", text: "To jest super.", wantTone: "happy", wantHumor: "none", wantProfanity: "none"},
		{name: "direct angry", text: "Jestem zły.", wantTone: "angry", wantHumor: "none", wantProfanity: "none"},
		{name: "direct joke", text: "To żartuję.", wantTone: "joking", wantHumor: "playful", wantProfanity: "none"},
		{name: "direct profanity", text: "To kurwa.", wantTone: "angry", wantHumor: "none", wantProfanity: "strong"},
		{name: "sentence boundary", text: "Nie. Super.", wantTone: "happy", wantHumor: "none", wantProfanity: "none"},
		{name: "contrast boundary", text: "Nie jestem zły, ale super.", wantTone: "happy", wantHumor: "none", wantProfanity: "none"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			profile, err := Build(test.text, "Bosun", "mention", "generic", "harsh")
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if profile.Tone != test.wantTone || profile.Humor != test.wantHumor || profile.ProfanityStrength != test.wantProfanity {
				t.Fatalf("term signals = %q, %q, %q", profile.Tone, profile.Humor, profile.ProfanityStrength)
			}
		})
	}
}

func TestLengthUsesWordAndRuneBounds(t *testing.T) {
	t.Parallel()

	profile, err := Build(strings.Repeat("a", 1000), "Bosun", "ask", "generic", "harsh")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if profile.LengthBucket != "long" {
		t.Fatalf("length bucket = %q, want long", profile.LengthBucket)
	}
}

func TestInterrogativeRequestKeepsQuestionForm(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"Czy możesz to napisać?", "Czy możesz to napisać"} {
		profile, err := Build(text, "Bosun", "ask", "request", "harsh")
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if profile.Form != "question" || profile.Category != "request" {
			t.Fatalf("profile form and category = %q, %q", profile.Form, profile.Category)
		}
	}
}

func TestProfileValidateRejectsEachUnknownLabel(t *testing.T) {
	t.Parallel()

	valid, err := Build("zwykła wiadomość", "Bosun", "reply", "generic", "harsh")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	tests := []struct {
		name string
		edit func(*Profile)
	}{
		{"version", func(p *Profile) { p.Version = "v3" }},
		{"trigger", func(p *Profile) { p.Trigger = "private-trigger" }},
		{"category", func(p *Profile) { p.Category = "private-category" }},
		{"conversation role", func(p *Profile) { p.ConversationRole = "private-role" }},
		{"question kind", func(p *Profile) { p.QuestionKind = "private-question" }},
		{"response mode", func(p *Profile) { p.ResponseMode = "private-response" }},
		{"form", func(p *Profile) { p.Form = "private-form" }},
		{"structure", func(p *Profile) { p.Structure = "private-structure" }},
		{"length", func(p *Profile) { p.LengthBucket = "private-length" }},
		{"punctuation", func(p *Profile) { p.Punctuation = "private-punctuation" }},
		{"tone", func(p *Profile) { p.Tone = "private-tone" }},
		{"humor", func(p *Profile) { p.Humor = "private-humor" }},
		{"directness", func(p *Profile) { p.Directness = "private-directness" }},
		{"casing", func(p *Profile) { p.Casing = "private-casing" }},
		{"profanity", func(p *Profile) { p.ProfanityStrength = "private-profanity" }},
		{"spice", func(p *Profile) { p.Spice = "private-spice" }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			profile := valid
			test.edit(&profile)
			if profile.Validate() == nil {
				t.Fatal("Validate() accepted an unknown label")
			}
		})
	}
}

func FuzzBuildUsesOnlyFixedLabels(f *testing.F) {
	for _, value := range []string{"", "cześć", "CO?!", "😂", "1. a\n2. b", "Discord Bosun configured-server"} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, text string) {
		profile, err := Build(text, "Bosun", "mention", "generic", "savage")
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if err := profile.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
		encoded, err := json.Marshal(profile)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		if len(encoded) > 448 {
			t.Fatalf("profile size = %d", len(encoded))
		}
	})
}

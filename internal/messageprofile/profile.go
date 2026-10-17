package messageprofile

import "errors"

const Version = "v2"

type Profile struct {
	Version           string `json:"version"`
	Trigger           string `json:"trigger"`
	Category          string `json:"category"`
	ConversationRole  string `json:"conversation_role"`
	QuestionKind      string `json:"question_kind"`
	ResponseMode      string `json:"response_mode"`
	Form              string `json:"form"`
	Structure         string `json:"structure"`
	LengthBucket      string `json:"length_bucket"`
	Punctuation       string `json:"punctuation"`
	Tone              string `json:"tone"`
	Humor             string `json:"humor"`
	Directness        string `json:"directness"`
	Casing            string `json:"casing"`
	ProfanityStrength string `json:"profanity_strength"`
	Spice             string `json:"spice"`
}

var (
	triggers   = set("mention", "reply", "handle", "ask")
	categories = set(
		"generic", "greeting", "question", "request", "complaint", "boast",
		"insult", "joke", "praise", "thanks", "challenge", "presence", "status",
		"opinion", "agreement", "disagreement", "reaction", "confusion", "acknowledgement",
	)
	conversationRoles = set("direct_address", "continuation", "explicit_request")
	questionKinds     = set("none", "presence", "status", "opinion", "information")
	responseModes     = set("social_banter", "answer_then_roast", "stance_banter", "reaction_banter", "task_roast", "counter_roast")
	forms             = set("fragment", "statement", "question", "command", "exclamation")
	structures        = set("minimal", "simple", "compound", "list", "repeated")
	lengths           = set("tiny", "short", "medium", "long")
	punctuations      = set("none", "period", "question", "exclamation", "mixed", "ellipsis", "other")
	tones             = set("neutral", "happy", "angry", "joking", "sad")
	humors            = set("none", "playful", "sarcastic")
	directnesses      = set("direct", "indirect")
	casings           = set("uncased", "lower", "mixed", "upper")
	profanities       = set("none", "mild", "strong")
	spices            = set("sharp", "harsh", "savage")
)

func Build(text string, botHandle string, trigger string, category string, spice string) (Profile, error) {
	text = Clean(text, botHandle)
	profile := Profile{
		Version:           Version,
		Trigger:           trigger,
		Category:          category,
		ConversationRole:  conversationRole(trigger),
		QuestionKind:      questionKind(category),
		ResponseMode:      responseMode(category),
		Form:              classifyForm(text, category),
		Structure:         classifyStructure(text),
		LengthBucket:      classifyLength(text),
		Punctuation:       classifyPunctuation(text),
		Tone:              classifyTone(text, category),
		Humor:             classifyHumor(text, category),
		Directness:        classifyDirectness(text, category),
		Casing:            classifyCasing(text),
		ProfanityStrength: classifyProfanity(text),
		Spice:             spice,
	}
	if err := profile.Validate(); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func (p Profile) Validate() error {
	checks := []struct {
		value   string
		allowed map[string]struct{}
	}{
		{p.Version, set(Version)},
		{p.Trigger, triggers},
		{p.Category, categories},
		{p.ConversationRole, conversationRoles},
		{p.QuestionKind, questionKinds},
		{p.ResponseMode, responseModes},
		{p.Form, forms},
		{p.Structure, structures},
		{p.LengthBucket, lengths},
		{p.Punctuation, punctuations},
		{p.Tone, tones},
		{p.Humor, humors},
		{p.Directness, directnesses},
		{p.Casing, casings},
		{p.ProfanityStrength, profanities},
		{p.Spice, spices},
	}
	for _, check := range checks {
		if _, ok := check.allowed[check.value]; !ok {
			return errors.New("message profile has an invalid label")
		}
	}
	if p.ConversationRole != conversationRole(p.Trigger) ||
		p.QuestionKind != questionKind(p.Category) ||
		p.ResponseMode != responseMode(p.Category) {
		return errors.New("message profile has inconsistent conversation labels")
	}
	return nil
}

func conversationRole(trigger string) string {
	switch trigger {
	case "reply":
		return "continuation"
	case "ask":
		return "explicit_request"
	default:
		return "direct_address"
	}
}

func questionKind(category string) string {
	switch category {
	case "presence", "status", "opinion":
		return category
	case "question":
		return "information"
	default:
		return "none"
	}
}

func responseMode(category string) string {
	switch category {
	case "greeting", "presence", "status":
		return "social_banter"
	case "question", "opinion":
		return "answer_then_roast"
	case "agreement", "disagreement":
		return "stance_banter"
	case "reaction", "confusion", "acknowledgement":
		return "reaction_banter"
	case "request":
		return "task_roast"
	default:
		return "counter_roast"
	}
}

func set(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

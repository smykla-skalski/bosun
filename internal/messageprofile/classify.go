package messageprofile

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var tokenBreaks = regexp.MustCompile(`[^\pL\pN]+`)

var happyTerms = termSet(
	"dobrze", "fajnie", "super", "świetnie", "swietnie", "radość", "radosc", "happy", "great", "love",
)
var angryTerms = termSet(
	"zły", "zly", "zła", "zla", "wkurzony", "wkurzona", "wściekły", "wsciekly", "nienawidzę", "nienawidze", "angry", "hate",
)
var sadTerms = termSet(
	"smutny", "smutna", "smutno", "przykro", "płaczę", "placze", "sad", "sorry",
)
var jokeTerms = termSet(
	"haha", "hehe", "hihi", "lol", "lmao", "xd", "żart", "zart", "żartuję", "zartuje", "joke",
)
var sarcasmTerms = termSet(
	"jasne", "oczywiście", "oczywiscie", "genialnie", "brawo", "sure", "obviously", "sarcasm",
)
var compoundTerms = termSet(
	"ale", "więc", "wiec", "oraz", "ponieważ", "poniewaz", "chociaż", "chociaz", "lecz", "jednak", "and", "but", "because",
)
var strongProfanity = termSet(
	"kurwa", "chuj", "chuja", "pierdolić", "pierdolic", "jebać", "jebac", "skurwysyn", "skurwysynu",
)
var mildProfanity = termSet(
	"cholera", "kurde", "gówno", "gowno", "dupa", "debil", "idiota", "idioto",
)
var negationTerms = termSet(
	"nie", "nigdy", "bez", "żaden", "zaden", "żadna", "zadna",
)
var negationBreakTerms = termSet(
	"ale", "lecz", "jednak", "tylko", "but", "yet",
)

const negationWindow = 4

func classifyForm(text string, category string) string {
	trimmed := strings.TrimSpace(text)
	switch {
	case hasQuestionForm(trimmed) || category == "question":
		return "question"
	case category == "request":
		return "command"
	case strings.HasSuffix(trimmed, "!"):
		return "exclamation"
	case len(words(trimmed)) <= 3 && !strings.ContainsAny(trimmed, ".!?"):
		return "fragment"
	default:
		return "statement"
	}
}

func classifyStructure(text string) string {
	trimmed := strings.TrimSpace(text)
	tokens := words(trimmed)
	switch {
	case looksLikeList(trimmed):
		return "list"
	case hasRepeatedPunctuation(trimmed) || hasAdjacentDuplicate(tokens):
		return "repeated"
	case len(tokens) <= 3:
		return "minimal"
	case containsTerm(tokens, compoundTerms) || sentenceBreakCount(trimmed) > 1:
		return "compound"
	default:
		return "simple"
	}
}

func classifyLength(text string) string {
	wordCount := len(words(text))
	runeCount := utf8.RuneCountInString(strings.TrimSpace(text))
	switch {
	case wordCount <= 3 && runeCount <= 12:
		return "tiny"
	case wordCount <= 10 && runeCount <= 64:
		return "short"
	case wordCount <= 30 && runeCount <= 200:
		return "medium"
	default:
		return "long"
	}
}

func hasQuestionForm(text string) bool {
	if strings.Contains(text, "?") {
		return true
	}
	tokens := words(text)
	if len(tokens) == 0 {
		return false
	}
	switch tokens[0] {
	case "czy", "co", "kto", "kiedy", "gdzie", "jak", "dlaczego", "czemu", "ile", "który", "ktory":
		return true
	default:
		return false
	}
}

func classifyPunctuation(text string) string {
	hasQuestion := strings.Contains(text, "?")
	hasExclamation := strings.Contains(text, "!")
	switch {
	case hasQuestion && hasExclamation:
		return "mixed"
	case strings.Contains(text, "...") || strings.Contains(text, "…"):
		return "ellipsis"
	case hasQuestion:
		return "question"
	case hasExclamation:
		return "exclamation"
	case strings.Contains(text, "."):
		return "period"
	case strings.ContainsAny(text, ",:;-"):
		return "other"
	default:
		return "none"
	}
}

func classifyTone(text string, category string) string {
	switch {
	case category == "joke" || containsUnnegatedTerm(text, jokeTerms) || strings.Contains(text, "😂") || strings.Contains(text, "🤣"):
		return "joking"
	case category == "insult" || category == "complaint" || containsUnnegatedTerm(text, angryTerms) || containsUnnegatedTerm(text, strongProfanity) || strings.Contains(text, "😡"):
		return "angry"
	case containsUnnegatedTerm(text, sadTerms) || strings.Contains(text, "😢") || strings.Contains(text, "😭"):
		return "sad"
	case category == "praise" || category == "thanks" || containsUnnegatedTerm(text, happyTerms) || strings.Contains(text, "😊") || strings.Contains(text, "❤️"):
		return "happy"
	default:
		return "neutral"
	}
}

func classifyHumor(text string, category string) string {
	switch {
	case containsUnnegatedTerm(text, sarcasmTerms):
		return "sarcastic"
	case category == "joke" || containsUnnegatedTerm(text, jokeTerms) || strings.Contains(text, "😂") || strings.Contains(text, "🤣"):
		return "playful"
	default:
		return "none"
	}
}

func classifyDirectness(_ string, category string) string {
	switch category {
	case "question", "request", "insult", "challenge", "praise", "thanks", "greeting",
		"presence", "status", "opinion", "agreement", "disagreement", "reaction", "confusion", "acknowledgement":
		return "direct"
	default:
		return "indirect"
	}
}

func classifyCasing(text string) string {
	hasLower := false
	hasUpper := false
	for _, value := range text {
		hasLower = hasLower || unicode.IsLower(value)
		hasUpper = hasUpper || unicode.IsUpper(value)
	}
	switch {
	case !hasLower && !hasUpper:
		return "uncased"
	case hasLower && !hasUpper:
		return "lower"
	case hasUpper && !hasLower:
		return "upper"
	default:
		return "mixed"
	}
}

func classifyProfanity(text string) string {
	switch {
	case containsUnnegatedTerm(text, strongProfanity):
		return "strong"
	case containsUnnegatedTerm(text, mildProfanity):
		return "mild"
	default:
		return "none"
	}
}

func words(text string) []string {
	return strings.Fields(tokenBreaks.ReplaceAllString(strings.ToLower(text), " "))
}

func containsTerm(tokens []string, terms map[string]struct{}) bool {
	for _, token := range tokens {
		if _, ok := terms[token]; ok {
			return true
		}
	}
	return false
}

type scopedTerm struct {
	value  string
	clause int
}

func containsUnnegatedTerm(text string, terms map[string]struct{}) bool {
	tokens := scanScopedTerms(text)
	for index, token := range tokens {
		if _, ok := terms[token.value]; !ok {
			continue
		}
		if !isNegated(tokens, index) {
			return true
		}
	}
	return false
}

func isNegated(tokens []scopedTerm, index int) bool {
	start := max(0, index-negationWindow)
	for previous := index - 1; previous >= start; previous-- {
		if tokens[previous].clause != tokens[index].clause {
			break
		}
		if _, ok := negationBreakTerms[tokens[previous].value]; ok {
			break
		}
		if _, ok := negationTerms[tokens[previous].value]; ok {
			return true
		}
	}
	return false
}

func scanScopedTerms(text string) []scopedTerm {
	normalized := strings.ToLower(text)
	tokens := make([]scopedTerm, 0, 8)
	start := -1
	clause := 0
	for index, value := range normalized {
		if unicode.IsLetter(value) || unicode.IsDigit(value) {
			if start < 0 {
				start = index
			}
			continue
		}
		if start >= 0 {
			tokens = append(tokens, scopedTerm{value: normalized[start:index], clause: clause})
			start = -1
		}
		if strings.ContainsRune(".,!?;:", value) {
			clause++
		}
	}
	if start >= 0 {
		tokens = append(tokens, scopedTerm{value: normalized[start:], clause: clause})
	}
	return tokens
}

func termSet(values ...string) map[string]struct{} {
	return set(values...)
}

func looksLikeList(text string) bool {
	return strings.Contains(text, listShapeMarker)
}

func hasRepeatedPunctuation(text string) bool {
	return strings.Contains(text, "???") || strings.Contains(text, "!!!") || strings.Contains(text, "...")
}

func hasAdjacentDuplicate(tokens []string) bool {
	for index := 1; index < len(tokens); index++ {
		if tokens[index] == tokens[index-1] {
			return true
		}
	}
	return false
}

func sentenceBreakCount(text string) int {
	count := 0
	previousBreak := false
	for _, value := range text {
		isBreak := value == '.' || value == '!' || value == '?'
		if isBreak && !previousBreak {
			count++
		}
		previousBreak = isBreak
	}
	return count
}

package replies

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

var presenceWords = [...]string{
	"żyjesz", "zyjesz", "działasz", "dzialasz", "śpisz", "spisz", "halo",
}

var opinionWords = [...]string{
	"myślisz", "myslisz", "sądzisz", "sadzisz", "oceniasz", "uważasz", "uwazasz",
}

var agreementUtterances = [...]string{
	"tak", "dokładnie", "dokladnie", "racja", "zgoda", "prawda", "ano", "pewnie", "no właśnie", "no wlasnie",
}

var disagreementUtterances = [...]string{
	"nie", "nieprawda", "bzdura", "nonsens", "bez sensu", "nie zgadzam się", "nie zgadzam sie",
}

var confusionUtterances = [...]string{
	"co", "hę", "he", "wtf", "serio", "jak to", "nie rozumiem", "o co chodzi",
}

var acknowledgementUtterances = [...]string{
	"ok", "okej", "dobra", "jasne", "spoko", "rozumiem", "przyjąłem", "przyjalem", "przyjęte", "przyjete",
}

var reactionUtterances = [...]string{
	"ambitnie", "ciekawie", "grubo", "mocno", "ładnie", "ladnie", "pięknie", "pieknie", "odważnie", "odwaznie",
}

func classifyConversation(text string) (Category, int) {
	utterance := trimUtterance(text)
	hasQuestion := strings.Contains(text, "?")
	switch {
	case matchesUtterance(utterance, confusionUtterances[:]):
		return CategoryConfusion, countWords(text)
	case matchesUtterance(utterance, disagreementUtterances[:]):
		return CategoryDisagreement, countWords(text)
	case matchesUtterance(utterance, acknowledgementUtterances[:]):
		return CategoryAcknowledgement, countWords(text)
	case matchesUtterance(utterance, reactionUtterances[:]):
		return CategoryReaction, countWords(text)
	case !hasQuestion && matchesUtterance(utterance, agreementUtterances[:]):
		return CategoryAgreement, countWords(text)
	}

	scan := scanConversation(text)
	switch {
	case scan.presence:
		return CategoryPresence, scan.wordCount
	case scan.status:
		return CategoryStatus, scan.wordCount
	case scan.opinion || hasQuestion && strings.EqualFold(utterance, "prawda"):
		return CategoryOpinion, scan.wordCount
	case scan.confusion:
		return CategoryConfusion, scan.wordCount
	default:
		return CategoryGeneric, scan.wordCount
	}
}

type conversationScan struct {
	wordCount int
	presence  bool
	status    bool
	opinion   bool
	confusion bool
}

func scanConversation(text string) conversationScan {
	result := conversationScan{}
	previous := ""
	previousPrevious := ""
	for index := 0; index < len(text); {
		if text[index] == '`' {
			index = skipCode(text, index)
			previousPrevious, previous = "", ""
			continue
		}
		if text[index] == '<' {
			index = skipDelimited(text, index, '>')
			previousPrevious, previous = "", ""
			continue
		}
		if isURLAt(text, index) {
			index = skipField(text, index)
			previousPrevious, previous = "", ""
			continue
		}
		if text[index] == ':' && hasClosingColon(text, index) {
			index = skipDelimited(text, index, ':')
			previousPrevious, previous = "", ""
			continue
		}
		r, size := utf8.DecodeRuneInString(text[index:])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			index += size
			continue
		}
		start := index
		for index < len(text) {
			r, size = utf8.DecodeRuneInString(text[index:])
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				break
			}
			index += size
		}
		word := text[start:index]
		result.wordCount++
		result.presence = result.presence || matchesWord(word, presenceWords[:]) ||
			(strings.EqualFold(previous, "jesteś") || strings.EqualFold(previous, "jestes")) && strings.EqualFold(word, "tam")
		result.status = result.status || statusPair(previous, word) ||
			(strings.EqualFold(previousPrevious, "jak") && (strings.EqualFold(previous, "się") || strings.EqualFold(previous, "sie")) && strings.EqualFold(word, "masz"))
		result.opinion = result.opinion || matchesWord(word, opinionWords[:]) ||
			strings.EqualFold(previous, "ma") && strings.EqualFold(word, "sens")
		result.confusion = result.confusion ||
			strings.EqualFold(previous, "nie") && strings.EqualFold(word, "rozumiem") ||
			strings.EqualFold(previousPrevious, "o") && strings.EqualFold(previous, "co") && strings.EqualFold(word, "chodzi")
		previousPrevious, previous = previous, word
	}
	return result
}

func statusPair(previous string, word string) bool {
	return strings.EqualFold(previous, "co") && (strings.EqualFold(word, "tam") || strings.EqualFold(word, "słychać") || strings.EqualFold(word, "slychac")) ||
		strings.EqualFold(previous, "jak") && (strings.EqualFold(word, "tam") || strings.EqualFold(word, "leci"))
}

func trimUtterance(text string) string {
	text = strings.TrimSpace(text)
	for text != "" {
		r, size := utf8.DecodeLastRuneInString(text)
		if !unicode.IsSpace(r) && !strings.ContainsRune("?!.,;:…", r) {
			break
		}
		text = text[:len(text)-size]
	}
	return strings.TrimSpace(text)
}

func matchesUtterance(text string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.EqualFold(text, candidate) {
			return true
		}
	}
	return false
}

func countWords(text string) int {
	return scanConversation(text).wordCount
}

func continuationTrigger(trigger string) bool {
	return trigger == "reply"
}

func shortReaction(text string, wordCount int) bool {
	return strings.TrimSpace(text) != "" && wordCount <= 3
}

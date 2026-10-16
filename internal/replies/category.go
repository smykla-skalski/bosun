package replies

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type Category string

const (
	CategoryGeneric   Category = "generic"
	CategoryGreeting  Category = "greeting"
	CategoryQuestion  Category = "question"
	CategoryRequest   Category = "request"
	CategoryComplaint Category = "complaint"
	CategoryBoast     Category = "boast"
	CategoryInsult    Category = "insult"
	CategoryJoke      Category = "joke"
	CategoryPraise    Category = "praise"
	CategoryThanks    Category = "thanks"
	CategoryChallenge Category = "challenge"

	categoryCount = 11
)

var categories = [...]Category{
	CategoryGeneric,
	CategoryGreeting,
	CategoryQuestion,
	CategoryRequest,
	CategoryComplaint,
	CategoryBoast,
	CategoryInsult,
	CategoryJoke,
	CategoryPraise,
	CategoryThanks,
	CategoryChallenge,
}

type categorySignals uint16

const (
	signalGreeting categorySignals = 1 << iota
	signalQuestion
	signalRequest
	signalComplaint
	signalBoast
	signalInsult
	signalJoke
	signalPraise
	signalThanks
	signalChallenge
)

func Classify(text string) Category {
	signals := scanSignals(text)
	for _, match := range [...]struct {
		signal   categorySignals
		category Category
	}{
		{signalInsult, CategoryInsult},
		{signalComplaint, CategoryComplaint},
		{signalChallenge, CategoryChallenge},
		{signalRequest, CategoryRequest},
		{signalThanks, CategoryThanks},
		{signalPraise, CategoryPraise},
		{signalBoast, CategoryBoast},
		{signalJoke, CategoryJoke},
		{signalQuestion, CategoryQuestion},
		{signalGreeting, CategoryGreeting},
	} {
		if signals&match.signal != 0 {
			return match.category
		}
	}
	return CategoryGeneric
}

func scanSignals(text string) categorySignals {
	var signals categorySignals
	nieScopeActive := false
	negationWindow := 0
	secondPerson := false
	waitsForAbility := false
	for index := 0; index < len(text); {
		if text[index] == '?' {
			signals |= signalQuestion
			nieScopeActive = false
			negationWindow = 0
			index++
			continue
		}
		if text[index] == '`' {
			index = skipCode(text, index)
			continue
		}
		if text[index] == '<' {
			index = skipDelimited(text, index, '>')
			continue
		}
		if isURLAt(text, index) {
			index = skipField(text, index)
			continue
		}
		if text[index] == ':' && hasClosingColon(text, index) {
			index = skipDelimited(text, index, ':')
			continue
		}

		r, size := utf8.DecodeRuneInString(text[index:])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			if isNegationBoundary(r) {
				nieScopeActive = false
				negationWindow = 0
			}
			index += size
			continue
		}

		start := index
		hasLetter := false
		for index < len(text) {
			r, size = utf8.DecodeRuneInString(text[index:])
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				break
			}
			hasLetter = hasLetter || unicode.IsLetter(r)
			index += size
		}
		if hasLetter {
			word := text[start:index]
			if matchesWord(word, negationBreakWords[:]) {
				nieScopeActive = false
				negationWindow = 0
			}
			if nieScopeActive && matchesWord(word, failureWords[:]) {
				signals |= signalComplaint
			}
			if waitsForAbility && matchesWord(word, abilityWords[:]) {
				signals |= signalChallenge
			}
			wordSignals := signalsForWord(word)
			if secondPerson && matchesWord(word, superlativeWords[:]) {
				wordSignals |= signalPraise
			}
			if negationWindow > 0 {
				wordSignals = 0
			}
			signals |= wordSignals
			if matchesWord(word, negationWords[:]) {
				nieScopeActive = strings.EqualFold(word, "nie")
				negationWindow = 4
			} else {
				if negationWindow > 0 {
					negationWindow--
				}
				if negationWindow == 0 {
					nieScopeActive = false
				}
			}
			waitsForAbility = strings.EqualFold(word, "dasz")
			secondPerson = strings.EqualFold(word, "jesteś") || strings.EqualFold(word, "jestes")
		}
	}
	return signals
}

func isNegationBoundary(r rune) bool {
	switch r {
	case ',', '.', ';', ':', '!', '?':
		return true
	default:
		return false
	}
}

func signalsForWord(word string) categorySignals {
	var signals categorySignals
	if matchesWord(word, greetingWords[:]) {
		signals |= signalGreeting
	}
	if matchesWord(word, questionWords[:]) {
		signals |= signalQuestion
	}
	if matchesWord(word, requestWords[:]) {
		signals |= signalRequest
	}
	if matchesWord(word, complaintWords[:]) {
		signals |= signalComplaint
	}
	if matchesWord(word, boastWords[:]) {
		signals |= signalBoast
	}
	if matchesWord(word, insultWords[:]) {
		signals |= signalInsult
	}
	if matchesWord(word, jokeWords[:]) {
		signals |= signalJoke
	}
	if matchesWord(word, praiseWords[:]) {
		signals |= signalPraise
	}
	if matchesWord(word, thanksWords[:]) {
		signals |= signalThanks
	}
	if matchesWord(word, challengeWords[:]) {
		signals |= signalChallenge
	}
	return signals
}

func matchesWord(word string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.EqualFold(word, candidate) {
			return true
		}
	}
	return false
}

func skipCode(text string, start int) int {
	markerLength := 1
	if strings.HasPrefix(text[start:], "```") {
		markerLength = 3
	}
	marker := text[start : start+markerLength]
	rest := text[start+markerLength:]
	end := strings.Index(rest, marker)
	if end < 0 {
		return len(text)
	}
	return start + markerLength + end + markerLength
}

func skipDelimited(text string, start int, delimiter byte) int {
	end := strings.IndexByte(text[start+1:], delimiter)
	if end < 0 {
		return start + 1
	}
	return start + end + 2
}

func skipField(text string, start int) int {
	for start < len(text) {
		r, size := utf8.DecodeRuneInString(text[start:])
		if unicode.IsSpace(r) {
			break
		}
		start += size
	}
	return start
}

func isURLAt(text string, index int) bool {
	if index > 0 {
		r, _ := utf8.DecodeLastRuneInString(text[:index])
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	remaining := text[index:]
	return hasFoldPrefix(remaining, "http://") ||
		hasFoldPrefix(remaining, "https://") ||
		hasFoldPrefix(remaining, "www.") ||
		hasFoldPrefix(remaining, "discord.gg/") ||
		hasFoldPrefix(remaining, "discord.com/invite/")
}

func hasFoldPrefix(text, prefix string) bool {
	return len(text) >= len(prefix) && strings.EqualFold(text[:len(prefix)], prefix)
}

func hasClosingColon(text string, start int) bool {
	end := strings.IndexByte(text[start+1:], ':')
	if end < 1 {
		return false
	}
	for _, r := range text[start+1 : start+end+1] {
		if unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

var greetingWords = [...]string{
	"cześć", "czesc", "hej", "hejka", "siema", "witaj", "witam", "elo", "dzień", "dzien",
}

var questionWords = [...]string{
	"co", "kto", "kiedy", "gdzie", "jak", "dlaczego", "czemu", "czy", "ile", "który", "ktory",
}

var requestWords = [...]string{
	"proszę", "prosze", "zrób", "zrob", "daj", "napisz", "powiedz", "pomóż", "pomoz", "wyjaśnij", "wyjasnij", "sprawdź", "sprawdz", "pokaż", "pokaz", "wygeneruj", "opisz", "możesz", "mozesz", "mógłbyś", "moglbys",
}

var complaintWords = [...]string{
	"problem", "błąd", "blad", "awaria", "zepsute", "zepsuło", "zepsulo", "niedziała", "niedziala", "wkurza", "beznadziejne", "słabo", "slabo", "porażka", "porazka",
}

var boastWords = [...]string{
	"wygrałem", "wygralem", "wygrałam", "wygralam", "zrobiłem", "zrobilem", "zrobiłam", "zrobilam", "potrafię", "potrafie", "mistrz", "geniusz", "najlepszy", "najlepsza",
}

var insultWords = [...]string{
	"idiota", "idioto", "idiotą", "idiotka", "głupi", "glupi", "głupia", "glupia", "głupku", "glupku", "debil", "debilu", "debilem", "kretyn", "kretynie", "kretynem", "dureń", "duren", "pajac", "pajacu", "pajacem", "frajer", "frajerze", "frajerem", "spierdalaj",
}

var jokeWords = [...]string{
	"haha", "hehe", "hahaha", "lol", "lmao", "xd", "żart", "zart", "żartuję", "zartuje", "śmieszne", "smieszne", "beka",
}

var praiseWords = [...]string{
	"świetny", "swietny", "świetna", "swietna", "super", "genialny", "genialna", "piękny", "piekny", "kocham", "szacun", "imponujące", "imponujace",
}

var thanksWords = [...]string{
	"dzięki", "dzieki", "dziękuję", "dziekuje", "podziękowania", "podziekowania", "thx", "thanks",
}

var challengeWords = [...]string{
	"spróbuj", "sprobuj", "udowodnij", "wyzywam", "pokonaj", "odważyłbyś", "odwazylbys", "odważyłabyś", "odwazylabys", "roastuj", "jedziesz",
}

var negationWords = [...]string{
	"nie", "nigdy", "bez", "żaden", "zaden", "żadna", "zadna",
}

var negationBreakWords = [...]string{
	"ale", "lecz", "jednak", "tylko", "but", "yet",
}

var failureWords = [...]string{"działa", "dziala", "działają", "dzialaja", "uruchamia", "odpowiada"}

var abilityWords = [...]string{"radę", "rade"}

var superlativeWords = [...]string{"najlepszy", "najlepsza", "najlepsze"}

func normalizeCategory(category Category) Category {
	if categoryIndex(category) < 0 {
		return CategoryGeneric
	}
	return category
}

func categoryIndex(category Category) int {
	switch category {
	case CategoryGeneric:
		return 0
	case CategoryGreeting:
		return 1
	case CategoryQuestion:
		return 2
	case CategoryRequest:
		return 3
	case CategoryComplaint:
		return 4
	case CategoryBoast:
		return 5
	case CategoryInsult:
		return 6
	case CategoryJoke:
		return 7
	case CategoryPraise:
		return 8
	case CategoryThanks:
		return 9
	case CategoryChallenge:
		return 10
	default:
		return -1
	}
}

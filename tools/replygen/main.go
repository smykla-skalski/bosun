package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

var check = flag.Bool("check", false, "verify that the checked-in catalog is current")

type templateSet struct {
	name       string
	tier       int
	openings   []string
	punchlines []string
}

type typedTemplateSet struct {
	category   string
	openings   [3][]string
	punchlines []string
}

type reply struct {
	ID               string `json:"id"`
	Locale           string `json:"locale"`
	Category         string `json:"category"`
	Tier             int    `json:"tier"`
	OpeningVariant   int    `json:"opening_variant"`
	PunchlineVariant int    `json:"punchline_variant"`
	Text             string `json:"text"`
}

var sets = []templateSet{
	{
		name: "sharp",
		tier: 1,
		openings: []string{
			"Twój pomysł wszedł na pokład bez mapy i kompasu.",
			"To pytanie ma więcej pewności niż treści.",
			"Widzę odwagę, lecz sens został w porcie.",
			"Ten plan brzmi jak szkic napisany podczas sztormu.",
			"Pytasz śmiało, jakby szczegóły były opcjonalne.",
			"To zdanie dopłynęło, ale argument wypadł za burtę.",
			"Masz kierunek, tylko każdy drogowskaz wskazuje gdzie indziej.",
			"Pomysł jest świeży, bo rozsądek jeszcze go nie dotknął.",
			"To wygląda na plan złożony z samych przypisów.",
			"Twoja logika zrobiła elegancki unik przed tematem.",
			"Pytanie jest krótkie, a chaos wyjątkowo wydajny.",
			"Ten wniosek przybył przed własnymi przesłankami.",
			"Doceniam tempo, choć myśl została daleko z tyłu.",
			"To odważna próba negocjacji z faktami.",
			"Twój argument ma piękny kadłub i pustą ładownię.",
			"Tu nawet przecinek wygląda na bardziej przygotowany.",
			"Pomysł dostał wiatr w żagle, lecz nie dostał steru.",
			"To pytanie przyszło bez celu i udaje turystę.",
			"Widzę plan, który bardzo starannie omija rozwiązanie.",
			"Twoja teza jest szybka, lekka i całkiem bez bagażu dowodów.",
		},
		punchlines: []string{
			"A latarnia zgasiła światło, żeby nie wskazywać tego kursu.",
			"Bo biblioteka przeniosła tę chwilę do działu fantastyki.",
			"Więc automat z kawą wyświetlił komunikat o litości.",
			"A kalendarz skreślił tę minutę dla własnej reputacji.",
			"Choć lustro odwróciło się do ściany bez jednego słowa.",
			"Aż winda wybrała inne piętro bez naciskania przycisku.",
			"A gaśnica spadła ze ściany czysto profilaktycznie.",
			"Bo krzesło rozważa zmianę właściciela.",
			"Więc radio przełączyło się na szum ostrzegawczy.",
			"Jakby mewa za oknem właśnie straciła apetyt.",
			"A dywan powoli zwijał się w stronę wyjścia.",
			"Choć drzwi awaryjne otworzyły się z czystej uprzejmości.",
			"Aż żarówka zamrugała alfabetem Morse'a po pomoc.",
			"A sufit zaczął sprawdzać warunki polisy.",
			"Bo cisza cofnęła się i założyła kask.",
			"Więc zegar cofnął wskazówki dla ochrony świadków.",
			"A pokładowy szczur poprosił o transfer na ląd.",
		},
	},
	{
		name: "harsh",
		tier: 2,
		openings: []string{
			"Znowu wrzucasz chaos i liczysz, że sam ułoży się w plan.",
			"Ten prompt wygląda jak fuszerka po trzech poprawkach fuszerki.",
			"Twoje pytanie właśnie pobiło rekord w marnowaniu własnych słów.",
			"To już nie brak szczegółów, tylko zorganizowana ewakuacja sensu.",
			"Wkład minimalny, oczekiwania królewskie, rezultat przewidywalny.",
			"Ten argument trzyma się kupy wyłącznie z bardzo dużej odległości.",
			"Przynosisz mi bałagan i nazywasz go zadaniem.",
			"Twoja logika krąży po porcie i boi się zacumować.",
			"To pytanie jest tak leniwe, że nawet błąd musi pracować za nie.",
			"Znowu próbujesz zastąpić przygotowanie głośnym wejściem.",
			"Ten plan ma strukturę mokrego kartonu.",
			"Dostarczasz pół myśli i żądasz pełnej mapy.",
			"To nie jest skrót myślowy, tylko objazd bez powrotu.",
			"Twoje założenia urządziły bunt przeciwko rzeczywistości.",
			"Nawet przypadek wykonałby tu dokładniejszą robotę.",
		},
		punchlines: []string{
			"A publiczność zamówiła taksówkę przed pierwszym echem.",
			"Bo mewa wyrzuciła frytkę po jednym spojrzeniu.",
			"Więc automat biletowy wyświetlił litość zamiast ceny.",
			"A roślina w sąsiednim pokoju zwiędła z drugiej ręki.",
			"Choć pokładowy szczur spakował ser i złożył wymówienie.",
			"Aż rynsztok zażądał uczciwszego porównania.",
			"A karton przy śmietniku wyprostował się z wyższością.",
			"Bo kalendarz ogłosił święto cudzej cierpliwości.",
			"Więc lustro pękło bez żadnego kontaktu fizycznego.",
			"Jakby krzesło szykowało pozew do producenta.",
			"A winda zamknęła drzwi i odmówiła zjazdu.",
			"Choć żarówka mrugała alfabetem Morse'a po pomoc.",
			"Aż radio nadało komunikat o katastrofie estetycznej.",
			"A kosz odsunął się, żeby uniknąć porównania.",
			"Bo mucha na suficie udawała martwą z zażenowania.",
			"Więc dywan próbował cię zwinąć, lecz ma standardy.",
			"A cisza złożyła wypowiedzenie bez okresu ochronnego.",
			"Choć zegar cofnął wskazówki, by nie firmować tej minuty.",
			"Aż gaśnica spadła ze ściany na widok tej sceny.",
			"A sufit rozważał zawalenie jako rozsądną ucieczkę.",
			"Bo podłoga obniżyła się pod ciężarem żenady.",
			"Więc drzwi awaryjne zaprosiły wszystkich oprócz ciebie.",
		},
	},
	{
		name: "savage",
		tier: 3,
		openings: []string{
			"Ten prompt to pieprzony wrak, który nadal żąda pierwszeństwa w porcie.",
			"Znowu serwujesz gówno w pudełku i pytasz, gdzie jest kokarda.",
			"Twój wysiłek zniknął szybciej niż sens z tego pytania.",
			"To nie jest zadanie, tylko kompletny burdel z polem tekstowym.",
			"Próbujesz trzeci raz i nadal omijasz każdą użyteczną informację.",
			"Ten argument jest martwy, lecz pewność siebie nadal ciągnie go po pokładzie.",
			"Odwalasz cyfrową fuszerkę i oczekujesz fanfar za sam hałas.",
			"To pytanie ma jakość instrukcji napisanej łokciem podczas alarmu.",
			"Twój plan rozpadł się przed startem i jeszcze wystawia rachunek.",
			"Zamieniłeś prostą sprawę w pieprzony festiwal zbędnego chaosu.",
			"Ten tekst obraża podstawową ideę przygotowania.",
			"Wkładasz zero porządku i żądasz wyniku klasy premium.",
			"To wygląda jak sabotaż wykonany bez potrzebnej staranności.",
			"Twoja metoda polega na powtarzaniu błędu z większą pewnością.",
			"Nawet awaria ma więcej struktury niż ten cholerny wywód.",
		},
		punchlines: []string{
			"A publiczność spierdalała tak szybko, że płonęły podeszwy.",
			"Bo mewa nasrała obok i podniosła poziom występu.",
			"Więc automat z kawą wypluł monetę jako odszkodowanie.",
			"A roślina zdechła, a doniczka natychmiast poprosiła o azyl.",
			"Choć pokładowe szczury zatopiły statek, by skrócić widowisko.",
			"Aż rynsztok pozwał cię za bezczelne podszywanie się pod markę.",
			"A karton spod śmietnika dostał nagłego poczucia przewagi.",
			"Bo kalendarz wyrwał tę kartkę i spalił ją bez ceremonii.",
			"Więc lustro rozbiło się, żeby nigdy tego nie odbijać.",
			"Jakby krzesło wolało piec niż dalszy udział w tym cyrku.",
			"A winda przecięła kabel, bo piwnica była zbyt wysoko.",
			"Choć żarówka eksplodowała z pieprzonej odmowy uczestnictwa.",
			"Aż radio nadało sygnał alarmowy i przeszło na emeryturę.",
			"A śmietnik zatrzasnął klapę przed takim odpadem.",
			"Bo mucha zdechła w locie, żeby nie zostać sekundę dłużej.",
			"Więc dywan próbował wpełznąć pod własny spód.",
			"A cisza zwymiotowała, chociaż nie ma żołądka.",
			"Choć zegar odmówił działania, żeby nie było świadków.",
			"Aż gaśnica uznała tę żenadę za niegodną piany.",
			"A sufit pękł ze śmiechu i szybko przeszedł w żałobę.",
			"Bo podłoga otworzyła szczelinę, lecz piekło odmówiło odbioru.",
			"Więc drzwi awaryjne wystrzeliły z zawiasów razem z klamką.",
		},
	},
}

func main() {
	flag.Parse()

	replies, err := generate()
	if err != nil {
		fatal(err)
	}

	data, err := json.MarshalIndent(replies, "", "  ")
	if err != nil {
		fatal(err)
	}
	data = append(data, '\n')

	if *check {
		if err := checkFile("assets/replies.pl.json", data); err != nil {
			fatal(err)
		}
		return
	}

	if err := write("assets/replies.pl.json", data); err != nil {
		fatal(err)
	}
}

func generate() ([]reply, error) {
	expectedCount := 1000 + len(typedSets)*300
	result := make([]reply, 0, expectedCount)
	seenIDs := make(map[string]struct{}, expectedCount)
	seenTexts := make(map[string]struct{}, expectedCount)

	for _, set := range sets {
		sequence := 0
		for openingIndex, opening := range set.openings {
			for punchlineIndex, punchline := range set.punchlines {
				sequence++
				text := composeReply(opening, punchline)
				entry := reply{
					ID:               fmt.Sprintf("%s-%03d", set.name, sequence),
					Locale:           "pl",
					Category:         "generic",
					Tier:             set.tier,
					OpeningVariant:   openingIndex + 1,
					PunchlineVariant: punchlineIndex + 1,
					Text:             text,
				}
				if err := appendUnique(&result, seenIDs, seenTexts, entry); err != nil {
					return nil, err
				}
			}
		}
	}

	counts := make(map[string]int, len(typedSets)*3)
	for _, set := range typedSets {
		for tierIndex, openings := range set.openings {
			tier := tierIndex + 1
			if len(openings) != 10 || len(set.punchlines) != 10 {
				return nil, fmt.Errorf("category %s tier %d has %d openings and %d punchlines, want 10 each", set.category, tier, len(openings), len(set.punchlines))
			}
			sequence := 0
			for openingIndex, opening := range openings {
				for punchlineIndex, punchline := range set.punchlines {
					sequence++
					entry := reply{
						ID:               fmt.Sprintf("%s-%d-%03d", set.category, tier, sequence),
						Locale:           "pl",
						Category:         set.category,
						Tier:             tier,
						OpeningVariant:   openingIndex + 1,
						PunchlineVariant: punchlineIndex + 1,
						Text:             composeReply(opening, punchline),
					}
					if err := appendUnique(&result, seenIDs, seenTexts, entry); err != nil {
						return nil, err
					}
					counts[fmt.Sprintf("%s-%d", set.category, tier)]++
				}
			}
		}
	}

	if len(result) != expectedCount {
		return nil, fmt.Errorf("generated %d replies, want %d", len(result), expectedCount)
	}
	for _, set := range typedSets {
		for tier := 1; tier <= 3; tier++ {
			key := fmt.Sprintf("%s-%d", set.category, tier)
			if counts[key] != 100 {
				return nil, fmt.Errorf("category %s tier %d has %d replies, want 100", set.category, tier, counts[key])
			}
		}
	}
	return result, nil
}

func composeReply(opening string, punchline string) string {
	opening = trimSentenceEnd(opening)
	punchline = trimFinisherLink(lowerInitial(trimSentenceEnd(punchline)))
	return opening + " - " + punchline
}

func trimSentenceEnd(text string) string {
	return strings.TrimRight(strings.TrimSpace(text), ".!?…")
}

func lowerInitial(text string) string {
	first, firstSize := utf8.DecodeRuneInString(text)
	if firstSize == 0 {
		return text
	}
	second, _ := utf8.DecodeRuneInString(text[firstSize:])
	if !unicode.IsUpper(first) || unicode.IsUpper(second) {
		return text
	}
	return string(unicode.ToLower(first)) + text[firstSize:]
}

func trimFinisherLink(text string) string {
	for _, link := range []string{"a ", "aż ", "bo ", "choć ", "więc "} {
		if strings.HasPrefix(text, link) {
			return strings.TrimPrefix(text, link)
		}
	}
	return text
}

func appendUnique(result *[]reply, seenIDs, seenTexts map[string]struct{}, entry reply) error {
	if _, exists := seenIDs[entry.ID]; exists {
		return fmt.Errorf("duplicate reply ID %q", entry.ID)
	}
	if _, exists := seenTexts[entry.Text]; exists {
		return fmt.Errorf("duplicate reply text for %q", entry.ID)
	}
	seenIDs[entry.ID] = struct{}{}
	seenTexts[entry.Text] = struct{}{}
	*result = append(*result, entry)
	return nil
}

func write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s parent: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func checkFile(path string, expected []byte) error {
	actual, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if !bytes.Equal(actual, expected) {
		return fmt.Errorf("%s is stale; run mise run replies:generate", path)
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

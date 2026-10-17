package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
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
			"Publiczność właśnie spojrzała na zegarek z desperacją.",
			"Mewa za oknem pokręciła głową i odleciała bez słowa.",
			"Automat z kawą odmówił komentarza i zatrzasnął podajnik.",
			"W pobliskiej bibliotece jedna książka sama spadła ze wstydu.",
			"Pokładowy szczur poprosił o przeniesienie na inny statek.",
			"Nawet kurz na półce udaje, że tego nie słyszał.",
			"Jeden ziemniak w kuchni poczuł nagły przypływ wyższości.",
			"Kalendarz skreślił tę minutę, żeby chronić własną reputację.",
			"Lustro w korytarzu właśnie odwróciło się do ściany.",
			"Krzesło pod tobą rozważa zmianę właściciela.",
			"Winda pojechała na inne piętro bez naciskania przycisku.",
			"Światło przygasło, jakby pomieszczenie chciało zniknąć.",
			"Radio samo przełączyło stację na szum ostrzegawczy.",
			"Sąsiedni kosz zamknął klapę i zachował resztki godności.",
			"Jedna mucha przerwała lot, żeby okazać politowanie.",
			"Dywan zwija się powoli i szuka wyjścia.",
			"Nawet cisza zrobiła krok wstecz i założyła kask.",
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
			"Publiczność zamówiła taksówkę, zanim skończyło się pierwsze echo.",
			"Mewa wyrzuciła frytkę, bo straciła apetyt do tego widowiska.",
			"Automat biletowy wyświetlił litość zamiast ceny.",
			"W sąsiednim pokoju roślina zwiędła z drugiej ręki.",
			"Pokładowy szczur spakował ser i złożył wymówienie.",
			"Nawet rynsztok uznał ten poziom za przesadę.",
			"Karton przy śmietniku wyprostował się z poczucia wyższości.",
			"Kalendarz ogłosił ten dzień świętem cudzej cierpliwości.",
			"Lustro pękło, choć nikt go nawet nie dotknął.",
			"Krzesło skrzypi po cichu, jakby szykowało pozew do producenta.",
			"Winda zamknęła drzwi i odmówiła powrotu na to piętro.",
			"Żarówka mruga alfabetem Morse'a po pomoc.",
			"Radio nadało komunikat o lokalnej katastrofie estetycznej.",
			"Kosz na śmieci odsunął się, żeby uniknąć porównania.",
			"Mucha na suficie udaje martwą z czystego zażenowania.",
			"Dywan próbuje zwinąć także ciebie, lecz ma swoje standardy.",
			"Cisza właśnie złożyła wypowiedzenie bez okresu ochronnego.",
			"Zegar cofnął wskazówki, bo nie chce firmować tej minuty.",
			"Gaśnica sama spadła ze ściany na widok tego pożaru godności.",
			"Sufit rozważa zawalenie jako rozsądną drogę ucieczki.",
			"Podłoga obniżyła się o centymetr pod ciężarem żenady.",
			"Drzwi awaryjne otworzyły się same i zaprosiły wszystkich oprócz ciebie.",
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
			"Publiczność uciekła tak szybko, że zostały po niej płonące podeszwy.",
			"Mewa nasrała obok, żeby zaznaczyć wyższy poziom wypowiedzi.",
			"Automat z kawą wypluł monetę i nazwał ją odszkodowaniem.",
			"Roślina w sąsiednim pokoju zdechła, a doniczka prosi o azyl.",
			"Pokładowe szczury zatopiły statek, żeby skrócić to przedstawienie.",
			"Rynsztok właśnie pozwał cię za bezczelne podszywanie się pod jego markę.",
			"Karton spod śmietnika dostał erekcji od nagłego poczucia przewagi.",
			"Kalendarz wyrwał tę kartkę i spalił ją bez ceremonii.",
			"Lustro rozbiło się samo, żeby nigdy więcej tego nie odbijać.",
			"Krzesło woli trafić do pieca niż dalej wspierać ten cyrk.",
			"Winda przecięła kabel, bo piwnica nadal była zbyt wysoko.",
			"Żarówka eksplodowała z czystej, pieprzonej odmowy uczestnictwa.",
			"Radio nadało sygnał alarmowy i przeszło na emeryturę.",
			"Śmietnik zatrzasnął klapę, bo nie przyjmuje odpadów tej klasy.",
			"Mucha zdechła w locie, żeby nie spędzić tu kolejnej sekundy.",
			"Dywan próbuje wpełznąć pod własny spód i zniknąć.",
			"Cisza zwymiotowała, chociaż nie ma nawet żołądka.",
			"Zegar popełnił awarię, żeby ta chwila nie miała świadków.",
			"Gaśnica uznała ten pożar godności za niegodny piany.",
			"Sufit pękł ze śmiechu, lecz szybko przeszedł w żałobę.",
			"Podłoga otworzyła szczelinę, ale piekło odmówiło odbioru.",
			"Drzwi awaryjne wystrzeliły z zawiasów i zabrały ze sobą klamkę.",
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
	result := make([]reply, 0, 4000)
	seenIDs := make(map[string]struct{}, 4000)
	seenTexts := make(map[string]struct{}, 4000)

	for _, set := range sets {
		sequence := 0
		for openingIndex, opening := range set.openings {
			for punchlineIndex, punchline := range set.punchlines {
				sequence++
				text := opening + " " + punchline
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
						Text:             opening + " " + punchline,
					}
					if err := appendUnique(&result, seenIDs, seenTexts, entry); err != nil {
						return nil, err
					}
					counts[fmt.Sprintf("%s-%d", set.category, tier)]++
				}
			}
		}
	}

	if len(result) != 4000 {
		return nil, fmt.Errorf("generated %d replies, want 4000", len(result))
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

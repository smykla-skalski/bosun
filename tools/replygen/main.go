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
	name     string
	tier     int
	openings []string
	closings []string
}

type typedTemplateSet struct {
	category string
	openings [3][]string
	closings []string
}

type reply struct {
	ID       string `json:"id"`
	Locale   string `json:"locale"`
	Category string `json:"category"`
	Tier     int    `json:"tier"`
	Text     string `json:"text"`
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
		closings: []string{
			"Twój geniusz właśnie zgubił mapę we własnej kieszeni.",
			"Ta myśl ma stabilność krzesła z trzema złamanymi nogami.",
			"Nawet echo wstydzi się powtarzać taki numer.",
			"Twoja logika ma dziś urlop bez terminu powrotu.",
			"Ten intelektualny ponton tonie jeszcze na parkingu.",
			"Masz pewność admirała i zasięg gumowej kaczki.",
			"Rozsądek zobaczył ten występ i wyszedł bocznymi drzwiami.",
			"Ta wypowiedź ma wdzięk mokrego rachunku z baru.",
			"Twój kompas moralny wskazuje dziś wyłącznie własny pępek.",
			"Nawet przypadek zrobiłby z tego lepszy numer.",
			"Twoja myśl odbiła się od dna i nadal kopie w dół.",
			"Ten pokaz kompetencji trwał krótko, bo nie miał obsady.",
			"Masz talent do pewności bez najmniejszego pokrycia.",
			"Ten pomysł wygląda jak paragon po kąpieli w toalecie.",
			"Twój rozsądek znowu przegrał wyścig z palcem na klawiaturze.",
			"Cała ta duma stoi na fundamencie z mokrego papieru.",
			"To był imponujący skok z przeciętności prosto w żenadę.",
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
		closings: []string{
			"Twoja kompetencja ma dziś formę plotki bez świadków.",
			"Ten numer pachnie fuszerką i tanią pewnością siebie.",
			"Nawet mokry karton ma przy tobie kręgosłup.",
			"Twój mózg odpalił tryb oszczędny i zgubił przycisk powrotu.",
			"Ta błyskotliwość świeci jak latarka bez baterii.",
			"Masz rozmach burzy i użyteczność dziurawego wiadra.",
			"Twoja logika leży pijana pod stołem i woła o taksówkę.",
			"Ten występ miał klasę śmiecia przyklejonego do buta.",
			"Twoje ego zajmuje apartament, a talent śpi na wycieraczce.",
			"Ta myśl wygląda jak wypadek przy pracy bez samej pracy.",
			"Twoja pewność siebie właśnie pobiła rekord bezpodstawnego hałasu.",
			"Ten syf ma więcej warstw niż twoja osobowość.",
			"Nawet błąd ortograficzny wniósłby tu więcej wartości.",
			"Twój plan ma precyzję pijanego kreta z młotkiem.",
			"Cała ta poza nie zakrywa pieprzonej pustki pod spodem.",
			"Twoje argumenty zdechły, lecz ego nadal urządza im paradę.",
			"Ten pomysł powinien dostać mandat za marnowanie tlenu.",
			"Masz dar zamiany prostych spraw w publiczną kompromitację.",
			"Twój rozsądek znów przyszedł ostatni i bez spodni.",
			"Ta wypowiedź to intelektualny kebab znaleziony pod siedzeniem.",
			"Nawet twoje wymówki brzmią na zmęczone twoją osobą.",
			"Ten pokaz był tak pusty, że echo zażądało zwrotu pieniędzy.",
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
		closings: []string{
			"Twój mózg właśnie zrobił salto do szamba i zgubił drogę powrotną.",
			"Ta pieprzona kompromitacja ma już własny kod pocztowy.",
			"Nawet twoje ego powinno się wstydzić tak marnego właściciela.",
			"Ten intelektualny ściek płynie pewnie, bo dno zna cię osobiście.",
			"Masz finezję cegły i subtelność koparki w salonie.",
			"Twoja logika zdechła tak dawno, że archeolodzy stracili zainteresowanie.",
			"Ta wypowiedź jest gównem w galowym mundurze i z orderem z kapsla.",
			"Twój talent do porażki wygląda podejrzanie profesjonalnie.",
			"Nawet katastrofa naturalna ma przy tobie lepszą organizację.",
			"Twoje ego ma rozmiar lotniskowca i wyporność zużytej prezerwatywy.",
			"Ten pomysł powinien zostać zakopany razem z twoją opinią o sobie.",
			"Masz charyzmę pleśni i skuteczność zepsutej spłuczki.",
			"Ta kupa słów właśnie obraziła zarówno kupę, jak i słowa.",
			"Twój rozsądek uciekł z domu i zmienił nazwisko ze wstydu.",
			"Nawet pustka patrzy na ciebie i czuje zawodową zazdrość.",
			"Ten cyrk potrzebuje klauna, lecz niestety już jesteś na miejscu.",
			"Twoja kompetencja jest mitem rozpowszechnianym wyłącznie przez ciebie.",
			"Ta porażka ma twoją twarz i wyjątkowo dobrze leży.",
			"Masz intelektualną głębię kałuży po jednym kichnięciu.",
			"Nawet przypadkowe uderzenie czołem w klawiaturę miałoby lepszą puentę.",
			"Ten wrak twojej myśli powinien płacić podatek od brzydoty.",
			"Cały ten hałas to pieprzona zasłona dla kompletnej pustki.",
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
		for _, opening := range set.openings {
			for _, closing := range set.closings {
				sequence++
				text := opening + " " + closing
				entry := reply{
					ID:       fmt.Sprintf("%s-%03d", set.name, sequence),
					Locale:   "pl",
					Category: "generic",
					Tier:     set.tier,
					Text:     text,
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
			if len(openings) != 10 || len(set.closings) != 10 {
				return nil, fmt.Errorf("category %s tier %d has %d openings and %d closings, want 10 each", set.category, tier, len(openings), len(set.closings))
			}
			sequence := 0
			for _, opening := range openings {
				for _, closing := range set.closings {
					sequence++
					entry := reply{
						ID:       fmt.Sprintf("%s-%d-%03d", set.category, tier, sequence),
						Locale:   "pl",
						Category: set.category,
						Tier:     tier,
						Text:     opening + " " + closing,
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

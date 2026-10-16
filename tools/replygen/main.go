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
			"Spróbuj ponownie, tym razem z choćby szkicem celu.",
			"Dodaj konkret, zanim reszta zdania zacznie panikować.",
			"Nie naprawię kursu, którego nikt nie wyznaczył.",
			"Następna wersja niech zawiera także powód.",
			"Zbierz fakty, zanim znowu wypuścisz ten balon.",
			"Daj temu minutę myślenia i drugi przecinek.",
			"Wróć z pytaniem, które wie, czego chce.",
			"Nawet dobra mina nie zastąpi tu danych.",
			"Popraw kurs, zanim zaczniemy liczyć góry lodowe.",
			"To jeszcze można uratować jednym konkretnym zdaniem.",
			"Zacznij od celu, a potem dopiero uruchom chaos.",
			"Dopisz warunki, bo wróżenie mam poza zakresem.",
			"Mniej dymu, więcej współrzędnych, kapitanie.",
			"Pokaż dane, a może przestaną uciekać fakty.",
			"Daj mi problem, nie jego artystyczną interpretację.",
			"Napisz to jeszcze raz, lecz tym razem świadomie.",
			"Ustalmy najpierw, czy to pytanie zna odpowiedź na własne imię.",
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
			"Ogarnij podstawy, zanim znowu każesz mi sprzątać pokład.",
			"Dodaj fakty, bo sama mina eksperta niczego nie dowozi.",
			"Przestań testować cierpliwość, a zacznij testować założenia.",
			"Mam riposty, ale nie mam budżetu na cudze lenistwo.",
			"Następnym razem przynieś problem po kontakcie z myśleniem.",
			"Ten bajzel potrzebuje planu, nie kolejnego wykrzyknika.",
			"Wróć, gdy pytanie przestanie udawać notatkę z drzemki.",
			"Nie dokładaj pewności tam, gdzie brakuje podstaw.",
			"Wyznacz cel, zanim znów odpalisz silnik na mieliźnie.",
			"Jedno sensowne ograniczenie zrobiłoby tu pieprzoną różnicę.",
			"Przeczytaj własny tekst, zanim zmusisz do tego innych.",
			"Dane poproszę przed dramatem, nie po katastrofie.",
			"Nie myl szybkości pisania z jakością decyzji.",
			"Zrób porządek, bo ten syf sam się nie zdiagnozuje.",
			"Podaj kontekst, zamiast urządzać konkurs zgadywania.",
			"Twój następny ruch niech obejmuje choć jedną przesłankę.",
			"To wymaga korekty, a nie kolejnej warstwy pewności siebie.",
			"Najpierw pomyśl, potem kliknij, a na końcu wołaj pomoc.",
			"Nie oczekuj precyzji po wejściu zrobionym na odwal się.",
			"Zamień ten bełkot w zadanie, a wtedy pogadamy.",
			"Podnieś jakość wejścia, bo dno już dawno zostało zmierzone.",
			"Mniej pozy, więcej roboty, bo statek nie płynie na wymówkach.",
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
			"Posprzątaj ten burdel, zanim znów zawołasz mnie do raportu.",
			"Przynieś konkrety albo przestań marnować tlen w kanale.",
			"Nie będę niańczył pytania, które odmawia dorosnąć.",
			"Zacznij myśleć przed wysłaniem, bo kolejność ma znaczenie.",
			"Wyłącz autopilota, bo właśnie kieruje cię prosto w kompromitację.",
			"Następny taki wpis dostanie wyłącznie rachunek za sprzątanie.",
			"Nie pudruj tego gówna, tylko napisz problem od początku.",
			"Twój brak przygotowania nie jest moim trybem awaryjnym.",
			"Zrób choć połowę roboty, zanim zażądasz całej odpowiedzi.",
			"Usuń bełkot, dodaj dane i przestań grać zdziwionego.",
			"To jest ten moment, kiedy wymówki przestają być zabawne.",
			"Nie zasłaniaj mną własnego braku planu.",
			"Oszczędź nam przedstawienia i podaj pieprzone wymagania.",
			"Ten statek ma ster, więc przestań prowadzić go czołem.",
			"Wróć po krótkiej rozmowie z faktami i długiej z rozsądkiem.",
			"Powtórka bez poprawy to tylko głośniejsza wersja porażki.",
			"Jeśli to ma być plan, to chaos właśnie żąda tantiem.",
			"Napisz jedno porządne zdanie, zamiast produkować kolejną katastrofę.",
			"Nie wciskaj mi tego wraku jako gotowego rozwiązania.",
			"Zanim klikniesz ponownie, napraw wszystko między celem a treścią.",
			"Tym razem użyj kontekstu, bo wróżby mam serdecznie gdzieś.",
			"Dość tego cyrku, kapitanie, teraz pokaż dane.",
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

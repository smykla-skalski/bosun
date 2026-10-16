package replies

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/smykla-skalski/bosun/assets"
)

type Tier uint8

const (
	TierSharp  Tier = 1
	TierHarsh  Tier = 2
	TierSavage Tier = 3

	minimumTierReplies = recentCapacity + CandidateCount
)

type Reply struct {
	ID       string   `json:"id"`
	Locale   string   `json:"locale"`
	Category Category `json:"category"`
	Tier     Tier     `json:"tier"`
	Text     string   `json:"text"`
}

type Catalog struct {
	entries        []Reply
	byCategoryTier [categoryCount][4][]Reply
	tierCounts     [4]int
}

var (
	embeddedOnce    sync.Once
	embeddedCatalog Catalog
	embeddedErr     error

	modelURLPattern       = regexp.MustCompile(`(?i)(?:https?://|www\.|\b[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+(?::[0-9]{1,5})?(?:[/?:#][^\s]*)?)`)
	modelMentionPattern   = regexp.MustCompile(`(?i)(?:<@!?[0-9]+>|<@&[0-9]+>|<#[0-9]+>|@everyone\b|@here\b)`)
	modelEmailPattern     = regexp.MustCompile(`(?i)\b[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}\b`)
	modelLongDigitPattern = regexp.MustCompile(`[0-9]{7,}`)
	modelServicePattern   = regexp.MustCompile(`(?i)(?:bosun|discord|smykland|openrouter)`)
)

var ErrUnsafeModelCandidate = errors.New("unsafe model candidate")

func LoadEmbedded() (Catalog, error) {
	embeddedOnce.Do(func() {
		var entries []Reply
		if err := json.Unmarshal([]byte(assets.PolishReplies()), &entries); err != nil {
			embeddedErr = fmt.Errorf("decode embedded reply catalog: %w", err)
			return
		}

		embeddedCatalog, embeddedErr = NewCatalog(entries)
		if embeddedErr != nil {
			embeddedErr = fmt.Errorf("validate embedded reply catalog: %w", embeddedErr)
			return
		}

		embeddedErr = validateEmbeddedCounts(embeddedCatalog)
	})
	return embeddedCatalog, embeddedErr
}

func NewCatalog(entries []Reply) (Catalog, error) {
	if len(entries) == 0 {
		return Catalog{}, errors.New("reply catalog is empty")
	}

	catalog := Catalog{entries: make([]Reply, len(entries))}
	copy(catalog.entries, entries)

	ids := make(map[string]struct{}, len(entries))
	texts := make(map[string]struct{}, len(entries))
	for index, entry := range catalog.entries {
		if err := validateReply(entry); err != nil {
			return Catalog{}, fmt.Errorf("reply %d: %w", index, err)
		}
		if _, exists := ids[entry.ID]; exists {
			return Catalog{}, fmt.Errorf("reply %d: duplicate ID %q", index, entry.ID)
		}
		if _, exists := texts[entry.Text]; exists {
			return Catalog{}, fmt.Errorf("reply %d: duplicate text", index)
		}
		ids[entry.ID] = struct{}{}
		texts[entry.Text] = struct{}{}
		category := categoryIndex(entry.Category)
		catalog.byCategoryTier[category][entry.Tier] = append(catalog.byCategoryTier[category][entry.Tier], entry)
		catalog.tierCounts[entry.Tier]++
	}

	for tier := TierSharp; tier <= TierSavage; tier++ {
		if count := len(catalog.byCategoryTier[0][tier]); count < minimumTierReplies {
			return Catalog{}, fmt.Errorf("category %q tier %d has %d replies, need at least %d", CategoryGeneric, tier, count, minimumTierReplies)
		}
	}
	for categoryIndex, byTier := range catalog.byCategoryTier[1:] {
		categoryHasReplies := false
		for tier := TierSharp; tier <= TierSavage; tier++ {
			categoryHasReplies = categoryHasReplies || len(byTier[tier]) > 0
		}
		if !categoryHasReplies {
			continue
		}
		for tier := TierSharp; tier <= TierSavage; tier++ {
			count := len(byTier[tier])
			if count < minimumTierReplies {
				return Catalog{}, fmt.Errorf("category %q tier %d has %d replies, need at least %d", categories[categoryIndex+1], tier, count, minimumTierReplies)
			}
		}
	}
	return catalog, nil
}

func (c Catalog) Len() int {
	return len(c.entries)
}

func (c Catalog) Count(tier Tier) int {
	if tier < TierSharp || tier > TierSavage {
		return 0
	}
	return c.tierCounts[tier]
}

func (c Catalog) CountCategory(category Category, tier Tier) int {
	if tier < TierSharp || tier > TierSavage {
		return 0
	}
	return len(c.byCategoryTier[categoryIndex(normalizeCategory(category))][tier])
}

func (c Catalog) candidates(category Category, tier Tier) []Reply {
	return c.byCategoryTier[categoryIndex(normalizeCategory(category))][tier]
}

func ValidateModelCandidate(text string, forbiddenValues ...string) error {
	switch {
	case strings.TrimSpace(text) == "":
		return fmt.Errorf("%w: text is empty", ErrUnsafeModelCandidate)
	case modelMentionPattern.MatchString(text):
		return fmt.Errorf("%w: text contains mention syntax", ErrUnsafeModelCandidate)
	case modelEmailPattern.MatchString(text):
		return fmt.Errorf("%w: text contains an email form", ErrUnsafeModelCandidate)
	case modelURLPattern.MatchString(text):
		return fmt.Errorf("%w: text contains a URL", ErrUnsafeModelCandidate)
	case modelLongDigitPattern.MatchString(text):
		return fmt.Errorf("%w: text contains a long digit sequence", ErrUnsafeModelCandidate)
	case modelServicePattern.MatchString(text):
		return fmt.Errorf("%w: text contains a forbidden service name", ErrUnsafeModelCandidate)
	}

	lowerText := strings.ToLower(text)
	for _, value := range forbiddenValues {
		value = strings.TrimSpace(value)
		if value != "" && strings.Contains(lowerText, strings.ToLower(value)) {
			return fmt.Errorf("%w: text contains a forbidden value", ErrUnsafeModelCandidate)
		}
	}
	return nil
}

func validateReply(reply Reply) error {
	if strings.TrimSpace(reply.ID) == "" {
		return errors.New("ID is empty")
	}
	if reply.Locale != "pl" {
		return fmt.Errorf("locale is %q, want pl", reply.Locale)
	}
	if categoryIndex(reply.Category) < 0 {
		return fmt.Errorf("category %q is invalid", reply.Category)
	}
	if reply.Tier < TierSharp || reply.Tier > TierSavage {
		return fmt.Errorf("tier %d is invalid", reply.Tier)
	}
	if strings.TrimSpace(reply.Text) == "" {
		return errors.New("text is empty")
	}
	if err := ValidateModelCandidate(reply.Text); err != nil {
		return err
	}
	return nil
}

func validateEmbeddedCounts(catalog Catalog) error {
	if catalog.Len() != 4000 {
		return fmt.Errorf("reply count is %d, want 4000", catalog.Len())
	}
	expected := map[Tier]int{
		TierSharp:  1340,
		TierHarsh:  1330,
		TierSavage: 1330,
	}
	for tier, count := range expected {
		if actual := catalog.Count(tier); actual != count {
			return fmt.Errorf("tier %d count is %d, want %d", tier, actual, count)
		}
	}
	for _, category := range categories[1:] {
		for tier := TierSharp; tier <= TierSavage; tier++ {
			if actual := catalog.CountCategory(category, tier); actual != 100 {
				return fmt.Errorf("category %q tier %d count is %d, want 100", category, tier, actual)
			}
		}
	}
	return nil
}

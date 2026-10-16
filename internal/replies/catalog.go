package replies

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/smykla-skalski/bosun/assets"
)

type Tier uint8

const (
	TierSharp  Tier = 1
	TierHarsh  Tier = 2
	TierSavage Tier = 3
)

type Reply struct {
	ID       string `json:"id"`
	Locale   string `json:"locale"`
	Category string `json:"category"`
	Tier     Tier   `json:"tier"`
	Text     string `json:"text"`
}

type Catalog struct {
	entries []Reply
	byTier  [4][]Reply
}

var (
	embeddedOnce    sync.Once
	embeddedCatalog Catalog
	embeddedErr     error
)

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
		catalog.byTier[entry.Tier] = append(catalog.byTier[entry.Tier], entry)
	}

	for tier := TierSharp; tier <= TierSavage; tier++ {
		if len(catalog.byTier[tier]) == 0 {
			return Catalog{}, fmt.Errorf("tier %d is empty", tier)
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
	return len(c.byTier[tier])
}

func validateReply(reply Reply) error {
	if strings.TrimSpace(reply.ID) == "" {
		return errors.New("ID is empty")
	}
	if reply.Locale != "pl" {
		return fmt.Errorf("locale is %q, want pl", reply.Locale)
	}
	if reply.Category != "generic" {
		return fmt.Errorf("category is %q, want generic", reply.Category)
	}
	if reply.Tier < TierSharp || reply.Tier > TierSavage {
		return fmt.Errorf("tier %d is invalid", reply.Tier)
	}
	if strings.TrimSpace(reply.Text) == "" {
		return errors.New("text is empty")
	}
	return nil
}

func validateEmbeddedCounts(catalog Catalog) error {
	if catalog.Len() != 1000 {
		return fmt.Errorf("reply count is %d, want 1000", catalog.Len())
	}
	expected := map[Tier]int{
		TierSharp:  340,
		TierHarsh:  330,
		TierSavage: 330,
	}
	for tier, count := range expected {
		if actual := catalog.Count(tier); actual != count {
			return fmt.Errorf("tier %d count is %d, want %d", tier, actual, count)
		}
	}
	return nil
}

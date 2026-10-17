package replies

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestSelectBatchReturnsReservedCategoryCandidates(t *testing.T) {
	t.Parallel()

	selector := testSelector(t)
	batch, err := selector.SelectBatch("user-1", CategoryComplaint)
	if err != nil {
		t.Fatalf("SelectBatch() error = %v", err)
	}

	ids := make(map[string]struct{}, CandidateCount)
	openings := make(map[uint8]struct{}, CandidateCount)
	punchlines := make(map[uint8]struct{}, CandidateCount)
	for _, selection := range batch.Selections {
		if selection.Reply.Category != CategoryComplaint {
			t.Errorf("reply category = %q, want %q", selection.Reply.Category, CategoryComplaint)
		}
		if selection.Tier != TierHarsh || selection.Reply.Tier != TierHarsh {
			t.Errorf("reply tier = %d and selection tier = %d, want %d", selection.Reply.Tier, selection.Tier, TierHarsh)
		}
		if selection.TriggerCount != 1 {
			t.Errorf("trigger count = %d, want 1", selection.TriggerCount)
		}
		if _, exists := ids[selection.Reply.ID]; exists {
			t.Errorf("duplicate reply ID %q", selection.Reply.ID)
		}
		ids[selection.Reply.ID] = struct{}{}
		if _, exists := openings[selection.Reply.OpeningVariant]; exists {
			t.Errorf("duplicate opening variant %d", selection.Reply.OpeningVariant)
		}
		if _, exists := punchlines[selection.Reply.PunchlineVariant]; exists {
			t.Errorf("duplicate punchline variant %d", selection.Reply.PunchlineVariant)
		}
		openings[selection.Reply.OpeningVariant] = struct{}{}
		punchlines[selection.Reply.PunchlineVariant] = struct{}{}
	}

	next, err := selector.Select("user-1", CategoryComplaint)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if next.TriggerCount != 2 {
		t.Fatalf("Select() trigger count = %d, want 2", next.TriggerCount)
	}
	if _, exists := ids[next.Reply.ID]; exists {
		t.Fatalf("Select() returned reserved reply %q", next.Reply.ID)
	}
}

func TestCommitBatchRecordsOnlyChosenReply(t *testing.T) {
	t.Parallel()

	selector := testSelector(t)
	batch, err := selector.SelectBatch("user-1", CategoryGeneric)
	if err != nil {
		t.Fatalf("SelectBatch() error = %v", err)
	}
	want := batch.Selections[3]
	batch.Selections[3] = batch.Selections[4]

	got, err := selector.CommitBatch(batch, 3)
	if err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}
	if got.Reply.ID != want.Reply.ID {
		t.Fatalf("CommitBatch() reply = %q, want %q", got.Reply.ID, want.Reply.ID)
	}
	state := selector.users["user-1"]
	if state.recentCount != 1 || state.recent[0] != want.Reply.ID {
		t.Fatalf("recent replies = %v count %d, want only %q", state.recent, state.recentCount, want.Reply.ID)
	}
	if selector.reservationCount != 0 || reservedIDCount(selector) != 0 || len(selector.reservedUsers) != 0 {
		t.Fatal("reservation remains after commit")
	}
}

func TestCancelBatchReleasesWithoutRecentReply(t *testing.T) {
	t.Parallel()

	selector := testSelector(t)
	batch, err := selector.SelectBatch("user-1", CategoryGeneric)
	if err != nil {
		t.Fatalf("SelectBatch() error = %v", err)
	}
	if err := selector.CancelBatch(batch); err != nil {
		t.Fatalf("CancelBatch() error = %v", err)
	}
	if selector.users["user-1"].recentCount != 0 {
		t.Fatal("CancelBatch() recorded a recent reply")
	}
	if selector.reservationCount != 0 || reservedIDCount(selector) != 0 || len(selector.reservedUsers) != 0 {
		t.Fatal("reservation remains after cancel")
	}
}

func TestSelectBatchSupportsTwentyOneDisplayedChoices(t *testing.T) {
	t.Parallel()

	selector := minimalBatchSelector(t)
	chosen := make(map[string]struct{}, recentCapacity+1)
	for range recentCapacity + 1 {
		batch, err := selector.SelectBatch("user-1", CategoryGeneric)
		if err != nil {
			t.Fatalf("SelectBatch() error = %v", err)
		}
		selection, err := selector.CommitBatch(batch, 0)
		if err != nil {
			t.Fatalf("CommitBatch() error = %v", err)
		}
		if _, exists := chosen[selection.Reply.ID]; exists {
			t.Fatalf("displayed recent reply %q twice", selection.Reply.ID)
		}
		chosen[selection.Reply.ID] = struct{}{}
	}
	if len(chosen) != recentCapacity+1 {
		t.Fatalf("displayed reply count = %d, want %d", len(chosen), recentCapacity+1)
	}
	if got := selector.users["user-1"].recentCount; got != recentCapacity {
		t.Fatalf("recent reply count = %d, want %d", got, recentCapacity)
	}
}

func TestSelectBatchReservationsDoNotOverlap(t *testing.T) {
	t.Parallel()

	selector := testSelector(t)
	first, err := selector.SelectBatch("user-1", CategoryChallenge)
	if err != nil {
		t.Fatalf("SelectBatch() first error = %v", err)
	}
	second, err := selector.SelectBatch("user-1", CategoryChallenge)
	if err != nil {
		t.Fatalf("SelectBatch() second error = %v", err)
	}

	ids := make(map[string]struct{}, CandidateCount)
	for _, selection := range first.Selections {
		ids[selection.Reply.ID] = struct{}{}
	}
	for _, selection := range second.Selections {
		if _, exists := ids[selection.Reply.ID]; exists {
			t.Fatalf("overlapping reserved reply %q", selection.Reply.ID)
		}
	}
	if err := selector.CancelBatch(first); err != nil {
		t.Fatalf("CancelBatch() first error = %v", err)
	}
	if err := selector.CancelBatch(second); err != nil {
		t.Fatalf("CancelBatch() second error = %v", err)
	}
}

func TestSelectBatchReservationsAllowDifferentUsers(t *testing.T) {
	t.Parallel()

	selector := testSelector(t)
	const userCount = 13
	batches := make([]Batch, 0, userCount)
	for index := range userCount {
		batch, err := selector.SelectBatch(fmt.Sprintf("user-%02d", index), CategoryComplaint)
		if err != nil {
			t.Fatalf("SelectBatch() user %d error = %v", index, err)
		}
		batches = append(batches, batch)
	}

	for _, batch := range batches {
		if err := selector.CancelBatch(batch); err != nil {
			t.Fatalf("CancelBatch() error = %v", err)
		}
	}
}

func TestSelectBatchRejectsInvalidTokenAndChoice(t *testing.T) {
	t.Parallel()

	selector := testSelector(t)
	if _, err := selector.CommitBatch(Batch{}, 0); !errors.Is(err, ErrInvalidBatchToken) {
		t.Fatalf("CommitBatch() error = %v, want ErrInvalidBatchToken", err)
	}
	if err := selector.CancelBatch(Batch{}); !errors.Is(err, ErrInvalidBatchToken) {
		t.Fatalf("CancelBatch() error = %v, want ErrInvalidBatchToken", err)
	}

	batch, err := selector.SelectBatch("user-1", CategoryGeneric)
	if err != nil {
		t.Fatalf("SelectBatch() error = %v", err)
	}
	for _, choice := range []int{-1, CandidateCount} {
		if _, err := selector.CommitBatch(batch, choice); !errors.Is(err, ErrInvalidBatchChoice) {
			t.Errorf("CommitBatch(choice %d) error = %v, want ErrInvalidBatchChoice", choice, err)
		}
	}
	if err := selector.CancelBatch(batch); err != nil {
		t.Fatalf("CancelBatch() error = %v", err)
	}
	if _, err := selector.CommitBatch(batch, 0); !errors.Is(err, ErrInvalidBatchToken) {
		t.Fatalf("CommitBatch() after cancel error = %v, want ErrInvalidBatchToken", err)
	}
}

func TestSelectBatchHasBoundedReservations(t *testing.T) {
	t.Parallel()

	selector := testSelector(t)
	batches := make([]Batch, 0, maximumReservations)
	for index := range maximumReservations {
		category := categories[index%len(categories)]
		batch, err := selector.SelectBatch(fmt.Sprintf("user-%02d", index), category)
		if err != nil {
			t.Fatalf("SelectBatch() call %d error = %v", index+1, err)
		}
		batches = append(batches, batch)
	}
	if _, err := selector.SelectBatch("overflow-user", CategoryGeneric); !errors.Is(err, ErrReservationCapacity) {
		t.Fatalf("SelectBatch() overflow error = %v, want ErrReservationCapacity", err)
	}
	for _, batch := range batches {
		if err := selector.CancelBatch(batch); err != nil {
			t.Fatalf("CancelBatch() error = %v", err)
		}
	}
}

func TestSelectBatchUsesGenericForInvalidCategory(t *testing.T) {
	t.Parallel()

	selector := testSelector(t)
	batch, err := selector.SelectBatch("user-1", Category("unknown"))
	if err != nil {
		t.Fatalf("SelectBatch() error = %v", err)
	}
	defer func() {
		if err := selector.CancelBatch(batch); err != nil {
			t.Errorf("CancelBatch() error = %v", err)
		}
	}()
	for _, selection := range batch.Selections {
		if selection.Reply.Category != CategoryGeneric {
			t.Fatalf("reply category = %q, want %q", selection.Reply.Category, CategoryGeneric)
		}
	}
}

func TestSelectBatchReturnsSafeCandidates(t *testing.T) {
	t.Parallel()

	selector := testSelector(t)
	batch, err := selector.SelectBatch("user-1", CategoryGeneric)
	if err != nil {
		t.Fatalf("SelectBatch() error = %v", err)
	}
	defer func() {
		if err := selector.CancelBatch(batch); err != nil {
			t.Errorf("CancelBatch() error = %v", err)
		}
	}()
	for _, selection := range batch.Selections {
		if err := ValidateModelCandidate(selection.Reply.Text); err != nil {
			t.Errorf("ValidateModelCandidate(%q) error = %v", selection.Reply.ID, err)
		}
	}
}

func TestSelectBatchSupportsConcurrentUse(t *testing.T) {
	t.Parallel()

	selector := testSelector(t)
	const goroutines = 16
	const calls = 20
	errChannel := make(chan error, goroutines*calls)

	var wait sync.WaitGroup
	for worker := range goroutines {
		wait.Add(1)
		go func() {
			defer wait.Done()
			userID := fmt.Sprintf("batch-user-%d", worker)
			category := categories[1+worker%(len(categories)-1)]
			for call := range calls {
				batch, err := selector.SelectBatch(userID, category)
				if err != nil {
					errChannel <- err
					continue
				}
				if call%2 == 0 {
					_, err = selector.CommitBatch(batch, call%CandidateCount)
				} else {
					err = selector.CancelBatch(batch)
				}
				if err != nil {
					errChannel <- err
				}
			}
		}()
	}
	wait.Wait()
	close(errChannel)

	for err := range errChannel {
		t.Error(err)
	}
}

func BenchmarkSelectorSelectBatch(b *testing.B) {
	catalog, err := LoadEmbedded()
	if err != nil {
		b.Fatalf("LoadEmbedded() error = %v", err)
	}
	selector := NewSelector(catalog)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		batch, err := selector.SelectBatch("benchmark-batch-user", CategoryGeneric)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := selector.CommitBatch(batch, 0); err != nil {
			b.Fatal(err)
		}
	}
}

func minimalBatchSelector(t *testing.T) *Selector {
	t.Helper()

	repliesPerTier := minimumTierReplies
	entries := make([]Reply, 0, repliesPerTier*3)
	for tier := TierSharp; tier <= TierSavage; tier++ {
		for index := range repliesPerTier {
			entries = append(entries, Reply{
				ID:       fmt.Sprintf("minimal-%d-%02d", tier, index),
				Locale:   "pl",
				Category: CategoryGeneric,
				Tier:     tier,
				Text:     fmt.Sprintf("Bezpieczna odpowiedź %d %02d", tier, index),
			})
		}
	}
	catalog, err := NewCatalog(entries)
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	return NewSelector(catalog, WithIndex(func(int) int { return 0 }))
}

func reservedIDCount(selector *Selector) int {
	count := 0
	for _, ids := range selector.reservedIDs {
		count += len(ids)
	}
	return count
}

package replies

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestSelectorEscalatesByTriggerCount(t *testing.T) {
	t.Parallel()

	selector := testSelector(t, WithClock(func() time.Time {
		return time.Date(2026, time.October, 16, 12, 0, 0, 0, time.UTC)
	}))
	wants := []Tier{
		TierSharp,
		TierSharp,
		TierHarsh,
		TierHarsh,
		TierHarsh,
		TierSavage,
		TierSavage,
	}
	for index, want := range wants {
		selection, err := selector.Select("user-1")
		if err != nil {
			t.Fatalf("Select() call %d error = %v", index+1, err)
		}
		if selection.Tier != want {
			t.Errorf("Select() call %d tier = %d, want %d", index+1, selection.Tier, want)
		}
		wantCount := min(index+1, activityLimit)
		if selection.TriggerCount != wantCount {
			t.Errorf("Select() call %d count = %d, want %d", index+1, selection.TriggerCount, wantCount)
		}
	}
}

func TestSelectorResetsAfterQuietWindow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 16, 12, 0, 0, 0, time.UTC)
	selector := testSelector(t, WithClock(func() time.Time { return now }))
	for range activityLimit {
		if _, err := selector.Select("user-1"); err != nil {
			t.Fatalf("Select() error = %v", err)
		}
	}

	now = now.Add(defaultWindow)
	selection, err := selector.Select("user-1")
	if err != nil {
		t.Fatalf("Select() after quiet window error = %v", err)
	}
	if selection.Tier != TierSharp || selection.TriggerCount != 1 {
		t.Fatalf("Select() after quiet window = tier %d, count %d; want tier %d, count 1", selection.Tier, selection.TriggerCount, TierSharp)
	}
}

func TestSelectorKeepsRecentRepliesAfterQuietWindow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 16, 12, 0, 0, 0, time.UTC)
	selector := testSelector(t, WithClock(func() time.Time { return now }))
	first, err := selector.Select("user-1")
	if err != nil {
		t.Fatalf("Select() first error = %v", err)
	}

	now = now.Add(defaultWindow)
	second, err := selector.Select("user-1")
	if err != nil {
		t.Fatalf("Select() after quiet window error = %v", err)
	}
	if second.Tier != TierSharp || second.TriggerCount != 1 {
		t.Fatalf("Select() after quiet window = tier %d, count %d; want tier %d, count 1", second.Tier, second.TriggerCount, TierSharp)
	}
	if second.Reply.ID == first.Reply.ID {
		t.Fatalf("Select() repeated recent reply %q after quiet window", first.Reply.ID)
	}
}

func TestSelectorKeepsActiveUserState(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 16, 12, 0, 0, 0, time.UTC)
	selector := testSelector(t, WithClock(func() time.Time { return now }))
	for range 2 {
		if _, err := selector.Select("active-user"); err != nil {
			t.Fatalf("Select() active user error = %v", err)
		}
	}

	now = now.Add(9 * time.Minute)
	selection, err := selector.Select("active-user")
	if err != nil {
		t.Fatalf("Select() active user error = %v", err)
	}
	if selection.Tier != TierHarsh || selection.TriggerCount != 3 {
		t.Fatalf("active selection = tier %d, count %d; want tier %d, count 3", selection.Tier, selection.TriggerCount, TierHarsh)
	}
}

func TestSelectorCapsUsersWithEqualClockAccessOrder(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 16, 12, 0, 0, 0, time.UTC)
	selector := testSelector(t, WithClock(func() time.Time { return now }))
	for index := range maximumUsers {
		userID := fmt.Sprintf("user-%04d", index)
		if _, err := selector.Select(userID); err != nil {
			t.Fatalf("Select(%q) error = %v", userID, err)
		}
	}

	if _, err := selector.Select("user-0000"); err != nil {
		t.Fatalf("Select() refreshed user error = %v", err)
	}
	if _, err := selector.Select("overflow-user"); err != nil {
		t.Fatalf("Select() overflow user error = %v", err)
	}

	if got := len(selector.users); got != maximumUsers {
		t.Fatalf("user state count = %d, want %d", got, maximumUsers)
	}
	if _, exists := selector.users["user-0000"]; !exists {
		t.Fatal("recently refreshed user was evicted")
	}
	if _, exists := selector.users["user-0001"]; exists {
		t.Fatal("least recently seen user remains after capacity eviction")
	}
	if _, exists := selector.users["overflow-user"]; !exists {
		t.Fatal("new user is absent after capacity eviction")
	}
}

func TestSelectorExcludesTwentyMostRecentReplies(t *testing.T) {
	t.Parallel()

	selector := testSelector(t)
	recent := make([]string, 0, recentCapacity)
	for call := 0; call < 80; call++ {
		selection, err := selector.Select("user-1")
		if err != nil {
			t.Fatalf("Select() call %d error = %v", call+1, err)
		}
		for _, id := range recent {
			if selection.Reply.ID == id {
				t.Fatalf("Select() call %d repeated recent ID %q", call+1, id)
			}
		}
		recent = append(recent, selection.Reply.ID)
		if len(recent) > recentCapacity {
			recent = recent[1:]
		}
	}
}

func TestSelectorSupportsConcurrentUse(t *testing.T) {
	t.Parallel()

	selector := testSelector(t)
	const goroutines = 32
	const calls = 200
	errors := make(chan error, goroutines*calls)

	var wait sync.WaitGroup
	for worker := range goroutines {
		wait.Add(1)
		go func() {
			defer wait.Done()
			userID := fmt.Sprintf("user-%d", worker%4)
			for range calls {
				selection, err := selector.Select(userID)
				if err != nil {
					errors <- err
					continue
				}
				if selection.Reply.Tier != selection.Tier {
					errors <- fmt.Errorf("reply tier %d differs from selection tier %d", selection.Reply.Tier, selection.Tier)
				}
			}
		}()
	}
	wait.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

func TestSelectorWarmPathDoesNotAllocate(t *testing.T) {
	catalog, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded() error = %v", err)
	}
	selector := NewSelector(catalog, WithIndex(func(int) int { return 0 }))
	for range activityLimit {
		if _, err := selector.Select("user-1"); err != nil {
			t.Fatalf("Select() warmup error = %v", err)
		}
	}

	allocations := testing.AllocsPerRun(1000, func() {
		if _, err := selector.Select("user-1"); err != nil {
			panic(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("Select() allocations = %.2f, want 0", allocations)
	}
}

func BenchmarkSelectorSelect(b *testing.B) {
	catalog, err := LoadEmbedded()
	if err != nil {
		b.Fatalf("LoadEmbedded() error = %v", err)
	}
	selector := NewSelector(catalog)
	for range activityLimit {
		if _, err := selector.Select("benchmark-user"); err != nil {
			b.Fatalf("Select() warmup error = %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := selector.Select("benchmark-user"); err != nil {
			b.Fatal(err)
		}
	}
}

func testSelector(t *testing.T, options ...SelectorOption) *Selector {
	t.Helper()

	catalog, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded() error = %v", err)
	}
	options = append(options, WithIndex(func(int) int { return 0 }))
	return NewSelector(catalog, options...)
}

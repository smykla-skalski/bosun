package replies

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"
)

const (
	defaultWindow  = 10 * time.Minute
	maximumUsers   = 4096
	recentCapacity = 20
	activityLimit  = 6
)

var ErrEmptyUserID = errors.New("user ID is empty")

type Selection struct {
	Reply        Reply
	Tier         Tier
	TriggerCount int
}

type Selector struct {
	mu             sync.Mutex
	catalog        Catalog
	users          map[string]*userState
	now            func() time.Time
	index          func(int) int
	window         time.Duration
	accessSequence uint64
}

type userState struct {
	activity       [activityLimit]time.Time
	activityStart  int
	activityCount  int
	recent         [recentCapacity]string
	recentStart    int
	recentCount    int
	accessSequence uint64
}

type SelectorOption func(*Selector)

func WithClock(now func() time.Time) SelectorOption {
	return func(selector *Selector) {
		if now != nil {
			selector.now = now
		}
	}
}

func WithIndex(index func(int) int) SelectorOption {
	return func(selector *Selector) {
		if index != nil {
			selector.index = index
		}
	}
}

func WithWindow(window time.Duration) SelectorOption {
	return func(selector *Selector) {
		if window > 0 {
			selector.window = window
		}
	}
}

func NewSelector(catalog Catalog, options ...SelectorOption) *Selector {
	selector := &Selector{
		catalog: catalog,
		users:   make(map[string]*userState),
		now:     time.Now,
		index:   rand.IntN,
		window:  defaultWindow,
	}
	for _, option := range options {
		option(selector)
	}
	return selector
}

func (s *Selector) Select(userID string) (Selection, error) {
	if userID == "" {
		return Selection{}, ErrEmptyUserID
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	state := s.users[userID]
	if state == nil {
		if len(s.users) >= maximumUsers {
			s.evictLeastRecentlySeen()
		}
		state = &userState{}
		s.users[userID] = state
	}
	s.accessSequence++
	state.accessSequence = s.accessSequence

	triggerCount := state.record(now, s.window)
	tier := tierForCount(triggerCount)
	reply, err := s.pick(s.catalog.byTier[tier], state)
	if err != nil {
		return Selection{}, err
	}
	state.remember(reply.ID)

	return Selection{Reply: reply, Tier: tier, TriggerCount: triggerCount}, nil
}

func (s *Selector) evictLeastRecentlySeen() {
	oldestUserID := ""
	var oldestSequence uint64
	for userID, state := range s.users {
		if oldestUserID == "" || state.accessSequence < oldestSequence {
			oldestUserID = userID
			oldestSequence = state.accessSequence
		}
	}
	if oldestUserID != "" {
		delete(s.users, oldestUserID)
	}
}

func (s *Selector) pick(candidates []Reply, state *userState) (Reply, error) {
	eligible := 0
	for _, candidate := range candidates {
		if !state.wasRecent(candidate.ID) {
			eligible++
		}
	}
	if eligible == 0 {
		return Reply{}, errors.New("no reply is outside the recent reply window")
	}

	target := s.index(eligible)
	if target < 0 || target >= eligible {
		return Reply{}, fmt.Errorf("reply index %d is outside [0,%d)", target, eligible)
	}
	for _, candidate := range candidates {
		if state.wasRecent(candidate.ID) {
			continue
		}
		if target == 0 {
			return candidate, nil
		}
		target--
	}
	return Reply{}, errors.New("reply selection exhausted candidates")
}

func (s *userState) record(now time.Time, window time.Duration) int {
	cutoff := now.Add(-window)
	for s.activityCount > 0 {
		oldest := s.activity[s.activityStart]
		if oldest.After(cutoff) {
			break
		}
		s.activityStart = (s.activityStart + 1) % activityLimit
		s.activityCount--
	}

	if s.activityCount < activityLimit {
		index := (s.activityStart + s.activityCount) % activityLimit
		s.activity[index] = now
		s.activityCount++
		return s.activityCount
	}

	s.activity[s.activityStart] = now
	s.activityStart = (s.activityStart + 1) % activityLimit
	return activityLimit
}

func (s *userState) wasRecent(id string) bool {
	for offset := 0; offset < s.recentCount; offset++ {
		index := (s.recentStart + offset) % recentCapacity
		if s.recent[index] == id {
			return true
		}
	}
	return false
}

func (s *userState) remember(id string) {
	if s.recentCount < recentCapacity {
		index := (s.recentStart + s.recentCount) % recentCapacity
		s.recent[index] = id
		s.recentCount++
		return
	}
	s.recent[s.recentStart] = id
	s.recentStart = (s.recentStart + 1) % recentCapacity
}

func tierForCount(count int) Tier {
	switch {
	case count <= 2:
		return TierSharp
	case count <= 5:
		return TierHarsh
	default:
		return TierSavage
	}
}

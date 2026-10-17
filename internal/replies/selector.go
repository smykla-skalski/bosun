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

	CandidateCount      = 8
	maximumReservations = 64
)

var (
	ErrEmptyUserID         = errors.New("user ID is empty")
	ErrInvalidBatchToken   = errors.New("batch token is invalid")
	ErrInvalidBatchChoice  = errors.New("batch choice is invalid")
	ErrReservationCapacity = errors.New("reply reservation capacity is full")
)

type Selection struct {
	Reply        Reply
	Tier         Tier
	TriggerCount int
}

type Batch struct {
	Selections [CandidateCount]Selection
	token      uint64
}

type Selector struct {
	mu               sync.Mutex
	catalog          Catalog
	users            map[string]*userState
	now              func() time.Time
	index            func(int) int
	window           time.Duration
	accessSequence   uint64
	nextToken        uint64
	reservations     [maximumReservations]reservation
	reservationCount int
	reservedIDs      map[string]map[string]struct{}
	reservedUsers    map[string]int
}

type reservation struct {
	token      uint64
	userID     string
	selections [CandidateCount]Selection
}

type userState struct {
	activity       [activityLimit]time.Time
	activityStart  int
	activityCount  int
	recent         [recentCapacity]string
	recentStart    int
	recentCount    int
	lastReply      replyPattern
	accessSequence uint64
}

type replyPattern struct {
	category  Category
	tier      Tier
	opening   uint8
	punchline uint8
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
		catalog:       catalog,
		users:         make(map[string]*userState),
		now:           time.Now,
		index:         rand.IntN,
		window:        defaultWindow,
		reservedIDs:   make(map[string]map[string]struct{}, maximumReservations),
		reservedUsers: make(map[string]int, maximumReservations),
	}
	for _, option := range options {
		option(selector)
	}
	return selector
}

func (s *Selector) Select(userID string, category Category) (Selection, error) {
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
	reply, err := s.pick(s.catalog.candidates(category, tier), state, userID)
	if err != nil {
		return Selection{}, err
	}
	state.remember(reply)

	return Selection{Reply: reply, Tier: tier, TriggerCount: triggerCount}, nil
}

func (s *Selector) SelectBatch(userID string, category Category) (Batch, error) {
	if userID == "" {
		return Batch{}, ErrEmptyUserID
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reservationCount >= maximumReservations {
		return Batch{}, ErrReservationCapacity
	}

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
	candidates := s.catalog.candidates(category, tier)
	batch := Batch{}
	for index := range batch.Selections {
		reply, err := s.pickBatch(candidates, state, userID, batch.Selections[:index])
		if err != nil {
			return Batch{}, err
		}
		batch.Selections[index] = Selection{Reply: reply, Tier: tier, TriggerCount: triggerCount}
	}

	batch.token = s.reserveToken()
	reservationIndex := s.freeReservationIndex()
	s.reservations[reservationIndex] = reservation{token: batch.token, userID: userID, selections: batch.Selections}
	s.reservationCount++
	reservedIDs := s.reservedIDs[userID]
	if reservedIDs == nil {
		reservedIDs = make(map[string]struct{}, CandidateCount)
		s.reservedIDs[userID] = reservedIDs
	}
	for _, selection := range batch.Selections {
		reservedIDs[selection.Reply.ID] = struct{}{}
	}
	s.reservedUsers[userID]++
	return batch, nil
}

func (s *Selector) CommitBatch(batch Batch, choice int) (Selection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	reservationIndex := s.reservationIndex(batch.token)
	if reservationIndex < 0 || batch.token == 0 {
		return Selection{}, ErrInvalidBatchToken
	}
	if choice < 0 || choice >= CandidateCount {
		return Selection{}, ErrInvalidBatchChoice
	}

	reserved := s.reservations[reservationIndex]
	s.releaseReservation(reservationIndex, reserved)
	state := s.users[reserved.userID]
	state.remember(reserved.selections[choice].Reply)
	return reserved.selections[choice], nil
}

func (s *Selector) CancelBatch(batch Batch) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	reservationIndex := s.reservationIndex(batch.token)
	if reservationIndex < 0 || batch.token == 0 {
		return ErrInvalidBatchToken
	}
	s.releaseReservation(reservationIndex, s.reservations[reservationIndex])
	return nil
}

func (s *Selector) reserveToken() uint64 {
	for {
		s.nextToken++
		if s.nextToken == 0 {
			continue
		}
		if s.reservationIndex(s.nextToken) < 0 {
			return s.nextToken
		}
	}
}

func (s *Selector) reservationIndex(token uint64) int {
	for index := range s.reservations {
		if s.reservations[index].token == token {
			return index
		}
	}
	return -1
}

func (s *Selector) freeReservationIndex() int {
	return s.reservationIndex(0)
}

func (s *Selector) releaseReservation(index int, reserved reservation) {
	s.reservations[index] = reservation{}
	s.reservationCount--
	reservedIDs := s.reservedIDs[reserved.userID]
	for _, selection := range reserved.selections {
		delete(reservedIDs, selection.Reply.ID)
	}
	if s.reservedUsers[reserved.userID] <= 1 {
		delete(s.reservedUsers, reserved.userID)
		return
	}
	s.reservedUsers[reserved.userID]--
}

func (s *Selector) pickBatch(candidates []Reply, state *userState, userID string, selected []Selection) (Reply, error) {
	eligible := 0
	for _, candidate := range candidates {
		if state.wasRecent(candidate.ID) || state.matchesLast(candidate) || s.isReserved(userID, candidate.ID) || batchContains(selected, candidate) {
			continue
		}
		eligible++
	}
	if eligible == 0 {
		return Reply{}, errors.New("no safe reply is outside the recent reply window")
	}

	target := s.index(eligible)
	if target < 0 || target >= eligible {
		return Reply{}, fmt.Errorf("reply index %d is outside [0,%d)", target, eligible)
	}
	for _, candidate := range candidates {
		if state.wasRecent(candidate.ID) || state.matchesLast(candidate) || s.isReserved(userID, candidate.ID) || batchContains(selected, candidate) {
			continue
		}
		if target == 0 {
			return candidate, nil
		}
		target--
	}
	return Reply{}, errors.New("reply batch selection exhausted candidates")
}

func batchContains(selected []Selection, candidate Reply) bool {
	for _, selection := range selected {
		if selection.Reply.ID == candidate.ID || patternsOverlap(selection.Reply, candidate) {
			return true
		}
	}
	return false
}

func (s *Selector) evictLeastRecentlySeen() {
	oldestUserID := ""
	var oldestSequence uint64
	for userID, state := range s.users {
		if s.reservedUsers[userID] > 0 {
			continue
		}
		if oldestUserID == "" || state.accessSequence < oldestSequence {
			oldestUserID = userID
			oldestSequence = state.accessSequence
		}
	}
	if oldestUserID != "" {
		delete(s.users, oldestUserID)
		delete(s.reservedIDs, oldestUserID)
	}
}

func (s *Selector) pick(candidates []Reply, state *userState, userID string) (Reply, error) {
	eligible := 0
	for _, candidate := range candidates {
		if !state.wasRecent(candidate.ID) && !state.matchesLast(candidate) && !s.isReserved(userID, candidate.ID) {
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
		if state.wasRecent(candidate.ID) || state.matchesLast(candidate) || s.isReserved(userID, candidate.ID) {
			continue
		}
		if target == 0 {
			return candidate, nil
		}
		target--
	}
	return Reply{}, errors.New("reply selection exhausted candidates")
}

func (s *Selector) isReserved(userID, id string) bool {
	_, exists := s.reservedIDs[userID][id]
	return exists
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

func (s *userState) remember(reply Reply) {
	id := reply.ID
	if s.recentCount < recentCapacity {
		index := (s.recentStart + s.recentCount) % recentCapacity
		s.recent[index] = id
		s.recentCount++
		s.lastReply = patternFor(reply)
		return
	}
	s.recent[s.recentStart] = id
	s.recentStart = (s.recentStart + 1) % recentCapacity
	s.lastReply = patternFor(reply)
}

func (s *userState) matchesLast(reply Reply) bool {
	last := s.lastReply
	return last.opening != 0 && last.punchline != 0 && last.category == reply.Category && last.tier == reply.Tier &&
		(last.opening == reply.OpeningVariant || last.punchline == reply.PunchlineVariant)
}

func patternsOverlap(left Reply, right Reply) bool {
	return left.OpeningVariant != 0 && left.PunchlineVariant != 0 &&
		left.Category == right.Category && left.Tier == right.Tier &&
		(left.OpeningVariant == right.OpeningVariant || left.PunchlineVariant == right.PunchlineVariant)
}

func patternFor(reply Reply) replyPattern {
	return replyPattern{
		category:  reply.Category,
		tier:      reply.Tier,
		opening:   reply.OpeningVariant,
		punchline: reply.PunchlineVariant,
	}
}

func tierForCount(count int) Tier {
	if count >= 3 {
		return TierSavage
	}
	return TierHarsh
}

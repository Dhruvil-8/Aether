package node

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type PeerBook struct {
	path              string
	statePath         string
	mu                sync.Mutex
	states            map[string]peerState
	scoreDecay        time.Duration
	evictionThreshold int
	evictionCooldown  time.Duration
}

type peerState struct {
	Score         int       `json:"score"`
	BackoffUntil  time.Time `json:"backoff_until,omitempty"`
	BanUntil      time.Time `json:"ban_until,omitempty"`
	EvictedUntil  time.Time `json:"evicted_until,omitempty"`
	EvictionCount int       `json:"eviction_count,omitempty"`
	UpdatedAt     time.Time `json:"updated_at,omitempty"`
}

func OpenPeerBook(root string) (*PeerBook, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	statePath := filepath.Join(root, "peer_state.json")
	states, err := loadPeerStates(statePath)
	if err != nil {
		return nil, err
	}
	return &PeerBook{
		path:              filepath.Join(root, "peers.json"),
		statePath:         statePath,
		states:            states,
		scoreDecay:        time.Hour,
		evictionThreshold: 16,
		evictionCooldown:  30 * time.Minute,
	}, nil
}

func (b *PeerBook) ApplyPolicy(cfg Config) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.scoreDecay = peerScoreDecay(cfg)
	b.evictionThreshold = peerEvictionScore(cfg)
	b.evictionCooldown = peerEvictionCooldown(cfg)
}

func (b *PeerBook) Load() ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.loadUnlocked()
}

func (b *PeerBook) Merge(peers ...string) ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	current, err := b.loadUnlocked()
	if err != nil {
		return nil, err
	}

	merged := make(map[string]struct{}, len(current)+len(peers))
	for _, peer := range current {
		merged[peer] = struct{}{}
	}
	for _, peer := range peers {
		if cleaned := cleanPeerAddress(peer); cleaned != "" {
			if !b.isCandidateAllowedUnlocked(cleaned, time.Now()) {
				continue
			}
			merged[cleaned] = struct{}{}
		}
	}

	out := make([]string, 0, len(merged))
	for peer := range merged {
		out = append(out, peer)
	}
	sort.Strings(out)
	if err := b.writeUnlocked(out); err != nil {
		return nil, err
	}
	return out, nil
}

func (b *PeerBook) loadUnlocked() ([]string, error) {
	data, err := os.ReadFile(b.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var peers []string
	if err := json.Unmarshal(data, &peers); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(peers))
	for _, peer := range peers {
		if cleaned := cleanPeerAddress(peer); cleaned != "" {
			out = append(out, cleaned)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (b *PeerBook) writeUnlocked(peers []string) error {
	data, err := json.MarshalIndent(peers, "", "  ")
	if err != nil {
		return err
	}
	tmp := b.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, b.path)
}

func candidatePeers(cfg Config, book *PeerBook) []string {
	merged := make(map[string]struct{})
	now := time.Now()
	for _, peer := range cfg.BootstrapPeers {
		if cleaned := cleanPeerAddress(peer); cleaned != "" {
			if book != nil && !book.IsCandidateAllowed(cleaned, now) {
				continue
			}
			merged[cleaned] = struct{}{}
		}
	}
	if book != nil {
		stored, err := book.Load()
		if err == nil {
			for _, peer := range stored {
				if cleaned := cleanPeerAddress(peer); cleaned != "" && book.IsCandidateAllowed(cleaned, now) {
					merged[cleaned] = struct{}{}
				}
			}
		}
	}
	out := make([]string, 0, len(merged))
	for peer := range merged {
		out = append(out, peer)
	}
	sort.Strings(out)
	return out
}

func cleanPeerAddress(peer string) string {
	return strings.TrimSpace(peer)
}

func (b *PeerBook) IsBackedOff(peer string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	peer = cleanPeerAddress(peer)
	state, changed, ok := b.normalizeStateUnlocked(peer, now)
	if changed {
		b.persistStateUnlocked()
	}
	if !ok {
		return false
	}
	if !state.EvictedUntil.IsZero() && now.Before(state.EvictedUntil) {
		return true
	}
	return (!state.BanUntil.IsZero() && now.Before(state.BanUntil)) ||
		(!state.BackoffUntil.IsZero() && now.Before(state.BackoffUntil))
}

func (b *PeerBook) MarkPenalty(peer string, weight int, baseBackoff time.Duration, banThreshold int, banDuration time.Duration, now time.Time) {
	peer = cleanPeerAddress(peer)
	if peer == "" {
		return
	}
	if weight <= 0 {
		weight = 1
	}
	if baseBackoff <= 0 {
		baseBackoff = 30 * time.Second
	}
	if banThreshold <= 0 {
		banThreshold = 8
	}
	if banDuration <= 0 {
		banDuration = 15 * time.Minute
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	state, _, _ := b.normalizeStateUnlocked(peer, now)
	state.Score += weight
	state.UpdatedAt = now
	score := state.Score
	multiplier := 1 << min(score-1, 4)
	until := now.Add(time.Duration(multiplier) * baseBackoff)
	state.BackoffUntil = until
	if score >= banThreshold {
		banUntil := now.Add(banDuration)
		state.BanUntil = banUntil
		if banUntil.After(until) {
			state.BackoffUntil = banUntil
		}
	}
	b.states[peer] = state
	if state.Score >= b.evictionThreshold {
		b.evictPeerUnlocked(peer, now)
	}
	b.persistStateUnlocked()
}

func (b *PeerBook) MarkSuccess(peer string) {
	b.MarkSuccessAt(peer, time.Now())
}

func (b *PeerBook) MarkSuccessAt(peer string, now time.Time) {
	peer = cleanPeerAddress(peer)
	if peer == "" {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	state, _, ok := b.normalizeStateUnlocked(peer, now)
	if !ok {
		return
	}
	state.BackoffUntil = time.Time{}
	state.BanUntil = time.Time{}
	state.EvictedUntil = time.Time{}
	if state.Score <= 1 {
		delete(b.states, peer)
		b.persistStateUnlocked()
		return
	}
	state.Score--
	state.UpdatedAt = now
	b.states[peer] = state
	b.persistStateUnlocked()
}

func (b *PeerBook) IsBanned(peer string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	peer = cleanPeerAddress(peer)
	state, changed, ok := b.normalizeStateUnlocked(peer, now)
	if changed {
		b.persistStateUnlocked()
	}
	if !ok || state.BanUntil.IsZero() {
		return false
	}
	return now.Before(state.BanUntil)
}

func (b *PeerBook) PenaltyScoreAt(peer string, now time.Time) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	state, changed, ok := b.normalizeStateUnlocked(peer, now)
	if changed {
		b.persistStateUnlocked()
	}
	if !ok {
		return 0
	}
	return state.Score
}

func (b *PeerBook) Sweep(now time.Time) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	changed := false
	for peer := range b.states {
		_, stateChanged, ok := b.normalizeStateUnlocked(peer, now)
		if stateChanged {
			changed = true
		}
		if !ok {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return writePeerStates(b.statePath, b.states)
}

func (b *PeerBook) normalizeStateUnlocked(peer string, now time.Time) (peerState, bool, bool) {
	peer = cleanPeerAddress(peer)
	state, ok := b.states[peer]
	if !ok {
		return peerState{}, false, false
	}

	changed := false
	if b.scoreDecay > 0 && !state.UpdatedAt.IsZero() && now.After(state.UpdatedAt) && state.Score > 0 {
		steps := int(now.Sub(state.UpdatedAt) / b.scoreDecay)
		if steps > 0 {
			state.Score -= steps
			if state.Score < 0 {
				state.Score = 0
			}
			state.UpdatedAt = state.UpdatedAt.Add(time.Duration(steps) * b.scoreDecay)
			changed = true
		}
	}
	if !state.BanUntil.IsZero() && !now.Before(state.BanUntil) {
		state.BanUntil = time.Time{}
		changed = true
	}
	if !state.EvictedUntil.IsZero() && !now.Before(state.EvictedUntil) {
		state.EvictedUntil = time.Time{}
		changed = true
	}
	if !state.BackoffUntil.IsZero() && !now.Before(state.BackoffUntil) {
		state.BackoffUntil = time.Time{}
		changed = true
	}
	if state.Score <= 0 && state.BackoffUntil.IsZero() && state.BanUntil.IsZero() && state.EvictedUntil.IsZero() {
		delete(b.states, peer)
		return peerState{}, true, false
	}
	if changed {
		b.states[peer] = state
	}
	return state, changed, true
}

func (b *PeerBook) evictPeerUnlocked(peer string, now time.Time) {
	state := b.states[peer]
	state.EvictionCount++
	cooldown := b.evictionCooldown
	if cooldown <= 0 {
		cooldown = 30 * time.Minute
	}
	multiplier := 1 << min(state.EvictionCount-1, 4)
	state.EvictedUntil = now.Add(time.Duration(multiplier) * cooldown)
	if state.EvictedUntil.After(state.BackoffUntil) {
		state.BackoffUntil = state.EvictedUntil
	}
	b.states[peer] = state

	peers, err := b.loadUnlocked()
	if err != nil || len(peers) == 0 {
		return
	}
	filtered := make([]string, 0, len(peers))
	for _, candidate := range peers {
		if candidate != peer {
			filtered = append(filtered, candidate)
		}
	}
	if len(filtered) == len(peers) {
		return
	}
	_ = b.writeUnlocked(filtered)
}

func (b *PeerBook) IsCandidateAllowed(peer string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.isCandidateAllowedUnlocked(peer, now)
}

func (b *PeerBook) isCandidateAllowedUnlocked(peer string, now time.Time) bool {
	peer = cleanPeerAddress(peer)
	state, changed, ok := b.normalizeStateUnlocked(peer, now)
	if changed {
		b.persistStateUnlocked()
	}
	if !ok {
		return true
	}
	return state.EvictedUntil.IsZero() || !now.Before(state.EvictedUntil)
}

func (b *PeerBook) persistStateUnlocked() {
	_ = writePeerStates(b.statePath, b.states)
}

func loadPeerStates(path string) (map[string]peerState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]peerState), nil
		}
		return nil, err
	}
	var states map[string]peerState
	if err := json.Unmarshal(data, &states); err != nil {
		return nil, err
	}
	if states == nil {
		states = make(map[string]peerState)
	}
	out := make(map[string]peerState, len(states))
	for peer, state := range states {
		if cleaned := cleanPeerAddress(peer); cleaned != "" {
			out[cleaned] = state
		}
	}
	return out, nil
}

func writePeerStates(path string, states map[string]peerState) error {
	if states == nil {
		states = make(map[string]peerState)
	}
	data, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func peerScoreDecay(cfg Config) time.Duration {
	if cfg.PeerScoreDecay <= 0 {
		return time.Hour
	}
	return cfg.PeerScoreDecay
}

func peerEvictionScore(cfg Config) int {
	if cfg.PeerEvictionScore <= 0 {
		return 16
	}
	return cfg.PeerEvictionScore
}

func peerEvictionCooldown(cfg Config) time.Duration {
	if cfg.PeerEvictionCooldown <= 0 {
		return 30 * time.Minute
	}
	return cfg.PeerEvictionCooldown
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

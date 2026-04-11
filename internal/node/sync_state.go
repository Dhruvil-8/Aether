package node

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"aether/internal/protocol"
)

type SyncCursor struct {
	Offset        int                              `json:"offset"`
	LastHash      string                           `json:"last_hash,omitempty"`
	Checkpoints   []protocol.SyncCheckpoint        `json:"checkpoints,omitempty"`
	Accumulators  []protocol.SyncAccumulatorDigest `json:"accumulators,omitempty"`
	WindowDigests []protocol.SyncWindowDigest      `json:"window_digests,omitempty"`
}

type SyncState struct {
	path string
	mu   sync.Mutex
}

func OpenSyncState(root string) (*SyncState, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &SyncState{path: filepath.Join(root, "sync_state.json")}, nil
}

func (s *SyncState) Get(peer string) (SyncCursor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadUnlocked()
	if err != nil {
		return SyncCursor{}, err
	}
	return state[cleanPeerAddress(peer)], nil
}

func (s *SyncState) Set(peer string, cursor SyncCursor) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadUnlocked()
	if err != nil {
		return err
	}
	peer = cleanPeerAddress(peer)
	if peer == "" {
		return nil
	}
	if cursor.Offset < 0 {
		cursor.Offset = 0
	}
	state[peer] = cursor
	return s.writeUnlocked(state)
}

func (s *SyncState) loadUnlocked() (map[string]SyncCursor, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]SyncCursor), nil
		}
		return nil, err
	}

	state := make(map[string]SyncCursor)
	if err := json.Unmarshal(data, &state); err == nil {
		if state == nil {
			state = make(map[string]SyncCursor)
		}
		return state, nil
	}

	legacy := make(map[string]int)
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, err
	}
	for peer, offset := range legacy {
		state[cleanPeerAddress(peer)] = SyncCursor{Offset: offset}
	}
	return state, nil
}

func (s *SyncState) writeUnlocked(state map[string]SyncCursor) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

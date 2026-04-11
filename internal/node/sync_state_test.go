package node

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncStateLoadsLegacyOffsetFormat(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sync_state.json")
	if err := os.WriteFile(path, []byte("{\"peer.onion:9000\":3}"), 0o644); err != nil {
		t.Fatal(err)
	}

	state, err := OpenSyncState(root)
	if err != nil {
		t.Fatal(err)
	}

	cursor, err := state.Get("peer.onion:9000")
	if err != nil {
		t.Fatal(err)
	}
	if cursor.Offset != 3 {
		t.Fatalf("unexpected migrated offset: got %d want 3", cursor.Offset)
	}
	if cursor.LastHash != "" {
		t.Fatalf("unexpected migrated last hash: %q", cursor.LastHash)
	}
}

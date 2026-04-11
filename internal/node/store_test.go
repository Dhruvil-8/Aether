package node

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"aether/internal/protocol"
)

func TestStoreAppendAndLoadAll(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	first := &protocol.Message{V: 1, T: 1710000000, C: "hello"}
	first.H = first.ComputeHash()
	second := &protocol.Message{V: 1, T: 1710000001, C: "world"}
	second.H = second.ComputeHash()

	if err := store.Append(first); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(second); err != nil {
		t.Fatal(err)
	}

	got, err := store.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("unexpected message count: %d", len(got))
	}
	if got[0].H != first.H || got[1].H != second.H {
		t.Fatalf("loaded messages do not match appended messages")
	}
}

func TestStoreSyncMetadataIncludesWindowDigestsAndTip(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	var hashes []string
	for _, content := range []string{"a", "b", "c", "d"} {
		msg := &protocol.Message{V: 1, T: 1710000000, C: content}
		msg.H = msg.ComputeHash()
		hashes = append(hashes, msg.H)
		if err := store.Append(msg); err != nil {
			t.Fatal(err)
		}
	}

	meta, err := store.SyncMetadata(protocol.SyncMetaRequestPayload{
		Offsets:            []int{2, 4},
		AccumulatorOffsets: []int{2, 4},
		WindowEnds:         []int{4},
		WindowSize:         2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if meta.TotalMessages != 4 {
		t.Fatalf("unexpected total messages: got %d want 4", meta.TotalMessages)
	}
	if meta.TipHash != hashes[3] {
		t.Fatalf("unexpected tip hash: got %s want %s", meta.TipHash, hashes[3])
	}
	if len(meta.WindowDigests) != 1 {
		t.Fatalf("unexpected window digest count: got %d want 1", len(meta.WindowDigests))
	}
	if len(meta.Accumulators) != 2 {
		t.Fatalf("unexpected accumulator digest count: got %d want 2", len(meta.Accumulators))
	}
	if meta.Accumulators[0].Offset != 2 || meta.Accumulators[1].Offset != 4 {
		t.Fatalf("unexpected accumulator offsets: %+v", meta.Accumulators)
	}
	if meta.Accumulators[0].Hash == "" || meta.Accumulators[1].Hash == "" {
		t.Fatalf("expected accumulator hashes to be populated: %+v", meta.Accumulators)
	}
	if meta.WindowDigests[0].EndOffset != 4 || meta.WindowDigests[0].WindowSize != 2 {
		t.Fatalf("unexpected window digest metadata: %+v", meta.WindowDigests[0])
	}

	h := sha256.New()
	for _, src := range hashes[2:] {
		raw, err := hex.DecodeString(src)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = h.Write(raw)
	}
	want := hex.EncodeToString(h.Sum(nil))
	if meta.WindowDigests[0].Hash != want {
		t.Fatalf("unexpected window digest hash: got %s want %s", meta.WindowDigests[0].Hash, want)
	}
}

func TestStoreSealsAndCompressesSegments(t *testing.T) {
	root := t.TempDir()
	store, err := openStoreWithMaxSegmentSize(root, 120)
	if err != nil {
		t.Fatal(err)
	}

	var expected []string
	for _, content := range []string{"first compressed message", "second compressed message", "third compressed message"} {
		msg := &protocol.Message{V: 1, T: 1710000000, C: content}
		msg.H = msg.ComputeHash()
		expected = append(expected, msg.H)
		if err := store.Append(msg); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := os.ReadDir(filepath.Join(root, "segments"))
	if err != nil {
		t.Fatal(err)
	}

	foundCompressed := false
	foundActive := false
	for _, entry := range entries {
		switch filepath.Ext(entry.Name()) {
		case ".gz":
			foundCompressed = true
		case ".log":
			foundActive = true
		}
	}
	if !foundCompressed {
		t.Fatal("expected at least one compressed sealed segment")
	}
	if !foundActive {
		t.Fatal("expected an active log segment")
	}

	got, err := store.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(expected) {
		t.Fatalf("unexpected message count: got %d want %d", len(got), len(expected))
	}
	for i, msg := range got {
		if msg.H != expected[i] {
			t.Fatalf("unexpected hash at %d: got %s want %s", i, msg.H, expected[i])
		}
	}
}

func TestStoreLoadRange(t *testing.T) {
	root := t.TempDir()
	store, err := openStoreWithMaxSegmentSize(root, 120)
	if err != nil {
		t.Fatal(err)
	}

	var expected []string
	for _, content := range []string{"one", "two", "three", "four"} {
		msg := &protocol.Message{V: 1, T: 1710000000, C: content}
		msg.H = msg.ComputeHash()
		expected = append(expected, msg.H)
		if err := store.Append(msg); err != nil {
			t.Fatal(err)
		}
	}

	firstBatch, nextOffset, hasMore, err := store.LoadRange(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstBatch) != 2 {
		t.Fatalf("unexpected first batch size: got %d want 2", len(firstBatch))
	}
	if firstBatch[0].H != expected[1] || firstBatch[1].H != expected[2] {
		t.Fatalf("unexpected first batch hashes")
	}
	if nextOffset != 3 {
		t.Fatalf("unexpected next offset: got %d want 3", nextOffset)
	}
	if !hasMore {
		t.Fatal("expected more messages after first batch")
	}

	secondBatch, nextOffset, hasMore, err := store.LoadRange(nextOffset, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondBatch) != 1 {
		t.Fatalf("unexpected second batch size: got %d want 1", len(secondBatch))
	}
	if secondBatch[0].H != expected[3] {
		t.Fatalf("unexpected second batch hash: got %s want %s", secondBatch[0].H, expected[3])
	}
	if nextOffset != 4 {
		t.Fatalf("unexpected final next offset: got %d want 4", nextOffset)
	}
	if hasMore {
		t.Fatal("did not expect more messages after second batch")
	}
}

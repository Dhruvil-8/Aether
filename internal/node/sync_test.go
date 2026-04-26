package node

import (
	"strings"
	"testing"

	"aether/internal/protocol"
)

func TestVerifyChunkMerkleProofRejectsTampering(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"a", "b", "c", "d"} {
		msg := &protocol.Message{V: 1, T: 1710000000, C: content}
		msg.H = msg.ComputeHash()
		if err := store.Append(msg); err != nil {
			t.Fatal(err)
		}
	}

	meta, err := store.SyncMetadata(protocol.SyncMetaRequestPayload{
		ChunkIndices: []int{1},
		ChunkSize:    2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.ChunkMerkleProofs) != 1 {
		t.Fatalf("expected one proof, got %d", len(meta.ChunkMerkleProofs))
	}
	proof := meta.ChunkMerkleProofs[0]
	if !verifyChunkMerkleProof(proof, meta.ChunkMerkleRoot, meta.ChunkMerkleLeaves) {
		t.Fatal("expected untampered proof to verify")
	}

	tampered := proof
	if len(tampered.Siblings) == 0 {
		t.Fatal("expected proof siblings for tamper test")
	}
	tampered.Siblings[0] = strings.Repeat("0", len(tampered.Siblings[0]))
	if verifyChunkMerkleProof(tampered, meta.ChunkMerkleRoot, meta.ChunkMerkleLeaves) {
		t.Fatal("expected tampered proof to fail verification")
	}
}

func TestConfirmSyncMetaRequiresQuorumAgreement(t *testing.T) {
	primary := &protocol.SyncMetaPayload{
		TotalMessages: 4,
		TipHash:       "tip-a",
		Checkpoints: []protocol.SyncCheckpoint{
			{Offset: 4, Hash: "hash-a"},
		},
	}
	disagreeing := &protocol.SyncMetaPayload{
		TotalMessages: 4,
		TipHash:       "tip-b",
		Checkpoints: []protocol.SyncCheckpoint{
			{Offset: 4, Hash: "hash-b"},
		},
	}

	_, err := confirmSyncMeta(
		Config{SyncTrustQuorum: 2},
		"peer-a",
		[]string{"peer-a", "peer-b"},
		protocol.SyncMetaRequestPayload{},
		primary,
		func(Config, string, protocol.SyncMetaRequestPayload) (*protocol.SyncMetaPayload, error) {
			return disagreeing, nil
		},
	)
	if err == nil {
		t.Fatal("expected quorum failure when peers disagree")
	}
}

func TestConfirmSyncMetaAcceptsQuorumAgreement(t *testing.T) {
	primary := &protocol.SyncMetaPayload{
		TotalMessages: 4,
		TipHash:       "tip-a",
		Checkpoints: []protocol.SyncCheckpoint{
			{Offset: 4, Hash: "hash-a"},
		},
	}

	meta, err := confirmSyncMeta(
		Config{SyncTrustQuorum: 2},
		"peer-a",
		[]string{"peer-a", "peer-b", "peer-c"},
		protocol.SyncMetaRequestPayload{},
		primary,
		func(Config, string, protocol.SyncMetaRequestPayload) (*protocol.SyncMetaPayload, error) {
			return &protocol.SyncMetaPayload{
				TotalMessages: primary.TotalMessages,
				TipHash:       primary.TipHash,
				Checkpoints: []protocol.SyncCheckpoint{
					primary.Checkpoints[0],
				},
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("expected quorum success: %v", err)
	}
	if meta != primary {
		t.Fatal("expected primary metadata to be returned after confirmation")
	}
}

func TestSyncMetaTrustFingerprintIgnoresMetadataOrder(t *testing.T) {
	a := &protocol.SyncMetaPayload{
		TotalMessages: 8,
		TipHash:       "tip",
		Checkpoints: []protocol.SyncCheckpoint{
			{Offset: 8, Hash: "h8"},
			{Offset: 4, Hash: "h4"},
		},
		Accumulators: []protocol.SyncAccumulatorDigest{
			{Offset: 8, Hash: "a8"},
			{Offset: 4, Hash: "a4"},
		},
		ChunkDigests: []protocol.SyncChunkDigest{
			{Index: 1, StartOffset: 5, EndOffset: 8, Hash: "c1"},
			{Index: 0, StartOffset: 1, EndOffset: 4, Hash: "c0"},
		},
		WindowDigests: []protocol.SyncWindowDigest{
			{EndOffset: 8, WindowSize: 4, Hash: "w8"},
			{EndOffset: 4, WindowSize: 4, Hash: "w4"},
		},
	}
	b := &protocol.SyncMetaPayload{
		TotalMessages: a.TotalMessages,
		TipHash:       a.TipHash,
		Checkpoints: []protocol.SyncCheckpoint{
			a.Checkpoints[1],
			a.Checkpoints[0],
		},
		Accumulators: []protocol.SyncAccumulatorDigest{
			a.Accumulators[1],
			a.Accumulators[0],
		},
		ChunkDigests: []protocol.SyncChunkDigest{
			a.ChunkDigests[1],
			a.ChunkDigests[0],
		},
		WindowDigests: []protocol.SyncWindowDigest{
			a.WindowDigests[1],
			a.WindowDigests[0],
		},
	}

	if syncMetaTrustFingerprint(a) != syncMetaTrustFingerprint(b) {
		t.Fatal("expected metadata fingerprint to be stable across ordering differences")
	}
}

func TestNormalizeCursorRequiresValidChunkMerkleProofWhenProvided(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"a", "b", "c", "d", "e", "f"} {
		msg := &protocol.Message{V: 1, T: 1710000000, C: content}
		msg.H = msg.ComputeHash()
		if err := store.Append(msg); err != nil {
			t.Fatal(err)
		}
	}

	meta, err := store.SyncMetadata(protocol.SyncMetaRequestPayload{
		ChunkIndices: []int{1},
		ChunkSize:    2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.ChunkDigests) != 1 || len(meta.ChunkMerkleProofs) != 1 {
		t.Fatalf("unexpected chunk metadata: digests=%d proofs=%d", len(meta.ChunkDigests), len(meta.ChunkMerkleProofs))
	}

	cursor := SyncCursor{
		Offset:   6,
		LastHash: "bad-hash",
		ChunkDigests: []protocol.SyncChunkDigest{
			meta.ChunkDigests[0],
		},
	}

	tampered := *meta
	tampered.ChunkMerkleProofs = append([]protocol.SyncChunkProof(nil), meta.ChunkMerkleProofs...)
	tampered.ChunkMerkleProofs[0].Siblings = append([]string(nil), tampered.ChunkMerkleProofs[0].Siblings...)
	if len(tampered.ChunkMerkleProofs[0].Siblings) == 0 {
		t.Fatal("expected chunk proof siblings")
	}
	tampered.ChunkMerkleProofs[0].Siblings[0] = strings.Repeat("f", len(tampered.ChunkMerkleProofs[0].Siblings[0]))

	reset := normalizeCursor(cursor, &tampered)
	if reset.Offset != 0 {
		t.Fatalf("expected cursor reset when proof invalid, got offset %d", reset.Offset)
	}

	recovered := normalizeCursor(cursor, meta)
	if recovered.Offset != meta.ChunkDigests[0].EndOffset {
		t.Fatalf("expected cursor fallback to verified chunk offset %d, got %d", meta.ChunkDigests[0].EndOffset, recovered.Offset)
	}
}

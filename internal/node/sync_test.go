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

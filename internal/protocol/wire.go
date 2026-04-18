package protocol

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
)

const (
	FrameHello    = 0x01
	FrameGetPeers = 0x02
	FramePeers    = 0x03
	FrameMessage  = 0x04
	FrameSyncReq  = 0x05
	FrameSyncData = 0x06
	FramePing     = 0x07
	FramePong     = 0x08
	FrameAck      = 0x09
	FrameSyncMeta = 0x0A
)

var (
	ErrFrameTooShort = errors.New("frame too short")
	ErrFrameTooLarge = errors.New("frame too large")
)

const (
	ProtocolName = "aether-protocol-v1"
	NodeVersion  = "aetherd/0.1.0"
	MaxFrameSize = 8 * 1024 * 1024
)

type Frame struct {
	Type    byte
	Payload []byte
}

type HelloPayload struct {
	NetworkID    string   `json:"network_id"`
	NodeVersion  string   `json:"node_version"`
	Capabilities []string `json:"capabilities"`
	ListenAddr   string   `json:"listen_addr,omitempty"`
}

type PeersPayload struct {
	Peers []string `json:"peers"`
}

type MessagePayload struct {
	Message Message `json:"message"`
}

type SyncRequestPayload struct {
	Offset      int `json:"offset,omitempty"`
	MaxMessages int `json:"max_messages,omitempty"`
}

type SyncMetaRequestPayload struct {
	Offsets            []int `json:"offsets,omitempty"`
	AccumulatorOffsets []int `json:"accumulator_offsets,omitempty"`
	ChunkIndices       []int `json:"chunk_indices,omitempty"`
	ChunkSize          int   `json:"chunk_size,omitempty"`
	WindowEnds         []int `json:"window_ends,omitempty"`
	WindowSize         int   `json:"window_size,omitempty"`
}

type SyncCheckpoint struct {
	Offset int    `json:"offset"`
	Hash   string `json:"hash,omitempty"`
}

type SyncWindowDigest struct {
	EndOffset  int    `json:"end_offset"`
	WindowSize int    `json:"window_size"`
	Hash       string `json:"hash,omitempty"`
}

type SyncAccumulatorDigest struct {
	Offset int    `json:"offset"`
	Hash   string `json:"hash,omitempty"`
}

type SyncChunkDigest struct {
	Index       int    `json:"index"`
	StartOffset int    `json:"start_offset"`
	EndOffset   int    `json:"end_offset"`
	Hash        string `json:"hash,omitempty"`
}

type SyncChunkProof struct {
	Index    int      `json:"index"`
	LeafHash string   `json:"leaf_hash,omitempty"`
	Siblings []string `json:"siblings,omitempty"`
}

type SyncMetaPayload struct {
	TotalMessages     int                     `json:"total_messages"`
	TipHash           string                  `json:"tip_hash,omitempty"`
	Checkpoints       []SyncCheckpoint        `json:"checkpoints,omitempty"`
	Accumulators      []SyncAccumulatorDigest `json:"accumulators,omitempty"`
	ChunkDigests      []SyncChunkDigest       `json:"chunk_digests,omitempty"`
	ChunkMerkleRoot   string                  `json:"chunk_merkle_root,omitempty"`
	ChunkMerkleLeaves int                     `json:"chunk_merkle_leaves,omitempty"`
	ChunkMerkleProofs []SyncChunkProof        `json:"chunk_merkle_proofs,omitempty"`
	WindowDigests     []SyncWindowDigest      `json:"window_digests,omitempty"`
}

type SyncDataPayload struct {
	Messages   []Message `json:"messages"`
	NextOffset int       `json:"next_offset"`
	HasMore    bool      `json:"has_more"`
}

type AckPayload struct {
	Hash string `json:"hash"`
}

func WriteFrame(w io.Writer, frameType byte, payload []byte) error {
	totalLen := uint32(len(payload) + 1)
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, totalLen)
	if _, err := w.Write(header); err != nil {
		return err
	}
	if _, err := w.Write([]byte{frameType}); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func ReadFrame(r io.Reader) (*Frame, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}
	totalLen := binary.BigEndian.Uint32(header)
	if totalLen < 1 {
		return nil, ErrFrameTooShort
	}
	if totalLen > MaxFrameSize {
		return nil, ErrFrameTooLarge
	}
	buf := make([]byte, totalLen)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return &Frame{
		Type:    buf[0],
		Payload: buf[1:],
	}, nil
}

func EncodeJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

func DecodeJSON(data []byte, dest any) error {
	return json.Unmarshal(data, dest)
}

func NetworkID() string {
	sum := sha256.Sum256([]byte(ProtocolName))
	return hex.EncodeToString(sum[:])
}

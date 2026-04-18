package node

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"aether/internal/protocol"
)

const defaultMaxSegmentBytes int64 = 64 * 1024 * 1024

type Store struct {
	root            string
	segmentsDir     string
	maxSegmentBytes int64
	mu              sync.Mutex
}

func OpenStore(root string) (*Store, error) {
	return openStoreWithMaxSegmentSize(root, defaultMaxSegmentBytes)
}

func openStoreWithMaxSegmentSize(root string, maxSegmentBytes int64) (*Store, error) {
	segmentsDir := filepath.Join(root, "segments")
	if err := os.MkdirAll(segmentsDir, 0o755); err != nil {
		return nil, err
	}
	return &Store{
		root:            root,
		segmentsDir:     segmentsDir,
		maxSegmentBytes: maxSegmentBytes,
	}, nil
}

func (s *Store) Append(msg *protocol.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	filePath, err := s.ensureWritableSegmentLocked(int64(4 + len(record)))
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	header := make([]byte, 4)
	binary.LittleEndian.PutUint32(header, uint32(len(record)))
	if _, err := f.Write(header); err != nil {
		return err
	}
	_, err = f.Write(record)
	return err
}

func (s *Store) LoadAll() ([]protocol.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.loadAllLocked()
}

func (s *Store) LoadRange(offset, limit int) ([]protocol.Message, int, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 256
	}

	segments, err := s.segmentFilesLocked()
	if err != nil {
		return nil, 0, false, err
	}

	var out []protocol.Message
	index := 0
	for _, segment := range segments {
		messages, err := loadSegmentMessages(segment.path, segment.compressed)
		if err != nil {
			return nil, 0, false, err
		}
		for _, msg := range messages {
			if index < offset {
				index++
				continue
			}
			if len(out) < limit {
				out = append(out, msg)
				index++
				continue
			}
			return out, offset + len(out), true, nil
		}
	}
	return out, index, false, nil
}

func (s *Store) SyncMetadata(req protocol.SyncMetaRequestPayload) (*protocol.SyncMetaPayload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	messages, err := s.loadAllLocked()
	if err != nil {
		return nil, err
	}

	checkpoints := make([]protocol.SyncCheckpoint, 0, len(req.Offsets))
	seen := make(map[int]struct{}, len(req.Offsets))
	for _, offset := range req.Offsets {
		if offset <= 0 {
			continue
		}
		if _, ok := seen[offset]; ok {
			continue
		}
		seen[offset] = struct{}{}
		checkpoint := protocol.SyncCheckpoint{Offset: offset}
		if offset <= len(messages) {
			checkpoint.Hash = messages[offset-1].H
		}
		checkpoints = append(checkpoints, checkpoint)
	}

	sort.Slice(checkpoints, func(i, j int) bool {
		return checkpoints[i].Offset < checkpoints[j].Offset
	})

	accumulators := computeAccumulatorDigests(messages, req.AccumulatorOffsets)
	chunkSize := req.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 256
	}
	chunks := computeChunkDigests(messages, req.ChunkIndices, chunkSize)
	chunkRoot, chunkLeaves := computeChunkMerkleRoot(messages, chunkSize)
	chunkProofs := computeChunkMerkleProofs(messages, req.ChunkIndices, chunkSize)

	windowDigests := make([]protocol.SyncWindowDigest, 0, len(req.WindowEnds))
	windowSize := req.WindowSize
	if windowSize <= 0 {
		windowSize = 32
	}
	seenWindows := make(map[int]struct{}, len(req.WindowEnds))
	for _, endOffset := range req.WindowEnds {
		if endOffset <= 0 {
			continue
		}
		if _, ok := seenWindows[endOffset]; ok {
			continue
		}
		seenWindows[endOffset] = struct{}{}

		digest := protocol.SyncWindowDigest{
			EndOffset:  endOffset,
			WindowSize: windowSize,
		}
		if endOffset <= len(messages) {
			digest.Hash = computeWindowDigest(messages, endOffset, windowSize)
		}
		windowDigests = append(windowDigests, digest)
	}
	sort.Slice(windowDigests, func(i, j int) bool {
		return windowDigests[i].EndOffset < windowDigests[j].EndOffset
	})

	tipHash := ""
	if len(messages) > 0 {
		tipHash = messages[len(messages)-1].H
	}

	return &protocol.SyncMetaPayload{
		TotalMessages:     len(messages),
		TipHash:           tipHash,
		Checkpoints:       checkpoints,
		Accumulators:      accumulators,
		ChunkDigests:      chunks,
		ChunkMerkleRoot:   chunkRoot,
		ChunkMerkleLeaves: chunkLeaves,
		ChunkMerkleProofs: chunkProofs,
		WindowDigests:     windowDigests,
	}, nil
}

func computeAccumulatorDigests(messages []protocol.Message, offsets []int) []protocol.SyncAccumulatorDigest {
	if len(offsets) == 0 {
		return nil
	}

	target := make(map[int]struct{}, len(offsets))
	for _, offset := range offsets {
		if offset > 0 {
			target[offset] = struct{}{}
		}
	}
	if len(target) == 0 {
		return nil
	}

	outMap := make(map[int]string, len(target))
	acc := make([]byte, 32)
	for i := range messages {
		rawHash, err := hex.DecodeString(messages[i].H)
		if err != nil || len(rawHash) == 0 {
			rawHash = []byte(messages[i].H)
		}
		h := sha256.New()
		_, _ = h.Write(acc)
		_, _ = h.Write(rawHash)
		acc = h.Sum(nil)

		offset := i + 1
		if _, ok := target[offset]; ok {
			outMap[offset] = hex.EncodeToString(acc)
		}
	}

	out := make([]protocol.SyncAccumulatorDigest, 0, len(target))
	for offset := range target {
		digest := protocol.SyncAccumulatorDigest{Offset: offset}
		if hash, ok := outMap[offset]; ok {
			digest.Hash = hash
		}
		out = append(out, digest)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Offset < out[j].Offset
	})
	return out
}

func computeChunkDigests(messages []protocol.Message, indices []int, chunkSize int) []protocol.SyncChunkDigest {
	if len(indices) == 0 {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = 256
	}

	target := make(map[int]struct{}, len(indices))
	for _, idx := range indices {
		if idx >= 0 {
			target[idx] = struct{}{}
		}
	}
	if len(target) == 0 {
		return nil
	}

	out := make([]protocol.SyncChunkDigest, 0, len(target))
	for idx := range target {
		start := idx*chunkSize + 1
		end := (idx + 1) * chunkSize
		if end > len(messages) {
			end = len(messages)
		}

		digest := protocol.SyncChunkDigest{
			Index:       idx,
			StartOffset: start,
			EndOffset:   end,
		}
		if start <= len(messages) && start > 0 && start <= end {
			digest.Hash = computeRangeDigest(messages, start, end)
		}
		out = append(out, digest)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Index < out[j].Index
	})
	return out
}

func computeChunkMerkleRoot(messages []protocol.Message, chunkSize int) (string, int) {
	leafHashes := computeChunkLeafHashes(messages, chunkSize)
	if len(leafHashes) == 0 {
		return "", 0
	}
	levels := buildChunkMerkleLevels(leafHashes)
	if len(levels) == 0 || len(levels[len(levels)-1]) == 0 {
		return "", len(leafHashes)
	}
	root := levels[len(levels)-1][0]
	return hex.EncodeToString(root), len(leafHashes)
}

func computeChunkMerkleProofs(messages []protocol.Message, indices []int, chunkSize int) []protocol.SyncChunkProof {
	if len(indices) == 0 {
		return nil
	}

	target := make(map[int]struct{}, len(indices))
	for _, idx := range indices {
		if idx >= 0 {
			target[idx] = struct{}{}
		}
	}
	if len(target) == 0 {
		return nil
	}

	ordered := make([]int, 0, len(target))
	for idx := range target {
		ordered = append(ordered, idx)
	}
	sort.Ints(ordered)

	leafHashes := computeChunkLeafHashes(messages, chunkSize)
	if len(leafHashes) == 0 {
		out := make([]protocol.SyncChunkProof, 0, len(ordered))
		for _, idx := range ordered {
			out = append(out, protocol.SyncChunkProof{Index: idx})
		}
		return out
	}

	levels := buildChunkMerkleLevels(leafHashes)
	out := make([]protocol.SyncChunkProof, 0, len(ordered))
	for _, idx := range ordered {
		proof := protocol.SyncChunkProof{Index: idx}
		if idx >= len(leafHashes) {
			out = append(out, proof)
			continue
		}
		proof.LeafHash = leafHashes[idx]
		proof.Siblings = buildChunkMerkleProof(levels, idx)
		out = append(out, proof)
	}
	return out
}

func computeChunkLeafHashes(messages []protocol.Message, chunkSize int) []string {
	if len(messages) == 0 {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = 256
	}

	chunks := (len(messages) + chunkSize - 1) / chunkSize
	out := make([]string, 0, chunks)
	for idx := 0; idx < chunks; idx++ {
		start := idx*chunkSize + 1
		end := (idx + 1) * chunkSize
		if end > len(messages) {
			end = len(messages)
		}
		out = append(out, computeRangeDigest(messages, start, end))
	}
	return out
}

func buildChunkMerkleLevels(leafHashes []string) [][][]byte {
	if len(leafHashes) == 0 {
		return nil
	}

	current := make([][]byte, 0, len(leafHashes))
	for _, leaf := range leafHashes {
		current = append(current, decodeOrHashDigest(leaf))
	}

	levels := make([][][]byte, 0, 8)
	levels = append(levels, current)
	for len(current) > 1 {
		next := make([][]byte, 0, (len(current)+1)/2)
		for i := 0; i < len(current); i += 2 {
			left := current[i]
			right := left
			if i+1 < len(current) {
				right = current[i+1]
			}
			next = append(next, hashPair(left, right))
		}
		levels = append(levels, next)
		current = next
	}
	return levels
}

func buildChunkMerkleProof(levels [][][]byte, index int) []string {
	if len(levels) == 0 || index < 0 {
		return nil
	}
	if len(levels[0]) == 0 || index >= len(levels[0]) {
		return nil
	}

	siblings := make([]string, 0, len(levels)-1)
	idx := index
	for level := 0; level < len(levels)-1; level++ {
		nodes := levels[level]
		siblingIdx := idx ^ 1
		sibling := nodes[idx]
		if siblingIdx >= 0 && siblingIdx < len(nodes) {
			sibling = nodes[siblingIdx]
		}
		siblings = append(siblings, hex.EncodeToString(sibling))
		idx /= 2
	}
	return siblings
}

func decodeOrHashDigest(digest string) []byte {
	raw, err := hex.DecodeString(digest)
	if err == nil && len(raw) > 0 {
		return raw
	}
	sum := sha256.Sum256([]byte(digest))
	return sum[:]
}

func hashPair(left, right []byte) []byte {
	h := sha256.New()
	_, _ = h.Write(left)
	_, _ = h.Write(right)
	return h.Sum(nil)
}

func computeRangeDigest(messages []protocol.Message, startOffset, endOffset int) string {
	if startOffset <= 0 || endOffset < startOffset || endOffset > len(messages) {
		return ""
	}
	h := sha256.New()
	for i := startOffset - 1; i < endOffset; i++ {
		raw, err := hex.DecodeString(messages[i].H)
		if err != nil || len(raw) == 0 {
			_, _ = h.Write([]byte(messages[i].H))
			continue
		}
		_, _ = h.Write(raw)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func computeWindowDigest(messages []protocol.Message, endOffset, windowSize int) string {
	if endOffset <= 0 || windowSize <= 0 || endOffset > len(messages) {
		return ""
	}

	start := endOffset - windowSize
	if start < 0 {
		start = 0
	}

	h := sha256.New()
	for i := start; i < endOffset; i++ {
		raw, err := hex.DecodeString(messages[i].H)
		if err != nil || len(raw) == 0 {
			_, _ = h.Write([]byte(messages[i].H))
			continue
		}
		_, _ = h.Write(raw)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Store) loadAllLocked() ([]protocol.Message, error) {
	var out []protocol.Message
	segments, err := s.segmentFilesLocked()
	if err != nil {
		return nil, err
	}
	for _, segment := range segments {
		messages, err := loadSegmentMessages(segment.path, segment.compressed)
		if err != nil {
			return nil, err
		}
		out = append(out, messages...)
	}
	return out, nil
}

type segmentFile struct {
	index      int
	compressed bool
	path       string
}

func (s *Store) ensureWritableSegmentLocked(nextRecordBytes int64) (string, error) {
	active, err := s.activeSegmentLocked()
	if err != nil {
		return "", err
	}
	if active.path == "" {
		return s.segmentPath(0, false), nil
	}

	info, err := os.Stat(active.path)
	if err != nil {
		return "", err
	}
	if info.Size()+nextRecordBytes <= s.maxSegmentBytes || info.Size() == 0 {
		return active.path, nil
	}

	if err := s.compressSegmentLocked(active); err != nil {
		return "", err
	}
	return s.segmentPath(active.index+1, false), nil
}

func (s *Store) activeSegmentLocked() (segmentFile, error) {
	segments, err := s.segmentFilesLocked()
	if err != nil {
		return segmentFile{}, err
	}
	active := segmentFile{}
	for _, segment := range segments {
		if !segment.compressed && segment.index >= active.index {
			active = segment
		}
	}
	return active, nil
}

func (s *Store) segmentFilesLocked() ([]segmentFile, error) {
	entries, err := os.ReadDir(s.segmentsDir)
	if err != nil {
		return nil, err
	}
	segments := make([]segmentFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		index, compressed, ok := parseSegmentName(entry.Name())
		if !ok {
			continue
		}
		segments = append(segments, segmentFile{
			index:      index,
			compressed: compressed,
			path:       filepath.Join(s.segmentsDir, entry.Name()),
		})
	}
	sort.Slice(segments, func(i, j int) bool {
		if segments[i].index == segments[j].index {
			if segments[i].compressed == segments[j].compressed {
				return segments[i].path < segments[j].path
			}
			return segments[i].compressed
		}
		return segments[i].index < segments[j].index
	})
	return segments, nil
}

func (s *Store) compressSegmentLocked(segment segmentFile) error {
	src, err := os.Open(segment.path)
	if err != nil {
		return err
	}

	dstPath := s.segmentPath(segment.index, true)
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		_ = src.Close()
		return err
	}

	gz, err := gzip.NewWriterLevel(dst, gzip.BestCompression)
	if err != nil {
		_ = src.Close()
		_ = dst.Close()
		return err
	}
	if _, err := io.Copy(gz, src); err != nil {
		_ = gz.Close()
		_ = src.Close()
		_ = dst.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		_ = src.Close()
		_ = dst.Close()
		return err
	}
	if err := src.Close(); err != nil {
		_ = dst.Close()
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	return os.Remove(segment.path)
}

func (s *Store) segmentPath(index int, compressed bool) string {
	base := fmt.Sprintf("seg_%06d.log", index)
	if compressed {
		base += ".gz"
	}
	return filepath.Join(s.segmentsDir, base)
}

func parseSegmentName(name string) (index int, compressed bool, ok bool) {
	switch {
	case strings.HasSuffix(name, ".log.gz"):
		name = strings.TrimSuffix(name, ".log.gz")
		compressed = true
	case strings.HasSuffix(name, ".log"):
		name = strings.TrimSuffix(name, ".log")
	default:
		return 0, false, false
	}

	if !strings.HasPrefix(name, "seg_") {
		return 0, false, false
	}
	value, err := strconv.Atoi(strings.TrimPrefix(name, "seg_"))
	if err != nil {
		return 0, false, false
	}
	return value, compressed, true
}

func loadSegmentMessages(path string, compressed bool) ([]protocol.Message, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	reader := io.Reader(f)
	var gz *gzip.Reader
	if compressed {
		gz, err = gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		reader = gz
	}

	var out []protocol.Message
	for {
		header := make([]byte, 4)
		if _, err := io.ReadFull(reader, header); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return nil, err
		}
		recordLen := binary.LittleEndian.Uint32(header)
		record := make([]byte, recordLen)
		if _, err := io.ReadFull(reader, record); err != nil {
			return nil, err
		}
		var msg protocol.Message
		if err := json.Unmarshal(record, &msg); err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, nil
}

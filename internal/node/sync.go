package node

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"time"

	"aether/internal/protocol"
)

type Syncer struct {
	cfg   Config
	book  *PeerBook
	store *Store
	state *SyncState
}

func NewSyncer(cfg Config, book *PeerBook, store *Store, state *SyncState) *Syncer {
	return &Syncer{
		cfg:   cfg,
		book:  book,
		store: store,
		state: state,
	}
}

func (s *Syncer) SyncOnce(gossip *Gossip, seen map[string]struct{}) error {
	targets := candidatePeers(s.cfg, s.book)
	if len(targets) == 0 {
		return nil
	}

	var lastErr error
	for _, peer := range targets {
		cursor, err := s.cursorForPeer(peer)
		if err != nil {
			lastErr = err
			continue
		}
		metaReq := buildSyncMetaRequest(cursor, syncWindowSize(s.cfg))
		meta, err := fetchSyncMeta(s.cfg, peer, metaReq)
		if err != nil {
			markFailure(s.book, peer, s.cfg)
			Metrics().IncSyncFailure()
			lastErr = err
			continue
		}
		cursor = normalizeCursor(cursor, meta)

		if meta.TotalMessages == 0 || cursor.Offset == meta.TotalMessages {
			if err := s.setCursor(peer, cursor); err != nil {
				lastErr = err
				markFailure(s.book, peer, s.cfg)
				Metrics().IncSyncFailure()
				continue
			}
			if s.book != nil {
				s.book.MarkSuccess(peer)
			}
			continue
		}

		offset := cursor.Offset
		fetchedAny := false
		for {
			data, err := fetchSync(s.cfg, peer, offset, s.batchSize())
			if err != nil {
				markFailure(s.book, peer, s.cfg)
				Metrics().IncSyncFailure()
				lastErr = err
				break
			}
			nextOffset := offset
			if len(data.Messages) == 0 {
				nextOffset = data.NextOffset
			} else if data.NextOffset >= offset+len(data.Messages) {
				nextOffset = data.NextOffset
			} else {
				nextOffset = offset + len(data.Messages)
			}
			if len(data.Messages) == 0 {
				cursor.Offset = nextOffset
				cursor = s.hydrateCursorWindowDigests(cursor)
				if err := s.setCursor(peer, cursor); err != nil {
					markFailure(s.book, peer, s.cfg)
					Metrics().IncSyncFailure()
					lastErr = err
					break
				}
				if s.book != nil {
					s.book.MarkSuccess(peer)
				}
				offset = cursor.Offset
				if fetchedAny {
					return nil
				}
				break
			}
			fetchedAny = true
			batchOK := true
			applied := 0
			for i := range data.Messages {
				msg := data.Messages[i]
				if _, ok := seen[msg.H]; ok {
					cursor.LastHash = msg.H
					cursor.Checkpoints = updateCursorCheckpoints(cursor.Checkpoints, offset+i+1, msg.H)
					continue
				}
				if err := gossip.Validate(&msg); err != nil {
					markFailure(s.book, peer, s.cfg)
					Metrics().IncSyncFailure()
					lastErr = err
					batchOK = false
					break
				}
				if err := s.store.Append(&msg); err != nil {
					markFailure(s.book, peer, s.cfg)
					Metrics().IncSyncFailure()
					lastErr = err
					batchOK = false
					break
				}
				seen[msg.H] = struct{}{}
				cursor.LastHash = msg.H
				cursor.Checkpoints = updateCursorCheckpoints(cursor.Checkpoints, offset+i+1, msg.H)
				applied++
			}
			if !batchOK {
				break
			}
			Metrics().IncSyncBatch(applied)
			cursor.Offset = nextOffset
			cursor = s.hydrateCursorWindowDigests(cursor)
			if err := s.setCursor(peer, cursor); err != nil {
				markFailure(s.book, peer, s.cfg)
				Metrics().IncSyncFailure()
				lastErr = err
				break
			}
			if s.book != nil {
				s.book.MarkSuccess(peer)
			}
			offset = cursor.Offset
			if !data.HasMore {
				return nil
			}
		}
	}
	return lastErr
}

func fetchSync(cfg Config, target string, offset, batchSize int) (*protocol.SyncDataPayload, error) {
	conn, err := dialPeer(cfg, target)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_, err = clientHandshake(conn, cfg)
	if err != nil {
		return nil, err
	}

	payload, err := protocol.EncodeJSON(protocol.SyncRequestPayload{
		Offset:      offset,
		MaxMessages: batchSize,
	})
	if err != nil {
		return nil, err
	}
	if err := protocol.WriteFrame(conn, protocol.FrameSyncReq, payload); err != nil {
		return nil, err
	}

	frame, err := protocol.ReadFrame(conn)
	if err != nil {
		return nil, err
	}
	if frame.Type != protocol.FrameSyncData {
		return nil, errors.New("expected sync data response")
	}

	var data protocol.SyncDataPayload
	if err := protocol.DecodeJSON(frame.Payload, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func fetchSyncMeta(cfg Config, target string, req protocol.SyncMetaRequestPayload) (*protocol.SyncMetaPayload, error) {
	conn, err := dialPeer(cfg, target)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_, err = clientHandshake(conn, cfg)
	if err != nil {
		return nil, err
	}

	payload, err := protocol.EncodeJSON(req)
	if err != nil {
		return nil, err
	}
	if err := protocol.WriteFrame(conn, protocol.FrameSyncMeta, payload); err != nil {
		return nil, err
	}

	frame, err := protocol.ReadFrame(conn)
	if err != nil {
		return nil, err
	}
	if frame.Type != protocol.FrameSyncMeta {
		return nil, errors.New("expected sync meta response")
	}

	var data protocol.SyncMetaPayload
	if err := protocol.DecodeJSON(frame.Payload, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func (s *Syncer) cursorForPeer(peer string) (SyncCursor, error) {
	if s.state == nil {
		return SyncCursor{}, nil
	}
	return s.state.Get(peer)
}

func (s *Syncer) setCursor(peer string, cursor SyncCursor) error {
	if s.state == nil {
		return nil
	}
	return s.state.Set(peer, cursor)
}

func checkpointOffsets(cursor SyncCursor) []int {
	offsets := make([]int, 0, len(cursor.Checkpoints)+1)
	seen := make(map[int]struct{}, len(cursor.Checkpoints)+1)
	for _, checkpoint := range cursor.Checkpoints {
		if checkpoint.Offset <= 0 {
			continue
		}
		if _, ok := seen[checkpoint.Offset]; ok {
			continue
		}
		seen[checkpoint.Offset] = struct{}{}
		offsets = append(offsets, checkpoint.Offset)
	}
	if cursor.Offset > 0 {
		if _, ok := seen[cursor.Offset]; !ok {
			offsets = append(offsets, cursor.Offset)
		}
	}
	sort.Ints(offsets)
	return offsets
}

func buildSyncMetaRequest(cursor SyncCursor, windowSize int) protocol.SyncMetaRequestPayload {
	if windowSize <= 0 {
		windowSize = 32
	}
	return protocol.SyncMetaRequestPayload{
		Offsets:            checkpointOffsets(cursor),
		AccumulatorOffsets: accumulatorOffsets(cursor),
		WindowEnds:         windowDigestEnds(cursor),
		WindowSize:         windowSize,
	}
}

func accumulatorOffsets(cursor SyncCursor) []int {
	seen := make(map[int]struct{}, len(cursor.Accumulators)+1)
	out := make([]int, 0, len(cursor.Accumulators)+1)
	for _, digest := range cursor.Accumulators {
		if digest.Offset <= 0 {
			continue
		}
		if _, ok := seen[digest.Offset]; ok {
			continue
		}
		seen[digest.Offset] = struct{}{}
		out = append(out, digest.Offset)
	}
	if cursor.Offset > 0 {
		if _, ok := seen[cursor.Offset]; !ok {
			out = append(out, cursor.Offset)
		}
	}
	sort.Ints(out)
	return out
}

func windowDigestEnds(cursor SyncCursor) []int {
	seen := make(map[int]struct{}, len(cursor.WindowDigests)+1)
	out := make([]int, 0, len(cursor.WindowDigests)+1)
	for _, digest := range cursor.WindowDigests {
		if digest.EndOffset <= 0 {
			continue
		}
		if _, ok := seen[digest.EndOffset]; ok {
			continue
		}
		seen[digest.EndOffset] = struct{}{}
		out = append(out, digest.EndOffset)
	}
	if cursor.Offset > 0 {
		if _, ok := seen[cursor.Offset]; !ok {
			out = append(out, cursor.Offset)
		}
	}
	sort.Ints(out)
	return out
}

func normalizeCursor(cursor SyncCursor, meta *protocol.SyncMetaPayload) SyncCursor {
	if cursor.Offset < 0 {
		cursor.Offset = 0
	}
	if meta == nil {
		return cursor
	}
	if cursor.Offset > meta.TotalMessages {
		cursor.Offset = meta.TotalMessages
	}
	if cursor.Offset == 0 {
		cursor.Accumulators = filterValidAccumulators(cursor.Accumulators, meta.Accumulators)
		cursor.WindowDigests = filterValidWindowDigests(cursor.WindowDigests, meta.WindowDigests)
		return cursor
	}
	metaHashes := make(map[int]string, len(meta.Checkpoints))
	for _, checkpoint := range meta.Checkpoints {
		metaHashes[checkpoint.Offset] = checkpoint.Hash
	}
	if hash, ok := metaHashes[cursor.Offset]; ok && hash != "" && hash == cursor.LastHash {
		cursor.Checkpoints = filterValidCheckpoints(cursor.Checkpoints, metaHashes)
		cursor.Accumulators = filterValidAccumulators(cursor.Accumulators, meta.Accumulators)
		cursor.WindowDigests = filterValidWindowDigests(cursor.WindowDigests, meta.WindowDigests)
		return cursor
	}

	best := SyncCursor{}
	bestAccumulatorOffset := bestMatchingAccumulatorOffset(cursor.Accumulators, meta.Accumulators)
	if bestAccumulatorOffset > best.Offset {
		best.Offset = bestAccumulatorOffset
	}
	bestWindowOffset := bestMatchingWindowOffset(cursor.WindowDigests, meta.WindowDigests)
	if bestWindowOffset > best.Offset {
		best.Offset = bestWindowOffset
	}
	for _, checkpoint := range cursor.Checkpoints {
		hash, ok := metaHashes[checkpoint.Offset]
		if !ok || hash == "" || checkpoint.Hash != hash {
			continue
		}
		if checkpoint.Offset > best.Offset {
			best.Offset = checkpoint.Offset
			best.LastHash = checkpoint.Hash
		}
	}
	best.Checkpoints = filterValidCheckpoints(cursor.Checkpoints, metaHashes)
	best.Accumulators = filterValidAccumulators(cursor.Accumulators, meta.Accumulators)
	best.WindowDigests = filterValidWindowDigests(cursor.WindowDigests, meta.WindowDigests)
	return best
}

func (s *Syncer) batchSize() int {
	return syncBatchSize(s.cfg)
}

func syncBatchSize(cfg Config) int {
	size := cfg.SyncBatchSize
	if size <= 0 {
		size = 256
	}
	maxAllowed := maxSyncResponseMessages(cfg)
	if size > maxAllowed {
		return maxAllowed
	}
	return size
}

func syncWindowSize(cfg Config) int {
	size := cfg.SyncWindowSize
	if size <= 0 {
		return 32
	}
	return size
}

func RunMaintenance(ctx context.Context, cfg Config, book *PeerBook, syncer *Syncer, gossip *Gossip, seenFn func() map[string]struct{}, refreshFn func() error) {
	logger := NewLogger(cfg.LogLevel)

	runSync := func() {
		if book != nil {
			if err := book.Sweep(time.Now()); err != nil {
				logger.Warnf("maintenance", "peer state sweep failed: %v", err)
			}
		}
		if err := syncer.SyncOnce(gossip, seenFn()); err != nil {
			logger.Warnf("sync", "sync iteration failed: %v", err)
			return
		}
		if refreshFn != nil {
			if err := refreshFn(); err != nil {
				logger.Warnf("sync", "seen-state refresh failed after sync: %v", err)
			}
		}
	}

	if len(cfg.BootstrapPeers) > 0 {
		go runPeriodic(ctx, cfg.BootstrapInterval, func() {
			if err := Bootstrap(cfg, book); err != nil {
				logger.Warnf("bootstrap", "bootstrap iteration failed: %v", err)
			}
		})
	}
	go runPeriodic(ctx, cfg.SyncInterval, runSync)
}

func runPeriodic(ctx context.Context, interval time.Duration, fn func()) {
	if interval <= 0 {
		return
	}

	fn()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fn()
		}
	}
}

func updateCursorCheckpoints(checkpoints []protocol.SyncCheckpoint, offset int, hash string) []protocol.SyncCheckpoint {
	if offset <= 0 || hash == "" || !shouldCaptureCheckpoint(offset) {
		return checkpoints
	}

	replaced := false
	out := make([]protocol.SyncCheckpoint, 0, len(checkpoints)+1)
	for _, checkpoint := range checkpoints {
		if checkpoint.Offset == offset {
			out = append(out, protocol.SyncCheckpoint{Offset: offset, Hash: hash})
			replaced = true
			continue
		}
		out = append(out, checkpoint)
	}
	if !replaced {
		out = append(out, protocol.SyncCheckpoint{Offset: offset, Hash: hash})
	}
	return trimCheckpoints(out, 12)
}

func shouldCaptureCheckpoint(offset int) bool {
	if offset <= 0 {
		return false
	}
	if offset == 1 {
		return true
	}
	return offset&(offset-1) == 0 || offset%64 == 0
}

func trimCheckpoints(checkpoints []protocol.SyncCheckpoint, limit int) []protocol.SyncCheckpoint {
	if limit <= 0 || len(checkpoints) <= limit {
		return checkpoints
	}
	sort.Slice(checkpoints, func(i, j int) bool {
		return checkpoints[i].Offset < checkpoints[j].Offset
	})
	return append([]protocol.SyncCheckpoint(nil), checkpoints[len(checkpoints)-limit:]...)
}

func filterValidCheckpoints(checkpoints []protocol.SyncCheckpoint, metaHashes map[int]string) []protocol.SyncCheckpoint {
	out := make([]protocol.SyncCheckpoint, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		hash, ok := metaHashes[checkpoint.Offset]
		if !ok || hash == "" || checkpoint.Hash != hash {
			continue
		}
		out = append(out, checkpoint)
	}
	return trimCheckpoints(out, 12)
}

func bestMatchingAccumulatorOffset(cursorDigests, remoteDigests []protocol.SyncAccumulatorDigest) int {
	remote := make(map[int]string, len(remoteDigests))
	for _, digest := range remoteDigests {
		remote[digest.Offset] = digest.Hash
	}

	best := 0
	for _, digest := range cursorDigests {
		if digest.Offset <= 0 || digest.Hash == "" {
			continue
		}
		if remoteHash, ok := remote[digest.Offset]; ok && remoteHash == digest.Hash && digest.Offset > best {
			best = digest.Offset
		}
	}
	return best
}

func filterValidAccumulators(existing, remote []protocol.SyncAccumulatorDigest) []protocol.SyncAccumulatorDigest {
	remoteMap := make(map[int]string, len(remote))
	for _, digest := range remote {
		remoteMap[digest.Offset] = digest.Hash
	}

	out := make([]protocol.SyncAccumulatorDigest, 0, len(existing))
	for _, digest := range existing {
		if digest.Offset <= 0 || digest.Hash == "" {
			continue
		}
		if hash, ok := remoteMap[digest.Offset]; ok && hash == digest.Hash {
			out = append(out, digest)
		}
	}

	const limit = 12
	if len(out) <= limit {
		return out
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Offset < out[j].Offset
	})
	return append([]protocol.SyncAccumulatorDigest(nil), out[len(out)-limit:]...)
}

func bestMatchingWindowOffset(cursorDigests, remoteDigests []protocol.SyncWindowDigest) int {
	remote := make(map[string]string, len(remoteDigests))
	for _, digest := range remoteDigests {
		remote[windowDigestKey(digest.EndOffset, digest.WindowSize)] = digest.Hash
	}

	best := 0
	for _, digest := range cursorDigests {
		if digest.EndOffset <= 0 || digest.Hash == "" {
			continue
		}
		key := windowDigestKey(digest.EndOffset, digest.WindowSize)
		if remoteHash, ok := remote[key]; ok && remoteHash == digest.Hash && digest.EndOffset > best {
			best = digest.EndOffset
		}
	}
	return best
}

func filterValidWindowDigests(existing, remote []protocol.SyncWindowDigest) []protocol.SyncWindowDigest {
	remoteMap := make(map[string]string, len(remote))
	for _, digest := range remote {
		remoteMap[windowDigestKey(digest.EndOffset, digest.WindowSize)] = digest.Hash
	}

	out := make([]protocol.SyncWindowDigest, 0, len(existing))
	for _, digest := range existing {
		if digest.EndOffset <= 0 || digest.Hash == "" {
			continue
		}
		key := windowDigestKey(digest.EndOffset, digest.WindowSize)
		if hash, ok := remoteMap[key]; ok && hash == digest.Hash {
			out = append(out, digest)
		}
	}

	const limit = 12
	if len(out) <= limit {
		return out
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].EndOffset < out[j].EndOffset
	})
	return append([]protocol.SyncWindowDigest(nil), out[len(out)-limit:]...)
}

func windowDigestKey(endOffset, windowSize int) string {
	return strconv.Itoa(endOffset) + ":" + strconv.Itoa(windowSize)
}

func (s *Syncer) hydrateCursorWindowDigests(cursor SyncCursor) SyncCursor {
	if cursor.Offset <= 0 {
		return cursor
	}
	meta, err := s.store.SyncMetadata(protocol.SyncMetaRequestPayload{
		AccumulatorOffsets: []int{cursor.Offset},
		WindowEnds:         []int{cursor.Offset},
		WindowSize:         syncWindowSize(s.cfg),
	})
	if err != nil {
		return cursor
	}

	accumulatorMap := make(map[int]string, len(cursor.Accumulators)+len(meta.Accumulators))
	for _, digest := range cursor.Accumulators {
		if digest.Offset <= 0 || digest.Hash == "" {
			continue
		}
		accumulatorMap[digest.Offset] = digest.Hash
	}
	for _, digest := range meta.Accumulators {
		if digest.Offset <= 0 || digest.Hash == "" {
			continue
		}
		accumulatorMap[digest.Offset] = digest.Hash
	}
	accOut := make([]protocol.SyncAccumulatorDigest, 0, len(accumulatorMap))
	for offset, hash := range accumulatorMap {
		accOut = append(accOut, protocol.SyncAccumulatorDigest{Offset: offset, Hash: hash})
	}
	sort.Slice(accOut, func(i, j int) bool {
		return accOut[i].Offset < accOut[j].Offset
	})
	if len(accOut) > 12 {
		accOut = accOut[len(accOut)-12:]
	}
	cursor.Accumulators = accOut

	existing := make(map[string]protocol.SyncWindowDigest, len(cursor.WindowDigests)+len(meta.WindowDigests))
	for _, digest := range cursor.WindowDigests {
		if digest.EndOffset <= 0 || digest.Hash == "" {
			continue
		}
		existing[windowDigestKey(digest.EndOffset, digest.WindowSize)] = digest
	}
	for _, digest := range meta.WindowDigests {
		if digest.EndOffset <= 0 || digest.Hash == "" {
			continue
		}
		existing[windowDigestKey(digest.EndOffset, digest.WindowSize)] = digest
	}

	out := make([]protocol.SyncWindowDigest, 0, len(existing))
	for _, digest := range existing {
		out = append(out, digest)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].EndOffset < out[j].EndOffset
	})
	if len(out) > 12 {
		out = out[len(out)-12:]
	}
	cursor.WindowDigests = out
	return cursor
}

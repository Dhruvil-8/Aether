package node

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"aether/internal/protocol"
)

var (
	ErrNetworkMismatch = errors.New("peer network id mismatch")
	ErrNoPeerTarget    = errors.New("no peer target configured")
)

type PeerServer struct {
	cfg    Config
	store  *Store
	peers  *PeerBook
	gossip *Gossip

	mu               sync.Mutex
	conns            map[net.Conn]string
	seen             map[string]struct{}
	globalReadWindow time.Time
	globalReadBytes  int
}

func NewPeerServer(cfg Config, store *Store, peers *PeerBook) *PeerServer {
	return &PeerServer{
		cfg:    cfg,
		store:  store,
		peers:  peers,
		gossip: NewGossip(),
		conns:  make(map[net.Conn]string),
		seen:   make(map[string]struct{}),
	}
}

func (s *PeerServer) RestoreSeen() error {
	messages, err := s.store.LoadAll()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, msg := range messages {
		s.seen[msg.H] = struct{}{}
	}
	return nil
}

func (s *PeerServer) Seen() map[string]struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]struct{}, len(s.seen))
	for h := range s.seen {
		out[h] = struct{}{}
	}
	return out
}

func (s *PeerServer) ConnectKnownPeersOnce() {
	remaining := s.remainingPeerSlots()
	if remaining <= 0 {
		return
	}

	for _, peer := range candidatePeers(s.cfg, s.peers) {
		if shouldSkipPeer(s.cfg, peer) || s.isConnectedTo(peer) || isBackedOff(s.peers, peer) {
			continue
		}
		go s.connectPeer(peer)
		remaining--
		if remaining <= 0 {
			return
		}
	}
}

func (s *PeerServer) RequestPeersOnce() {
	s.mu.Lock()
	conns := make([]net.Conn, 0, len(s.conns))
	for conn := range s.conns {
		conns = append(conns, conn)
	}
	s.mu.Unlock()

	for _, conn := range conns {
		_ = protocol.WriteFrame(conn, protocol.FrameGetPeers, nil)
	}
}

func (s *PeerServer) Listen(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.cfg.ListenAddress)
	if err != nil {
		return err
	}
	defer ln.Close()

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			continue
		}
		go s.handleInboundConn(conn)
	}
}

func SendMessage(cfg Config, book *PeerBook, target string, msg *protocol.Message) error {
	logger := NewLogger(cfg.LogLevel)
	targets := []string{}
	if cleaned := cleanPeerAddress(target); cleaned != "" {
		targets = append(targets, cleaned)
	} else {
		targets = candidatePeers(cfg, book)
	}
	if len(targets) == 0 {
		return ErrNoPeerTarget
	}

	payload, err := protocol.EncodeJSON(protocol.MessagePayload{Message: *msg})
	if err != nil {
		return err
	}

	var lastErr error
	for _, peer := range targets {
		if isBackedOff(book, peer) {
			logger.Debugf("post", "skipping backed-off peer %s", peer)
			continue
		}
		conn, err := dialPeer(cfg, peer)
		if err != nil {
			markFailure(book, peer, cfg)
			Metrics().IncPeerDialFailure()
			logger.Warnf("post", "dial failed for %s: %v", peer, err)
			lastErr = err
			continue
		}

		hello, err := clientHandshake(conn, cfg)
		if err != nil {
			markFailure(book, peer, cfg)
			logger.Warnf("post", "handshake failed for %s: %v", peer, err)
			lastErr = err
			_ = conn.Close()
			continue
		}
		if book != nil {
			_, _ = book.Merge(peer, hello.ListenAddr)
			book.MarkSuccess(peer)
			book.MarkSuccess(hello.ListenAddr)
		}
		err = protocol.WriteFrame(conn, protocol.FrameMessage, payload)
		if err != nil {
			markFailure(book, peer, cfg)
			logger.Warnf("post", "message write failed for %s: %v", peer, err)
			_ = conn.Close()
			lastErr = err
			continue
		}
		if err := waitForAck(conn, msg.H); err != nil {
			markFailure(book, peer, cfg)
			logger.Warnf("post", "ack wait failed for %s: %v", peer, err)
			_ = conn.Close()
			lastErr = err
			continue
		}
		_ = conn.Close()
		logger.Debugf("post", "message relayed via %s", peer)
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return ErrNoPeerTarget
}

func (s *PeerServer) handleInboundConn(conn net.Conn) {
	peerAddr, err := s.serverHandshake(conn)
	if err != nil {
		if errors.Is(err, protocol.ErrFrameTooLarge) {
			markAbuse(s.peers, conn.RemoteAddr().String(), s.cfg)
		}
		_ = conn.Close()
		return
	}
	if s.connectionCount() >= maxOpenConnections(s.cfg) {
		markFailure(s.peers, peerAddr, s.cfg)
		Metrics().IncConnectionReject()
		_ = conn.Close()
		return
	}

	s.handleEstablishedConn(conn, peerAddr, false)
}

func (s *PeerServer) connectPeer(target string) {
	if isBackedOff(s.peers, target) {
		return
	}
	conn, err := dialPeer(s.cfg, target)
	if err != nil {
		Metrics().IncPeerDialFailure()
		markFailure(s.peers, target, s.cfg)
		return
	}

	hello, err := clientHandshake(conn, s.cfg)
	if err != nil {
		markFailure(s.peers, target, s.cfg)
		_ = conn.Close()
		return
	}
	if s.peers != nil {
		s.peers.MarkSuccess(target)
		s.peers.MarkSuccess(hello.ListenAddr)
	}

	peerAddr := target
	if cleanPeerAddress(hello.ListenAddr) != "" {
		peerAddr = cleanPeerAddress(hello.ListenAddr)
	}
	s.handleEstablishedConn(conn, peerAddr, true)
}

func (s *PeerServer) handleEstablishedConn(conn net.Conn, peerAddr string, outbound bool) {
	defer conn.Close()

	if cleanPeerAddress(peerAddr) == "" {
		if remote := cleanPeerAddress(conn.RemoteAddr().String()); remote != "" {
			peerAddr = remote
		}
	}

	s.mu.Lock()
	if len(s.conns) >= maxOpenConnections(s.cfg) {
		s.mu.Unlock()
		markFailure(s.peers, peerAddr, s.cfg)
		Metrics().IncConnectionReject()
		return
	}
	if peerAddr != "" {
		for _, addr := range s.conns {
			if addr == peerAddr {
				s.mu.Unlock()
				return
			}
		}
	}
	s.conns[conn] = peerAddr
	Metrics().SetOpenConnections(len(s.conns))
	s.mu.Unlock()
	if s.peers != nil {
		_, _ = s.peers.Merge(peerAddr)
	}
	if outbound {
		_ = protocol.WriteFrame(conn, protocol.FrameGetPeers, nil)
	}
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		Metrics().SetOpenConnections(len(s.conns))
		s.mu.Unlock()
	}()

	messageLimiter := newTokenBucket(s.cfg.MaxMessagesPerSec, time.Now())
	controlLimiter := newTokenBucket(maxControlFramesPerSecond(s.cfg), time.Now())
	connReadBytes := 0
	connFrames := 0

	for {
		_ = conn.SetReadDeadline(time.Now().Add(connectionIdleTimeout(s.cfg)))
		frame, err := protocol.ReadFrame(conn)
		if err != nil {
			if errors.Is(err, protocol.ErrFrameTooLarge) {
				markAbuse(s.peers, peerAddr, s.cfg)
			}
			return
		}
		if len(frame.Payload) > maxJSONPayloadBytes(s.cfg) {
			markAbuse(s.peers, peerAddr, s.cfg)
			return
		}
		frameBytes := len(frame.Payload) + 5
		connReadBytes += frameBytes
		connFrames++
		if connReadBytes > maxConnectionBytes(s.cfg) || connFrames > maxFramesPerConnection(s.cfg) {
			Metrics().IncResourceReject()
			markAbuse(s.peers, peerAddr, s.cfg)
			return
		}
		if !s.allowGlobalRead(frameBytes, time.Now()) {
			Metrics().IncResourceReject()
			markAbuse(s.peers, peerAddr, s.cfg)
			return
		}
		switch frame.Type {
		case protocol.FrameGetPeers:
			if !controlLimiter.allow(time.Now()) {
				markAbuse(s.peers, peerAddr, s.cfg)
				return
			}
			_ = s.sendPeers(conn)
		case protocol.FrameMessage:
			if !messageLimiter.allow(time.Now()) {
				return
			}
			var payload protocol.MessagePayload
			if err := protocol.DecodeJSON(frame.Payload, &payload); err != nil {
				markAbuse(s.peers, peerAddr, s.cfg)
				return
			}
			if err := s.acceptMessage(&payload.Message, conn); err != nil {
				markAbuse(s.peers, peerAddr, s.cfg)
				return
			}
			ack, err := protocol.EncodeJSON(protocol.AckPayload{Hash: payload.Message.H})
			if err != nil {
				return
			}
			if err := protocol.WriteFrame(conn, protocol.FrameAck, ack); err != nil {
				return
			}
		case protocol.FrameSyncReq:
			if !controlLimiter.allow(time.Now()) {
				markAbuse(s.peers, peerAddr, s.cfg)
				return
			}
			if err := s.sendSync(conn, frame.Payload); err != nil {
				markAbuse(s.peers, peerAddr, s.cfg)
				return
			}
		case protocol.FrameSyncMeta:
			if !controlLimiter.allow(time.Now()) {
				markAbuse(s.peers, peerAddr, s.cfg)
				return
			}
			if err := s.sendSyncMeta(conn, frame.Payload); err != nil {
				markAbuse(s.peers, peerAddr, s.cfg)
				return
			}
		case protocol.FramePeers:
			if !controlLimiter.allow(time.Now()) {
				markAbuse(s.peers, peerAddr, s.cfg)
				return
			}
			var payload protocol.PeersPayload
			if err := protocol.DecodeJSON(frame.Payload, &payload); err != nil {
				markAbuse(s.peers, peerAddr, s.cfg)
				return
			}
			if s.peers != nil {
				_, _ = s.peers.Merge(limitPeers(payload.Peers, s.cfg.MaxPeerAnnouncements)...)
			}
		case protocol.FramePing:
			if !controlLimiter.allow(time.Now()) {
				markAbuse(s.peers, peerAddr, s.cfg)
				return
			}
			_ = protocol.WriteFrame(conn, protocol.FramePong, nil)
		}
	}
}

func (s *PeerServer) serverHandshake(conn net.Conn) (string, error) {
	frame, err := protocol.ReadFrame(conn)
	if err != nil {
		return "", err
	}
	if frame.Type != protocol.FrameHello {
		return "", errors.New("expected hello frame")
	}

	var hello protocol.HelloPayload
	if err := protocol.DecodeJSON(frame.Payload, &hello); err != nil {
		return "", err
	}
	if hello.NetworkID != protocol.NetworkID() {
		return "", ErrNetworkMismatch
	}

	reply := protocol.HelloPayload{
		NetworkID:    protocol.NetworkID(),
		NodeVersion:  protocol.NodeVersion,
		Capabilities: capabilitiesForConfig(s.cfg),
		ListenAddr:   advertisedAddress(s.cfg),
	}
	data, err := protocol.EncodeJSON(reply)
	if err != nil {
		return "", err
	}
	if err := protocol.WriteFrame(conn, protocol.FrameHello, data); err != nil {
		return "", err
	}
	return hello.ListenAddr, nil
}

func clientHandshake(conn net.Conn, cfg Config) (protocol.HelloPayload, error) {
	hello := protocol.HelloPayload{
		NetworkID:    protocol.NetworkID(),
		NodeVersion:  protocol.NodeVersion,
		Capabilities: capabilitiesForConfig(cfg),
		ListenAddr:   advertisedAddress(cfg),
	}
	data, err := protocol.EncodeJSON(hello)
	if err != nil {
		return protocol.HelloPayload{}, err
	}
	if err := protocol.WriteFrame(conn, protocol.FrameHello, data); err != nil {
		return protocol.HelloPayload{}, err
	}

	frame, err := protocol.ReadFrame(conn)
	if err != nil {
		return protocol.HelloPayload{}, err
	}
	if frame.Type != protocol.FrameHello {
		return protocol.HelloPayload{}, errors.New("expected hello response")
	}

	var response protocol.HelloPayload
	if err := protocol.DecodeJSON(frame.Payload, &response); err != nil {
		return protocol.HelloPayload{}, err
	}
	if response.NetworkID != protocol.NetworkID() {
		return protocol.HelloPayload{}, ErrNetworkMismatch
	}
	return response, nil
}

func dialPeer(cfg Config, target string) (net.Conn, error) {
	if cfg.DevClearnet {
		timeout := cfg.TorDialTimeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		return net.DialTimeout("tcp", target, timeout)
	}
	return NewTorTransportWithTimeout(cfg.TorSOCKS, cfg.TorDialTimeout).Dial("tcp", target)
}

func (s *PeerServer) sendPeers(conn net.Conn) error {
	peers := candidatePeers(s.cfg, s.peers)
	addr := advertisedAddress(s.cfg)
	if addr != "" {
		peers = append(peers, addr)
	}
	payload, err := protocol.EncodeJSON(protocol.PeersPayload{
		Peers: limitPeers(peers, s.cfg.MaxPeersPerResponse),
	})
	if err != nil {
		return err
	}
	return protocol.WriteFrame(conn, protocol.FramePeers, payload)
}

func (s *PeerServer) sendSync(conn net.Conn, raw []byte) error {
	var req protocol.SyncRequestPayload
	if len(raw) > 0 {
		if err := protocol.DecodeJSON(raw, &req); err != nil {
			return err
		}
	}

	if req.Offset < 0 {
		req.Offset = 0
	}
	if !s.cfg.ArchiveMode {
		all, err := s.store.LoadAll()
		if err != nil {
			return err
		}
		oldestAllowed := len(all) - relayHistoryWindow(s.cfg)
		if oldestAllowed < 0 {
			oldestAllowed = 0
		}
		if req.Offset < oldestAllowed {
			req.Offset = oldestAllowed
		}
	}

	limit := req.MaxMessages
	if limit <= 0 || limit > maxSyncResponseMessages(s.cfg) {
		limit = maxSyncResponseMessages(s.cfg)
	}

	messages, nextOffset, hasMore, err := s.store.LoadRange(req.Offset, limit)
	if err != nil {
		return err
	}

	payload, err := protocol.EncodeJSON(protocol.SyncDataPayload{
		Messages:   messages,
		NextOffset: nextOffset,
		HasMore:    hasMore,
	})
	if err != nil {
		return err
	}
	if len(payload) > maxSyncResponseBytes(s.cfg) {
		return errors.New("sync response exceeds byte budget")
	}
	return protocol.WriteFrame(conn, protocol.FrameSyncData, payload)
}

func (s *PeerServer) sendSyncMeta(conn net.Conn, raw []byte) error {
	var req protocol.SyncMetaRequestPayload
	if len(raw) > 0 {
		if err := protocol.DecodeJSON(raw, &req); err != nil {
			return err
		}
	}
	req.Offsets = limitIntSlice(req.Offsets, maxSyncMetaOffsets(s.cfg))
	req.AccumulatorOffsets = limitIntSlice(req.AccumulatorOffsets, maxSyncMetaOffsets(s.cfg))
	req.ChunkIndices = limitIntSlice(req.ChunkIndices, maxSyncMetaOffsets(s.cfg))
	req.WindowEnds = limitIntSlice(req.WindowEnds, maxSyncMetaOffsets(s.cfg))
	if req.ChunkSize <= 0 {
		req.ChunkSize = syncChunkSize(s.cfg)
	}
	if req.WindowSize <= 0 {
		req.WindowSize = syncWindowSize(s.cfg)
	}

	meta, err := s.store.SyncMetadata(req)
	if err != nil {
		return err
	}
	payload, err := protocol.EncodeJSON(meta)
	if err != nil {
		return err
	}
	return protocol.WriteFrame(conn, protocol.FrameSyncMeta, payload)
}

func Bootstrap(cfg Config, book *PeerBook) error {
	logger := NewLogger(cfg.LogLevel)
	targets := candidatePeers(cfg, book)
	if len(targets) == 0 {
		return nil
	}

	discovered := make([]string, 0, len(targets))
	for _, peer := range targets {
		hello, peers, err := fetchPeers(cfg, peer)
		if err != nil {
			markFailure(book, peer, cfg)
			logger.Warnf("bootstrap", "peer discovery failed for %s: %v", peer, err)
			continue
		}
		if book != nil {
			book.MarkSuccess(peer)
			book.MarkSuccess(hello.ListenAddr)
		}
		discovered = append(discovered, peer, hello.ListenAddr)
		discovered = append(discovered, peers...)
	}
	if book == nil || len(discovered) == 0 {
		return nil
	}
	_, err := book.Merge(discovered...)
	return err
}

func fetchPeers(cfg Config, target string) (protocol.HelloPayload, []string, error) {
	conn, err := dialPeer(cfg, target)
	if err != nil {
		return protocol.HelloPayload{}, nil, err
	}
	defer conn.Close()

	hello, err := clientHandshake(conn, cfg)
	if err != nil {
		return protocol.HelloPayload{}, nil, err
	}
	if err := protocol.WriteFrame(conn, protocol.FrameGetPeers, nil); err != nil {
		return protocol.HelloPayload{}, nil, err
	}

	frame, err := protocol.ReadFrame(conn)
	if err != nil {
		return protocol.HelloPayload{}, nil, err
	}
	if frame.Type != protocol.FramePeers {
		return protocol.HelloPayload{}, nil, errors.New("expected peers response")
	}

	var peers protocol.PeersPayload
	if err := protocol.DecodeJSON(frame.Payload, &peers); err != nil {
		return protocol.HelloPayload{}, nil, err
	}
	return hello, peers.Peers, nil
}

func (s *PeerServer) acceptMessage(msg *protocol.Message, source net.Conn) error {
	if err := s.gossip.Validate(msg); err != nil {
		return err
	}

	s.mu.Lock()
	if _, ok := s.seen[msg.H]; ok {
		s.mu.Unlock()
		return nil
	}
	s.seen[msg.H] = struct{}{}
	s.mu.Unlock()

	if err := s.store.Append(msg); err != nil {
		return err
	}
	Metrics().IncMessagesAccepted()
	return s.broadcastMessage(msg, source)
}

func (s *PeerServer) broadcastMessage(msg *protocol.Message, source net.Conn) error {
	payload, err := protocol.EncodeJSON(protocol.MessagePayload{Message: *msg})
	if err != nil {
		return err
	}

	s.mu.Lock()
	conns := make([]net.Conn, 0, len(s.conns))
	for conn := range s.conns {
		if conn != source {
			conns = append(conns, conn)
		}
	}
	s.mu.Unlock()

	sent := 0
	for _, conn := range conns {
		if err := protocol.WriteFrame(conn, protocol.FrameMessage, payload); err != nil {
			_ = conn.Close()
			continue
		}
		sent++
	}
	Metrics().IncMessagesRelayed(sent)
	return nil
}

func capabilitiesForConfig(cfg Config) []string {
	capabilities := []string{"relay"}
	if cfg.RequireTorProxy {
		capabilities = append(capabilities, "tor-required")
	}
	if cfg.ArchiveMode {
		capabilities = append(capabilities, "archive")
	}
	return capabilities
}

func advertisedAddress(cfg Config) string {
	if cfg.AdvertiseAddr != "" {
		return cfg.AdvertiseAddr
	}
	if cfg.RequireTorProxy && !cfg.DevClearnet {
		return ""
	}
	return cfg.ListenAddress
}

func (s *PeerServer) isConnectedTo(peer string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, addr := range s.conns {
		if addr == peer {
			return true
		}
	}
	return false
}

func (s *PeerServer) remainingPeerSlots() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for _, addr := range s.conns {
		if cleanPeerAddress(addr) != "" {
			count++
		}
	}
	remaining := targetPeerCount(s.cfg) - count
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s *PeerServer) connectionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.conns)
}

func shouldSkipPeer(cfg Config, peer string) bool {
	peer = cleanPeerAddress(peer)
	if peer == "" {
		return true
	}
	self := cleanPeerAddress(advertisedAddress(cfg))
	if self != "" && peer == self {
		return true
	}
	return peer == cleanPeerAddress(cfg.ListenAddress)
}

func RunPeerConnector(ctx context.Context, cfg Config, server *PeerServer) {
	logger := NewLogger(cfg.LogLevel)
	runPeriodic(ctx, cfg.BootstrapInterval, func() {
		server.ConnectKnownPeersOnce()
		logger.Debugf("connector", "connector iteration complete")
	})
}

func RunPeerExchange(ctx context.Context, cfg Config, server *PeerServer) {
	logger := NewLogger(cfg.LogLevel)
	runPeriodic(ctx, cfg.PeerExchangeInterval, func() {
		server.RequestPeersOnce()
		logger.Debugf("peer-exchange", "peer exchange iteration complete")
	})
}

func waitForAck(conn net.Conn, hash string) error {
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer func() {
		_ = conn.SetReadDeadline(time.Time{})
	}()

	frame, err := protocol.ReadFrame(conn)
	if err != nil {
		return err
	}
	if frame.Type != protocol.FrameAck {
		return errors.New("expected ack response")
	}

	var ack protocol.AckPayload
	if err := protocol.DecodeJSON(frame.Payload, &ack); err != nil {
		return err
	}
	if ack.Hash != hash {
		return errors.New("ack hash mismatch")
	}
	return nil
}

func isBackedOff(book *PeerBook, peer string) bool {
	if book == nil {
		return false
	}
	return book.IsBackedOff(peer, time.Now())
}

func markFailure(book *PeerBook, peer string, cfg Config) {
	if book == nil {
		return
	}
	book.MarkPenalty(peer, 1, cfg.PeerBackoff, cfg.PeerBanThreshold, cfg.PeerBanDuration, time.Now())
}

func markAbuse(book *PeerBook, peer string, cfg Config) {
	Metrics().IncPeerAbuse()
	if book == nil {
		return
	}
	book.MarkPenalty(peer, 3, cfg.PeerBackoff, cfg.PeerBanThreshold, cfg.PeerBanDuration, time.Now())
}

func (s *PeerServer) allowGlobalRead(frameBytes int, now time.Time) bool {
	if frameBytes <= 0 {
		return true
	}
	limit := maxGlobalReadPerSecond(s.cfg)
	if limit <= 0 {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.globalReadWindow.IsZero() || now.Sub(s.globalReadWindow) >= time.Second {
		s.globalReadWindow = now
		s.globalReadBytes = 0
	}
	if s.globalReadBytes+frameBytes > limit {
		return false
	}
	s.globalReadBytes += frameBytes
	return true
}

func targetPeerCount(cfg Config) int {
	if cfg.TargetPeerCount <= 0 {
		return 8
	}
	return cfg.TargetPeerCount
}

func maxSyncResponseMessages(cfg Config) int {
	if cfg.MaxSyncResponseMsgs <= 0 {
		return 256
	}
	return cfg.MaxSyncResponseMsgs
}

func maxOpenConnections(cfg Config) int {
	if cfg.MaxOpenConnections <= 0 {
		return 32
	}
	return cfg.MaxOpenConnections
}

func maxControlFramesPerSecond(cfg Config) int {
	if cfg.MaxControlPerSec <= 0 {
		return 30
	}
	return cfg.MaxControlPerSec
}

func maxJSONPayloadBytes(cfg Config) int {
	if cfg.MaxJSONPayloadBytes <= 0 {
		return 1024 * 1024
	}
	return cfg.MaxJSONPayloadBytes
}

func maxConnectionBytes(cfg Config) int {
	if cfg.MaxConnBytes <= 0 {
		return 8 * 1024 * 1024
	}
	return cfg.MaxConnBytes
}

func maxGlobalReadPerSecond(cfg Config) int {
	if cfg.MaxGlobalReadPerSec <= 0 {
		return 32 * 1024 * 1024
	}
	return cfg.MaxGlobalReadPerSec
}

func maxFramesPerConnection(cfg Config) int {
	if cfg.MaxFramesPerConn <= 0 {
		return 4000
	}
	return cfg.MaxFramesPerConn
}

func maxSyncResponseBytes(cfg Config) int {
	if cfg.MaxSyncResponseBytes <= 0 {
		return 2 * 1024 * 1024
	}
	return cfg.MaxSyncResponseBytes
}

func maxSyncMetaOffsets(cfg Config) int {
	if cfg.MaxSyncMetaOffsets <= 0 {
		return 128
	}
	return cfg.MaxSyncMetaOffsets
}

func connectionIdleTimeout(cfg Config) time.Duration {
	if cfg.ConnectionIdleTimeout <= 0 {
		return 90 * time.Second
	}
	return cfg.ConnectionIdleTimeout
}

func relayHistoryWindow(cfg Config) int {
	if cfg.RelayHistoryWindow <= 0 {
		return 4096
	}
	return cfg.RelayHistoryWindow
}

func limitIntSlice(values []int, limit int) []int {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return append([]int(nil), values[:limit]...)
}

func limitPeers(peers []string, limit int) []string {
	if limit <= 0 {
		limit = 64
	}

	seen := make(map[string]struct{}, len(peers))
	out := make([]string, 0, min(limit, len(peers)))
	for _, peer := range peers {
		peer = cleanPeerAddress(peer)
		if peer == "" {
			continue
		}
		if _, ok := seen[peer]; ok {
			continue
		}
		seen[peer] = struct{}{}
		out = append(out, peer)
		if len(out) >= limit {
			break
		}
	}
	return out
}

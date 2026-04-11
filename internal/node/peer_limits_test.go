package node

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aether/internal/protocol"
)

func TestSendSyncClampsResponseBatch(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"one", "two", "three"} {
		msg := mustMineMessage(t, content)
		if err := store.Append(msg); err != nil {
			t.Fatal(err)
		}
	}

	server := NewPeerServer(Config{MaxSyncResponseMsgs: 2}, store, nil)
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	errCh := make(chan error, 1)
	go func() {
		defer serverConn.Close()
		req, err := protocol.EncodeJSON(protocol.SyncRequestPayload{
			Offset:      0,
			MaxMessages: 100,
		})
		if err != nil {
			errCh <- err
			return
		}
		errCh <- server.sendSync(serverConn, req)
	}()

	frame, err := protocol.ReadFrame(clientConn)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Type != protocol.FrameSyncData {
		t.Fatalf("unexpected frame type: %d", frame.Type)
	}

	var payload protocol.SyncDataPayload
	if err := protocol.DecodeJSON(frame.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Messages) != 2 {
		t.Fatalf("unexpected sync batch size: got %d want 2", len(payload.Messages))
	}
	if payload.NextOffset != 2 {
		t.Fatalf("unexpected next offset: got %d want 2", payload.NextOffset)
	}
	if !payload.HasMore {
		t.Fatal("expected has_more to be true")
	}

	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestSendSyncNonArchiveServesRecentWindowOnly(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	var expected []*protocol.Message
	for _, content := range []string{"one", "two", "three", "four", "five", "six"} {
		msg := mustMineMessage(t, content)
		expected = append(expected, msg)
		if err := store.Append(msg); err != nil {
			t.Fatal(err)
		}
	}

	server := NewPeerServer(Config{
		ArchiveMode:         false,
		RelayHistoryWindow:  2,
		MaxSyncResponseMsgs: 10,
	}, store, nil)
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	errCh := make(chan error, 1)
	go func() {
		defer serverConn.Close()
		req, err := protocol.EncodeJSON(protocol.SyncRequestPayload{
			Offset:      0,
			MaxMessages: 10,
		})
		if err != nil {
			errCh <- err
			return
		}
		errCh <- server.sendSync(serverConn, req)
	}()

	frame, err := protocol.ReadFrame(clientConn)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Type != protocol.FrameSyncData {
		t.Fatalf("unexpected frame type: %d", frame.Type)
	}

	var payload protocol.SyncDataPayload
	if err := protocol.DecodeJSON(frame.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Messages) != 2 {
		t.Fatalf("unexpected sync batch size for non-archive recent window: got %d want 2", len(payload.Messages))
	}
	if payload.Messages[0].H != expected[4].H || payload.Messages[1].H != expected[5].H {
		t.Fatal("expected non-archive sync response to serve only the recent relay window")
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestLimitPeersDeduplicatesAndCaps(t *testing.T) {
	peers := []string{
		" alpha:1 ",
		"beta:2",
		"alpha:1",
		"",
		"gamma:3",
	}

	got := limitPeers(peers, 2)
	if len(got) != 2 {
		t.Fatalf("unexpected peer count: got %d want 2", len(got))
	}
	if got[0] != "alpha:1" || got[1] != "beta:2" {
		t.Fatalf("unexpected limited peers: %v", got)
	}
}

func TestPeerAnnouncementsAreClamped(t *testing.T) {
	root := filepath.Join(t.TempDir(), "peer-limit")
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	book, err := OpenPeerBook(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewPeerServer(Config{
		MaxPeerAnnouncements: 2,
		DevClearnet:          true,
		RequireTorProxy:      false,
	}, store, book)

	clientConn, serverConn := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.handleEstablishedConn(serverConn, "remote:1", false)
	}()

	payload, err := protocol.EncodeJSON(protocol.PeersPayload{
		Peers: []string{"a:1", "b:2", "c:3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := protocol.WriteFrame(clientConn, protocol.FramePeers, payload); err != nil {
		t.Fatal(err)
	}
	_ = clientConn.Close()
	<-done

	peers, err := book.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 3 {
		t.Fatalf("unexpected stored peer count: got %d want 3", len(peers))
	}
	for _, peer := range peers {
		if peer == "c:3" {
			t.Fatalf("unexpected unclamped peer announcement: %v", peers)
		}
	}
}

func TestControlFrameRateLimitDisconnectsPeer(t *testing.T) {
	cfg := Config{
		ListenAddress:    nextListenAddress(t),
		DataDir:          filepath.Join(t.TempDir(), "control-limit"),
		MaxControlPerSec: 1,
		PeerBackoff:      time.Second,
		DevClearnet:      true,
		RequireTorProxy:  false,
	}

	store, err := OpenStore(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	book, err := OpenPeerBook(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	server := NewPeerServer(cfg, store, book)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = server.Listen(ctx)
	}()
	waitForListening(t, cfg.ListenAddress)

	clientConn, err := net.Dial("tcp", cfg.ListenAddress)
	if err != nil {
		t.Fatal(err)
	}
	defer clientConn.Close()

	clientCfg := Config{
		AdvertiseAddr:   "remote:1",
		DevClearnet:     true,
		RequireTorProxy: false,
	}
	if _, err := clientHandshake(clientConn, clientCfg); err != nil {
		t.Fatal(err)
	}

	if err := protocol.WriteFrame(clientConn, protocol.FramePing, nil); err != nil {
		t.Fatal(err)
	}
	frame, err := protocol.ReadFrame(clientConn)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Type != protocol.FramePong {
		t.Fatalf("unexpected first control response: %d", frame.Type)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
	defer func() {
		_ = clientConn.SetReadDeadline(time.Time{})
	}()
	writeErr := protocol.WriteFrame(clientConn, protocol.FramePing, nil)
	if writeErr != nil && !isPipeClosed(writeErr) {
		t.Fatalf("unexpected write error after control-frame limit: %v", writeErr)
	}
	_, err = protocol.ReadFrame(clientConn)
	if err == nil {
		t.Fatal("expected connection close after control-frame limit exceeded")
	}
	if isTimeout(err) {
		t.Fatalf("expected disconnect after control-frame limit, got timeout: %v", err)
	}
	if !errors.Is(err, net.ErrClosed) && !isPipeClosed(err) {
		t.Fatalf("unexpected read error after limit: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if book.IsBackedOff("remote:1", time.Now()) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected peer to be backed off after control-frame abuse")
}

func TestConnectionCapRejectsExtraPeer(t *testing.T) {
	root := filepath.Join(t.TempDir(), "conn-cap")
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	book, err := OpenPeerBook(root)
	if err != nil {
		t.Fatal(err)
	}
	server := NewPeerServer(Config{
		MaxOpenConnections: 1,
		PeerBackoff:        time.Second,
		DevClearnet:        true,
		RequireTorProxy:    false,
	}, store, book)

	clientConn1, serverConn1 := net.Pipe()
	done1 := make(chan struct{})
	go func() {
		defer close(done1)
		server.handleEstablishedConn(serverConn1, "peer-1", false)
	}()
	time.Sleep(50 * time.Millisecond)

	clientConn2, serverConn2 := net.Pipe()
	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		server.handleEstablishedConn(serverConn2, "peer-2", false)
	}()

	writeErr := protocol.WriteFrame(clientConn2, protocol.FramePing, nil)
	if writeErr != nil && !isPipeClosed(writeErr) {
		t.Fatalf("unexpected write error for capped peer: %v", writeErr)
	}
	if writeErr == nil {
		_ = clientConn2.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		_, err = protocol.ReadFrame(clientConn2)
		_ = clientConn2.SetReadDeadline(time.Time{})
		if err == nil {
			t.Fatal("expected second peer to be disconnected due to connection cap")
		}
		if isTimeout(err) {
			t.Fatalf("expected disconnect due to connection cap, got timeout: %v", err)
		}
	}
	_ = clientConn2.Close()
	<-done2

	if server.connectionCount() != 1 {
		t.Fatalf("unexpected connection count: got %d want 1", server.connectionCount())
	}
	if !book.IsBackedOff("peer-2", time.Now()) {
		t.Fatal("expected second peer to be backed off after connection-cap rejection")
	}

	_ = clientConn1.Close()
	<-done1
}

func TestOversizedJSONPayloadDisconnectsPeer(t *testing.T) {
	cfg := Config{
		ListenAddress:         nextListenAddress(t),
		DataDir:               filepath.Join(t.TempDir(), "json-limit"),
		MaxJSONPayloadBytes:   64,
		PeerBackoff:           time.Second,
		DevClearnet:           true,
		RequireTorProxy:       false,
		ConnectionIdleTimeout: 2 * time.Second,
	}

	store, err := OpenStore(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	book, err := OpenPeerBook(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	server := NewPeerServer(cfg, store, book)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = server.Listen(ctx)
	}()
	waitForListening(t, cfg.ListenAddress)

	clientConn, err := net.Dial("tcp", cfg.ListenAddress)
	if err != nil {
		t.Fatal(err)
	}
	defer clientConn.Close()

	clientCfg := Config{
		AdvertiseAddr:   "remote-json:1",
		DevClearnet:     true,
		RequireTorProxy: false,
	}
	if _, err := clientHandshake(clientConn, clientCfg); err != nil {
		t.Fatal(err)
	}

	oversized := `{"peers":["` + strings.Repeat("a", 256) + `"]}`
	if err := protocol.WriteFrame(clientConn, protocol.FramePeers, []byte(oversized)); err != nil {
		t.Fatal(err)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
	_, err = protocol.ReadFrame(clientConn)
	_ = clientConn.SetReadDeadline(time.Time{})
	if err == nil {
		t.Fatal("expected disconnect after oversized JSON payload")
	}
	if isTimeout(err) {
		t.Fatalf("expected disconnect after oversized JSON payload, got timeout: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if book.IsBackedOff("remote-json:1", time.Now()) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected peer to be backed off after oversized JSON payload")
}

func isPipeClosed(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, net.ErrClosed) || err.Error() == "EOF" || err.Error() == "io: read/write on closed pipe"
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}

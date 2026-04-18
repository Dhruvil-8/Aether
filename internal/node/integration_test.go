package node

import (
	"context"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"aether/internal/protocol"
)

func TestRelayPersistsIncomingMessage(t *testing.T) {
	cfg, store, cancel := startTestServer(t)
	defer cancel()

	msg := mustMineMessage(t, "hello relay")
	if err := SendMessage(testClientConfig(), nil, cfg.ListenAddress, msg); err != nil {
		t.Fatalf("send message failed: %v", err)
	}

	messages := waitForMessages(t, store, 1)
	if messages[0].H != msg.H {
		t.Fatalf("unexpected stored hash: got %s want %s", messages[0].H, msg.H)
	}
}

func TestSyncFetchesMissingMessages(t *testing.T) {
	cfgA, storeA, cancelA := startTestServer(t)
	defer cancelA()

	first := mustMineMessage(t, "first")
	second := mustMineMessage(t, "second")
	if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, first); err != nil {
		t.Fatalf("send first failed: %v", err)
	}
	if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, second); err != nil {
		t.Fatalf("send second failed: %v", err)
	}
	waitForMessages(t, storeA, 2)

	rootB := filepath.Join(t.TempDir(), "node-b")
	storeB, err := OpenStore(rootB)
	if err != nil {
		t.Fatal(err)
	}
	bookB, err := OpenPeerBook(rootB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bookB.Merge(cfgA.ListenAddress); err != nil {
		t.Fatal(err)
	}

	cfgB := Config{
		ListenAddress:   nextListenAddress(t),
		AdvertiseAddr:   "",
		DataDir:         rootB,
		BootstrapPeers:  []string{cfgA.ListenAddress},
		DevClearnet:     true,
		RequireTorProxy: false,
	}
	stateB, err := OpenSyncState(rootB)
	if err != nil {
		t.Fatal(err)
	}
	syncer := NewSyncer(cfgB, bookB, storeB, stateB)
	if err := syncer.SyncOnce(NewGossip(), map[string]struct{}{}); err != nil {
		t.Fatalf("sync once failed: %v", err)
	}

	got, err := storeB.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("unexpected sync count: got %d want 2", len(got))
	}
	if got[0].H != first.H || got[1].H != second.H {
		t.Fatalf("unexpected sync order or hashes")
	}
}

func TestSyncFetchesInBatches(t *testing.T) {
	cfgA, storeA, cancelA := startTestServer(t)
	defer cancelA()

	for _, content := range []string{"one", "two", "three"} {
		if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, mustMineMessage(t, content)); err != nil {
			t.Fatalf("send failed: %v", err)
		}
	}
	waitForMessages(t, storeA, 3)

	rootB := filepath.Join(t.TempDir(), "node-batch")
	storeB, err := OpenStore(rootB)
	if err != nil {
		t.Fatal(err)
	}
	bookB, err := OpenPeerBook(rootB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bookB.Merge(cfgA.ListenAddress); err != nil {
		t.Fatal(err)
	}

	cfgB := Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         rootB,
		BootstrapPeers:  []string{cfgA.ListenAddress},
		SyncBatchSize:   1,
		DevClearnet:     true,
		RequireTorProxy: false,
	}
	stateB, err := OpenSyncState(rootB)
	if err != nil {
		t.Fatal(err)
	}
	syncer := NewSyncer(cfgB, bookB, storeB, stateB)
	if err := syncer.SyncOnce(NewGossip(), map[string]struct{}{}); err != nil {
		t.Fatalf("batched sync failed: %v", err)
	}

	got, err := storeB.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("unexpected batched sync count: got %d want 3", len(got))
	}
}

func TestRelayPersistsBackToBackMessages(t *testing.T) {
	cfg, store, cancel := startTestServer(t)
	defer cancel()

	first := mustMineMessage(t, "burst-one")
	second := mustMineMessage(t, "burst-two")
	if err := SendMessage(testClientConfig(), nil, cfg.ListenAddress, first); err != nil {
		t.Fatalf("send first failed: %v", err)
	}
	if err := SendMessage(testClientConfig(), nil, cfg.ListenAddress, second); err != nil {
		t.Fatalf("send second failed: %v", err)
	}

	got := waitForMessages(t, store, 2)
	if got[0].H != first.H || got[1].H != second.H {
		t.Fatalf("unexpected stored messages after back-to-back relay")
	}
}

func TestMultiHopRelayAcrossPersistentPeers(t *testing.T) {
	cfgA, storeA, serverA, cancelA := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "node-a"),
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelA()

	cfgB, storeB, serverB, cancelB := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "node-b"),
		BootstrapPeers:  []string{cfgA.ListenAddress},
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelB()

	_, storeC, serverC, cancelC := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "node-c"),
		BootstrapPeers:  []string{cfgB.ListenAddress},
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelC()

	serverA.ConnectKnownPeersOnce()
	serverB.ConnectKnownPeersOnce()
	serverC.ConnectKnownPeersOnce()
	time.Sleep(200 * time.Millisecond)

	msg := mustMineMessage(t, "multi-hop")
	if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, msg); err != nil {
		t.Fatalf("send message failed: %v", err)
	}

	waitForMessages(t, storeA, 1)
	waitForMessages(t, storeB, 1)
	gotC := waitForMessages(t, storeC, 1)
	if gotC[0].H != msg.H {
		t.Fatalf("unexpected hash at hop 3: got %s want %s", gotC[0].H, msg.H)
	}
}

func TestLongerChainRelayWithMultipleMessages(t *testing.T) {
	cfgA, storeA, serverA, cancelA := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "chain-a"),
		TargetPeerCount: 1,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelA()

	cfgB, _, serverB, cancelB := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "chain-b"),
		BootstrapPeers:  []string{cfgA.ListenAddress},
		TargetPeerCount: 1,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelB()

	cfgC, _, serverC, cancelC := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "chain-c"),
		BootstrapPeers:  []string{cfgB.ListenAddress},
		TargetPeerCount: 1,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelC()

	cfgD, _, serverD, cancelD := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "chain-d"),
		BootstrapPeers:  []string{cfgC.ListenAddress},
		TargetPeerCount: 1,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelD()

	_, storeE, serverE, cancelE := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "chain-e"),
		BootstrapPeers:  []string{cfgD.ListenAddress},
		TargetPeerCount: 1,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelE()

	serverA.ConnectKnownPeersOnce()
	serverB.ConnectKnownPeersOnce()
	serverC.ConnectKnownPeersOnce()
	serverD.ConnectKnownPeersOnce()
	serverE.ConnectKnownPeersOnce()
	time.Sleep(300 * time.Millisecond)

	expected := make(map[string]struct{})
	for _, content := range []string{"m1", "m2", "m3"} {
		msg := mustMineMessage(t, content)
		expected[msg.H] = struct{}{}
		if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, msg); err != nil {
			t.Fatalf("send failed: %v", err)
		}
	}

	waitForMessages(t, storeA, 3)
	got := waitForMessages(t, storeE, 3)
	for _, msg := range got {
		if _, ok := expected[msg.H]; !ok {
			t.Fatalf("unexpected hash in far node: %s", msg.H)
		}
	}
}

func TestPeerExchangeLearnsNewPeerOverLiveConnection(t *testing.T) {
	cfgC, _, _, cancelC := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "peer-c"),
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelC()

	cfgB, _, serverB, cancelB := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "peer-b"),
		BootstrapPeers:  []string{cfgC.ListenAddress},
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelB()

	cfgA, _, serverA, cancelA := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "peer-a"),
		BootstrapPeers:  []string{cfgB.ListenAddress},
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelA()

	serverB.ConnectKnownPeersOnce()
	serverA.ConnectKnownPeersOnce()
	time.Sleep(200 * time.Millisecond)
	serverA.RequestPeersOnce()

	waitForPeerInBook(t, cfgA.DataDir, cfgC.ListenAddress)
}

func TestSendMessageFallsBackToHealthyPeer(t *testing.T) {
	deadAddr, liveAddr := nextOrderedListenAddresses(t)
	cfgLive, storeLive, _, cancelLive := startManagedTestServer(t, Config{
		ListenAddress:   liveAddr,
		DataDir:         filepath.Join(t.TempDir(), "fallback-live"),
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelLive()

	root := filepath.Join(t.TempDir(), "fallback-send")
	book, err := OpenPeerBook(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := book.Merge(deadAddr, cfgLive.ListenAddress); err != nil {
		t.Fatal(err)
	}

	msg := mustMineMessage(t, "fallback-send")
	if err := SendMessage(testClientConfig(), book, "", msg); err != nil {
		t.Fatalf("send with fallback failed: %v", err)
	}

	got := waitForMessages(t, storeLive, 1)
	if got[0].H != msg.H {
		t.Fatalf("unexpected delivered hash: got %s want %s", got[0].H, msg.H)
	}
	waitForPeerBackoff(t, book, deadAddr)
}

func TestRepeatedPostsPreferHealthyPeerWhileDeadPeerIsBackedOff(t *testing.T) {
	deadAddr, liveAddr := nextOrderedListenAddresses(t)
	cfgLive, storeLive, _, cancelLive := startManagedTestServer(t, Config{
		ListenAddress:   liveAddr,
		DataDir:         filepath.Join(t.TempDir(), "repeat-fallback-live"),
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelLive()

	root := filepath.Join(t.TempDir(), "repeat-fallback-send")
	book, err := OpenPeerBook(root)
	if err != nil {
		t.Fatal(err)
	}
	book.ApplyPolicy(Config{
		PeerScoreDecay:    time.Hour,
		PeerEvictionScore: 16,
		PeerBackoff:       10 * time.Second,
	})
	if _, err := book.Merge(deadAddr, cfgLive.ListenAddress); err != nil {
		t.Fatal(err)
	}

	expected := make(map[string]struct{})
	for i := 0; i < 5; i++ {
		msg := mustMineMessage(t, "repeat-fallback-"+time.Now().Add(time.Duration(i)*time.Second).Format(time.RFC3339Nano))
		expected[msg.H] = struct{}{}
		if err := SendMessage(Config{
			DevClearnet:     true,
			RequireTorProxy: false,
			PeerBackoff:     10 * time.Second,
		}, book, "", msg); err != nil {
			t.Fatalf("send %d failed: %v", i, err)
		}
	}

	got := waitForMessagesWithin(t, storeLive, 5, 3*time.Second)
	if len(got) != 5 {
		t.Fatalf("unexpected repeated fallback message count: got %d want 5", len(got))
	}
	for _, msg := range got {
		if _, ok := expected[msg.H]; !ok {
			t.Fatalf("unexpected repeated fallback hash: %s", msg.H)
		}
	}
	waitForPeerBackoff(t, book, deadAddr)
}

func TestSyncResumesFromPersistedOffset(t *testing.T) {
	cfgA, storeA, cancelA := startTestServer(t)
	defer cancelA()

	for _, content := range []string{"resume-one", "resume-two", "resume-three"} {
		if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, mustMineMessage(t, content)); err != nil {
			t.Fatalf("send failed: %v", err)
		}
	}
	waitForMessages(t, storeA, 3)

	rootB := filepath.Join(t.TempDir(), "resume-b")
	storeB, err := OpenStore(rootB)
	if err != nil {
		t.Fatal(err)
	}
	bookB, err := OpenPeerBook(rootB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bookB.Merge(cfgA.ListenAddress); err != nil {
		t.Fatal(err)
	}
	stateB, err := OpenSyncState(rootB)
	if err != nil {
		t.Fatal(err)
	}

	cfgB := Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         rootB,
		BootstrapPeers:  []string{cfgA.ListenAddress},
		SyncBatchSize:   1,
		DevClearnet:     true,
		RequireTorProxy: false,
	}
	syncer := NewSyncer(cfgB, bookB, storeB, stateB)
	if err := syncer.SyncOnce(NewGossip(), map[string]struct{}{}); err != nil {
		t.Fatalf("initial sync failed: %v", err)
	}

	offset, err := stateB.Get(cfgA.ListenAddress)
	if err != nil {
		t.Fatal(err)
	}
	if offset.Offset != 3 {
		t.Fatalf("unexpected stored offset after initial sync: got %d want 3", offset.Offset)
	}
	if offset.LastHash == "" {
		t.Fatal("expected stored last hash after initial sync")
	}

	fourth := mustMineMessage(t, "resume-four")
	if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, fourth); err != nil {
		t.Fatalf("send fourth failed: %v", err)
	}
	waitForMessages(t, storeA, 4)

	seen := make(map[string]struct{})
	existing, err := storeB.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range existing {
		seen[msg.H] = struct{}{}
	}
	if err := syncer.SyncOnce(NewGossip(), seen); err != nil {
		t.Fatalf("resume sync failed: %v", err)
	}

	got, err := storeB.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("unexpected resumed sync count: got %d want 4", len(got))
	}
	if got[3].H != fourth.H {
		t.Fatalf("unexpected resumed message hash: got %s want %s", got[3].H, fourth.H)
	}

	offset, err = stateB.Get(cfgA.ListenAddress)
	if err != nil {
		t.Fatal(err)
	}
	if offset.Offset != 4 {
		t.Fatalf("unexpected stored offset after resume sync: got %d want 4", offset.Offset)
	}
	if offset.LastHash != fourth.H {
		t.Fatalf("unexpected stored last hash after resume sync: got %s want %s", offset.LastHash, fourth.H)
	}
}

func TestSyncResetsMismatchedCursorUsingMetadata(t *testing.T) {
	cfgA, storeA, cancelA := startTestServer(t)
	defer cancelA()

	var expected []string
	for _, content := range []string{"meta-one", "meta-two", "meta-three"} {
		msg := mustMineMessage(t, content)
		expected = append(expected, msg.H)
		if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, msg); err != nil {
			t.Fatalf("send failed: %v", err)
		}
	}
	waitForMessages(t, storeA, 3)

	rootB := filepath.Join(t.TempDir(), "meta-reset")
	storeB, err := OpenStore(rootB)
	if err != nil {
		t.Fatal(err)
	}
	bookB, err := OpenPeerBook(rootB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bookB.Merge(cfgA.ListenAddress); err != nil {
		t.Fatal(err)
	}
	stateB, err := OpenSyncState(rootB)
	if err != nil {
		t.Fatal(err)
	}
	if err := stateB.Set(cfgA.ListenAddress, SyncCursor{Offset: 2, LastHash: "bad-hash"}); err != nil {
		t.Fatal(err)
	}

	cfgB := Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         rootB,
		BootstrapPeers:  []string{cfgA.ListenAddress},
		SyncBatchSize:   2,
		DevClearnet:     true,
		RequireTorProxy: false,
	}
	syncer := NewSyncer(cfgB, bookB, storeB, stateB)
	if err := syncer.SyncOnce(NewGossip(), map[string]struct{}{}); err != nil {
		t.Fatalf("metadata-reset sync failed: %v", err)
	}

	got, err := storeB.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("unexpected synced count after cursor reset: got %d want 3", len(got))
	}
	for i, msg := range got {
		if msg.H != expected[i] {
			t.Fatalf("unexpected message hash at %d: got %s want %s", i, msg.H, expected[i])
		}
	}

	cursor, err := stateB.Get(cfgA.ListenAddress)
	if err != nil {
		t.Fatal(err)
	}
	if cursor.Offset != 3 {
		t.Fatalf("unexpected cursor offset after reset sync: got %d want 3", cursor.Offset)
	}
	if cursor.LastHash != expected[2] {
		t.Fatalf("unexpected cursor hash after reset sync: got %s want %s", cursor.LastHash, expected[2])
	}
}

func TestSyncFallsBackToLastMatchingCheckpoint(t *testing.T) {
	cfgA, storeA, cancelA := startTestServer(t)
	defer cancelA()

	var expected []*protocol.Message
	for _, content := range []string{"checkpoint-one", "checkpoint-two", "checkpoint-three", "checkpoint-four"} {
		msg := mustMineMessage(t, content)
		expected = append(expected, msg)
		if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, msg); err != nil {
			t.Fatalf("send failed: %v", err)
		}
	}
	waitForMessages(t, storeA, 4)

	rootB := filepath.Join(t.TempDir(), "checkpoint-fallback")
	storeB, err := OpenStore(rootB)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := storeB.Append(expected[i]); err != nil {
			t.Fatal(err)
		}
	}
	bookB, err := OpenPeerBook(rootB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bookB.Merge(cfgA.ListenAddress); err != nil {
		t.Fatal(err)
	}
	stateB, err := OpenSyncState(rootB)
	if err != nil {
		t.Fatal(err)
	}
	if err := stateB.Set(cfgA.ListenAddress, SyncCursor{
		Offset:   4,
		LastHash: "bad-hash",
		Checkpoints: []protocol.SyncCheckpoint{
			{Offset: 2, Hash: expected[1].H},
			{Offset: 4, Hash: "bad-hash"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	cfgB := Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         rootB,
		BootstrapPeers:  []string{cfgA.ListenAddress},
		SyncBatchSize:   2,
		DevClearnet:     true,
		RequireTorProxy: false,
	}
	syncer := NewSyncer(cfgB, bookB, storeB, stateB)
	seen := map[string]struct{}{
		expected[0].H: {},
		expected[1].H: {},
	}
	if err := syncer.SyncOnce(NewGossip(), seen); err != nil {
		t.Fatalf("checkpoint fallback sync failed: %v", err)
	}

	got, err := storeB.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("unexpected synced count after checkpoint fallback: got %d want 4", len(got))
	}
	if got[2].H != expected[2].H || got[3].H != expected[3].H {
		t.Fatal("expected sync to resume from last matching checkpoint")
	}

	cursor, err := stateB.Get(cfgA.ListenAddress)
	if err != nil {
		t.Fatal(err)
	}
	if cursor.Offset != 4 {
		t.Fatalf("unexpected cursor offset after checkpoint fallback: got %d want 4", cursor.Offset)
	}
	if cursor.LastHash != expected[3].H {
		t.Fatalf("unexpected cursor last hash after checkpoint fallback: got %s want %s", cursor.LastHash, expected[3].H)
	}
}

func TestSyncFallsBackUsingWindowDigestWhenCheckpointsMismatch(t *testing.T) {
	cfgA, storeA, cancelA := startTestServer(t)
	defer cancelA()

	var expected []*protocol.Message
	for _, content := range []string{"window-one", "window-two", "window-three", "window-four", "window-five", "window-six"} {
		msg := mustMineMessage(t, content)
		expected = append(expected, msg)
		if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, msg); err != nil {
			t.Fatalf("send failed: %v", err)
		}
	}
	waitForMessages(t, storeA, 6)

	rootB := filepath.Join(t.TempDir(), "window-fallback")
	storeB, err := OpenStore(rootB)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := storeB.Append(expected[i]); err != nil {
			t.Fatal(err)
		}
	}
	meta, err := storeB.SyncMetadata(protocol.SyncMetaRequestPayload{
		WindowEnds: []int{4},
		WindowSize: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.WindowDigests) != 1 || meta.WindowDigests[0].Hash == "" {
		t.Fatal("expected local window digest for fallback test")
	}

	bookB, err := OpenPeerBook(rootB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bookB.Merge(cfgA.ListenAddress); err != nil {
		t.Fatal(err)
	}
	stateB, err := OpenSyncState(rootB)
	if err != nil {
		t.Fatal(err)
	}
	if err := stateB.Set(cfgA.ListenAddress, SyncCursor{
		Offset:      6,
		LastHash:    "bad-hash",
		Checkpoints: []protocol.SyncCheckpoint{{Offset: 2, Hash: "bad-hash"}},
		WindowDigests: []protocol.SyncWindowDigest{
			meta.WindowDigests[0],
		},
	}); err != nil {
		t.Fatal(err)
	}

	cfgB := Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         rootB,
		BootstrapPeers:  []string{cfgA.ListenAddress},
		SyncBatchSize:   2,
		SyncWindowSize:  2,
		DevClearnet:     true,
		RequireTorProxy: false,
	}
	syncer := NewSyncer(cfgB, bookB, storeB, stateB)
	seen := map[string]struct{}{}
	for i := 0; i < 4; i++ {
		seen[expected[i].H] = struct{}{}
	}
	if err := syncer.SyncOnce(NewGossip(), seen); err != nil {
		t.Fatalf("window fallback sync failed: %v", err)
	}

	got, err := storeB.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 6 {
		t.Fatalf("unexpected synced count after window fallback: got %d want 6", len(got))
	}
	if got[4].H != expected[4].H || got[5].H != expected[5].H {
		t.Fatal("expected sync to resume from matching window digest")
	}
}

func TestSyncFallsBackUsingAccumulatorDigestWhenOtherProofsMismatch(t *testing.T) {
	cfgA, storeA, cancelA := startTestServer(t)
	defer cancelA()

	var expected []*protocol.Message
	for _, content := range []string{"acc-one", "acc-two", "acc-three", "acc-four", "acc-five"} {
		msg := mustMineMessage(t, content)
		expected = append(expected, msg)
		if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, msg); err != nil {
			t.Fatalf("send failed: %v", err)
		}
	}
	waitForMessages(t, storeA, 5)

	rootB := filepath.Join(t.TempDir(), "acc-fallback")
	storeB, err := OpenStore(rootB)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := storeB.Append(expected[i]); err != nil {
			t.Fatal(err)
		}
	}
	meta, err := storeB.SyncMetadata(protocol.SyncMetaRequestPayload{
		AccumulatorOffsets: []int{3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.Accumulators) != 1 || meta.Accumulators[0].Hash == "" {
		t.Fatal("expected local accumulator digest for fallback test")
	}

	bookB, err := OpenPeerBook(rootB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bookB.Merge(cfgA.ListenAddress); err != nil {
		t.Fatal(err)
	}
	stateB, err := OpenSyncState(rootB)
	if err != nil {
		t.Fatal(err)
	}
	if err := stateB.Set(cfgA.ListenAddress, SyncCursor{
		Offset:      5,
		LastHash:    "bad-hash",
		Checkpoints: []protocol.SyncCheckpoint{{Offset: 2, Hash: "bad-hash"}},
		WindowDigests: []protocol.SyncWindowDigest{
			{EndOffset: 3, WindowSize: 2, Hash: "bad-window"},
		},
		Accumulators: []protocol.SyncAccumulatorDigest{
			meta.Accumulators[0],
		},
	}); err != nil {
		t.Fatal(err)
	}

	cfgB := Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         rootB,
		BootstrapPeers:  []string{cfgA.ListenAddress},
		SyncBatchSize:   2,
		SyncWindowSize:  2,
		DevClearnet:     true,
		RequireTorProxy: false,
	}
	syncer := NewSyncer(cfgB, bookB, storeB, stateB)
	seen := map[string]struct{}{
		expected[0].H: {},
		expected[1].H: {},
		expected[2].H: {},
	}
	if err := syncer.SyncOnce(NewGossip(), seen); err != nil {
		t.Fatalf("accumulator fallback sync failed: %v", err)
	}

	got, err := storeB.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("unexpected synced count after accumulator fallback: got %d want 5", len(got))
	}
	if got[3].H != expected[3].H || got[4].H != expected[4].H {
		t.Fatal("expected sync to resume from matching accumulator digest")
	}
}

func TestSyncFallsBackUsingChunkDigestWhenOtherProofsMismatch(t *testing.T) {
	cfgA, storeA, cancelA := startTestServer(t)
	defer cancelA()

	var expected []*protocol.Message
	for _, content := range []string{"chunk-one", "chunk-two", "chunk-three", "chunk-four", "chunk-five", "chunk-six"} {
		msg := mustMineMessage(t, content)
		expected = append(expected, msg)
		if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, msg); err != nil {
			t.Fatalf("send failed: %v", err)
		}
	}
	waitForMessages(t, storeA, 6)

	rootB := filepath.Join(t.TempDir(), "chunk-fallback")
	storeB, err := OpenStore(rootB)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := storeB.Append(expected[i]); err != nil {
			t.Fatal(err)
		}
	}
	meta, err := storeB.SyncMetadata(protocol.SyncMetaRequestPayload{
		ChunkIndices: []int{1},
		ChunkSize:    2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.ChunkDigests) != 1 || meta.ChunkDigests[0].Hash == "" {
		t.Fatal("expected local chunk digest for fallback test")
	}

	bookB, err := OpenPeerBook(rootB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bookB.Merge(cfgA.ListenAddress); err != nil {
		t.Fatal(err)
	}
	stateB, err := OpenSyncState(rootB)
	if err != nil {
		t.Fatal(err)
	}
	if err := stateB.Set(cfgA.ListenAddress, SyncCursor{
		Offset:      6,
		LastHash:    "bad-hash",
		Checkpoints: []protocol.SyncCheckpoint{{Offset: 2, Hash: "bad-hash"}},
		Accumulators: []protocol.SyncAccumulatorDigest{
			{Offset: 4, Hash: "bad-acc"},
		},
		WindowDigests: []protocol.SyncWindowDigest{
			{EndOffset: 4, WindowSize: 2, Hash: "bad-window"},
		},
		ChunkDigests: []protocol.SyncChunkDigest{
			meta.ChunkDigests[0],
		},
	}); err != nil {
		t.Fatal(err)
	}

	cfgB := Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         rootB,
		BootstrapPeers:  []string{cfgA.ListenAddress},
		SyncBatchSize:   2,
		SyncWindowSize:  2,
		SyncChunkSize:   2,
		DevClearnet:     true,
		RequireTorProxy: false,
	}
	syncer := NewSyncer(cfgB, bookB, storeB, stateB)
	seen := map[string]struct{}{
		expected[0].H: {},
		expected[1].H: {},
		expected[2].H: {},
		expected[3].H: {},
	}
	if err := syncer.SyncOnce(NewGossip(), seen); err != nil {
		t.Fatalf("chunk fallback sync failed: %v", err)
	}

	got, err := storeB.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 6 {
		t.Fatalf("unexpected synced count after chunk fallback: got %d want 6", len(got))
	}
	if got[4].H != expected[4].H || got[5].H != expected[5].H {
		t.Fatal("expected sync to resume from matching chunk digest")
	}
}

func TestSyncFallsBackToHealthyPeer(t *testing.T) {
	deadAddr, liveAddr := nextOrderedListenAddresses(t)
	cfgLive, storeLive, _, cancelLive := startManagedTestServer(t, Config{
		ListenAddress:   liveAddr,
		DataDir:         filepath.Join(t.TempDir(), "sync-fallback-live"),
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelLive()

	for _, content := range []string{"sync-fallback-one", "sync-fallback-two"} {
		if err := SendMessage(testClientConfig(), nil, cfgLive.ListenAddress, mustMineMessage(t, content)); err != nil {
			t.Fatalf("seed live peer failed: %v", err)
		}
	}
	waitForMessages(t, storeLive, 2)

	root := filepath.Join(t.TempDir(), "sync-fallback")
	storeB, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	bookB, err := OpenPeerBook(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := bookB.Merge(deadAddr, cfgLive.ListenAddress); err != nil {
		t.Fatal(err)
	}

	stateB, err := OpenSyncState(root)
	if err != nil {
		t.Fatal(err)
	}
	cfgB := Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         root,
		BootstrapPeers:  []string{deadAddr, cfgLive.ListenAddress},
		SyncBatchSize:   2,
		DevClearnet:     true,
		RequireTorProxy: false,
	}
	syncer := NewSyncer(cfgB, bookB, storeB, stateB)
	if err := syncer.SyncOnce(NewGossip(), map[string]struct{}{}); err != nil {
		t.Fatalf("sync fallback failed: %v", err)
	}

	got, err := storeB.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("unexpected synced count through fallback peer: got %d want 2", len(got))
	}
	waitForPeerBackoff(t, bookB, deadAddr)
}

func TestSyncConvergesUnderRepeatedDeadPeerFallback(t *testing.T) {
	deadAddr, liveAddr := nextOrderedListenAddresses(t)
	cfgLive, storeLive, _, cancelLive := startManagedTestServer(t, Config{
		ListenAddress:   liveAddr,
		DataDir:         filepath.Join(t.TempDir(), "sync-soak-live"),
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelLive()

	for i := 0; i < 20; i++ {
		content := "sync-soak-" + time.Now().Add(time.Duration(i)*time.Second).Format(time.RFC3339Nano)
		if err := SendMessage(testClientConfig(), nil, cfgLive.ListenAddress, mustMineMessage(t, content)); err != nil {
			t.Fatalf("seed live peer failed: %v", err)
		}
	}
	waitForMessages(t, storeLive, 20)

	root := filepath.Join(t.TempDir(), "sync-soak-target")
	storeB, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	bookB, err := OpenPeerBook(root)
	if err != nil {
		t.Fatal(err)
	}
	bookB.ApplyPolicy(Config{
		PeerBackoff:       time.Second,
		PeerScoreDecay:    time.Hour,
		PeerEvictionScore: 64,
	})
	if _, err := bookB.Merge(deadAddr, cfgLive.ListenAddress); err != nil {
		t.Fatal(err)
	}

	stateB, err := OpenSyncState(root)
	if err != nil {
		t.Fatal(err)
	}
	cfgB := Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         root,
		BootstrapPeers:  []string{deadAddr, cfgLive.ListenAddress},
		SyncBatchSize:   3,
		DevClearnet:     true,
		RequireTorProxy: false,
		PeerBackoff:     time.Second,
	}
	syncer := NewSyncer(cfgB, bookB, storeB, stateB)

	seen := map[string]struct{}{}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if err := syncer.SyncOnce(NewGossip(), seen); err != nil {
			// Dead peer failures are expected during sustained fallback loops.
		}
		got, err := storeB.LoadAll()
		if err != nil {
			t.Fatal(err)
		}
		for _, msg := range got {
			seen[msg.H] = struct{}{}
		}
		if len(got) == 20 {
			if !bookB.IsBackedOff(deadAddr, time.Now()) {
				t.Fatal("expected dead peer to be backed off during sustained fallback")
			}
			return
		}
		time.Sleep(120 * time.Millisecond)
	}

	got, err := storeB.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("sync did not converge under repeated dead-peer fallback: got %d want 20", len(got))
}

func TestRelaySurvivesPeerChurnInMesh(t *testing.T) {
	cfgA, storeA, serverA, cancelA := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "mesh-a"),
		BootstrapPeers:  nil,
		TargetPeerCount: 2,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelA()

	cfgB, _, serverB, cancelB := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "mesh-b"),
		BootstrapPeers:  []string{cfgA.ListenAddress},
		TargetPeerCount: 2,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelB()

	cfgC, _, serverC, cancelC := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "mesh-c"),
		BootstrapPeers:  []string{cfgA.ListenAddress, cfgB.ListenAddress},
		TargetPeerCount: 2,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelC()

	cfgD, _, serverD, cancelD := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "mesh-d"),
		BootstrapPeers:  []string{cfgB.ListenAddress, cfgC.ListenAddress},
		TargetPeerCount: 2,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelD()

	_, storeE, serverE, cancelE := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "mesh-e"),
		BootstrapPeers:  []string{cfgC.ListenAddress, cfgD.ListenAddress},
		TargetPeerCount: 2,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelE()

	serverA.ConnectKnownPeersOnce()
	serverB.ConnectKnownPeersOnce()
	serverC.ConnectKnownPeersOnce()
	serverD.ConnectKnownPeersOnce()
	serverE.ConnectKnownPeersOnce()
	time.Sleep(400 * time.Millisecond)

	first := mustMineMessage(t, "mesh-before-churn")
	if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, first); err != nil {
		t.Fatalf("pre-churn send failed: %v", err)
	}
	waitForMessages(t, storeA, 1)
	waitForMessagesWithin(t, storeE, 1, 4*time.Second)

	cancelC()
	time.Sleep(200 * time.Millisecond)

	for _, content := range []string{"mesh-after-1", "mesh-after-2", "mesh-after-3"} {
		msg := mustMineMessage(t, content)
		if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, msg); err != nil {
			t.Fatalf("post-churn send failed: %v", err)
		}
	}

	got := waitForMessagesWithin(t, storeE, 4, 6*time.Second)
	if len(got) != 4 {
		t.Fatalf("unexpected mesh message count after churn: got %d want 4", len(got))
	}
}

func TestRelayRecoversAfterBridgeNodeReturns(t *testing.T) {
	cfgA, _, serverA, cancelA := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "bridge-a"),
		TargetPeerCount: 1,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelA()

	cfgB := Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "bridge-b"),
		BootstrapPeers:  []string{cfgA.ListenAddress},
		TargetPeerCount: 2,
		DevClearnet:     true,
		RequireTorProxy: false,
	}
	_, _, serverB, cancelB := startManagedTestServer(t, cfgB)

	_, storeC, serverC, cancelC := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "bridge-c"),
		BootstrapPeers:  []string{cfgB.ListenAddress},
		TargetPeerCount: 1,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelC()

	serverA.ConnectKnownPeersOnce()
	serverB.ConnectKnownPeersOnce()
	serverC.ConnectKnownPeersOnce()
	time.Sleep(300 * time.Millisecond)

	cancelB()
	time.Sleep(250 * time.Millisecond)

	restartedCfgB := cfgB
	restartedCfgB.BootstrapPeers = []string{cfgA.ListenAddress}
	_, _, restartedB, restartCancelB := startManagedTestServer(t, restartedCfgB)
	defer restartCancelB()

	serverA.ConnectKnownPeersOnce()
	restartedB.ConnectKnownPeersOnce()
	serverC.ConnectKnownPeersOnce()
	time.Sleep(400 * time.Millisecond)

	msg := mustMineMessage(t, "bridge-recovery")
	if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, msg); err != nil {
		t.Fatalf("bridge recovery send failed: %v", err)
	}

	got := waitForMessagesWithin(t, storeC, 1, 5*time.Second)
	if got[0].H != msg.H {
		t.Fatalf("unexpected bridge recovery hash: got %s want %s", got[0].H, msg.H)
	}
}

func TestPartitionedChainConvergesAfterBridgeRecoveryAndSync(t *testing.T) {
	cfgA, storeA, serverA, cancelA := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "partition-a"),
		TargetPeerCount: 1,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelA()

	cfgB := Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "partition-b"),
		BootstrapPeers:  []string{cfgA.ListenAddress},
		TargetPeerCount: 2,
		DevClearnet:     true,
		RequireTorProxy: false,
	}
	_, _, serverB, cancelB := startManagedTestServer(t, cfgB)

	cfgC, storeC, serverC, cancelC := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "partition-c"),
		BootstrapPeers:  []string{cfgB.ListenAddress},
		TargetPeerCount: 1,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelC()

	serverA.ConnectKnownPeersOnce()
	serverB.ConnectKnownPeersOnce()
	serverC.ConnectKnownPeersOnce()
	time.Sleep(300 * time.Millisecond)

	for _, content := range []string{"pre-1", "pre-2", "pre-3"} {
		if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, mustMineMessage(t, content)); err != nil {
			t.Fatalf("pre-partition send failed: %v", err)
		}
	}
	waitForMessages(t, storeA, 3)
	waitForMessagesWithin(t, storeC, 3, 4*time.Second)

	cancelB()
	time.Sleep(250 * time.Millisecond)

	for _, content := range []string{"during-1", "during-2", "during-3"} {
		if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, mustMineMessage(t, content)); err != nil {
			t.Fatalf("partition send failed: %v", err)
		}
	}
	waitForMessages(t, storeA, 6)

	_, _, restartedB, restartCancelB := startManagedTestServer(t, cfgB)
	defer restartCancelB()

	serverA.ConnectKnownPeersOnce()
	restartedB.ConnectKnownPeersOnce()
	serverC.ConnectKnownPeersOnce()
	time.Sleep(400 * time.Millisecond)

	bookC, err := OpenPeerBook(cfgC.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	stateC, err := OpenSyncState(cfgC.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	cfgSyncC := Config{
		ListenAddress:   cfgC.ListenAddress,
		DataDir:         cfgC.DataDir,
		BootstrapPeers:  []string{cfgB.ListenAddress},
		SyncBatchSize:   3,
		DevClearnet:     true,
		RequireTorProxy: false,
	}
	syncer := NewSyncer(cfgSyncC, bookC, storeC, stateC)
	seen := loadSeenFromStore(t, storeC)
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		_ = syncer.SyncOnce(NewGossip(), seen)
		seen = loadSeenFromStore(t, storeC)
		if len(seen) >= 6 {
			return
		}
		time.Sleep(120 * time.Millisecond)
	}
	t.Fatalf("partition recovery sync did not converge: got %d want 6", len(seen))
}

func TestRepeatedBridgeFlapsStillConvergesViaSync(t *testing.T) {
	cfgA, storeA, serverA, cancelA := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "flap-a"),
		TargetPeerCount: 1,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelA()

	cfgB := Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "flap-b"),
		BootstrapPeers:  []string{cfgA.ListenAddress},
		TargetPeerCount: 2,
		DevClearnet:     true,
		RequireTorProxy: false,
	}
	_, _, serverB, cancelB := startManagedTestServer(t, cfgB)

	cfgC, storeC, serverC, cancelC := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "flap-c"),
		BootstrapPeers:  []string{cfgB.ListenAddress},
		TargetPeerCount: 1,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelC()

	serverA.ConnectKnownPeersOnce()
	serverB.ConnectKnownPeersOnce()
	serverC.ConnectKnownPeersOnce()
	time.Sleep(300 * time.Millisecond)

	for i := 0; i < 3; i++ {
		if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, mustMineMessage(t, "flap-up-"+strconv.Itoa(i))); err != nil {
			t.Fatalf("send while bridge up failed: %v", err)
		}

		cancelB()
		time.Sleep(150 * time.Millisecond)
		if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, mustMineMessage(t, "flap-down-"+strconv.Itoa(i))); err != nil {
			t.Fatalf("send while bridge down failed: %v", err)
		}

		_, _, serverB, cancelB = startManagedTestServer(t, cfgB)
		serverA.ConnectKnownPeersOnce()
		serverB.ConnectKnownPeersOnce()
		serverC.ConnectKnownPeersOnce()
		time.Sleep(250 * time.Millisecond)
	}
	defer cancelB()

	waitForMessages(t, storeA, 6)

	bookC, err := OpenPeerBook(cfgC.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	stateC, err := OpenSyncState(cfgC.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	syncer := NewSyncer(Config{
		ListenAddress:   cfgC.ListenAddress,
		DataDir:         cfgC.DataDir,
		BootstrapPeers:  []string{cfgB.ListenAddress},
		SyncBatchSize:   2,
		DevClearnet:     true,
		RequireTorProxy: false,
	}, bookC, storeC, stateC)

	seen := loadSeenFromStore(t, storeC)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		_ = syncer.SyncOnce(NewGossip(), seen)
		seen = loadSeenFromStore(t, storeC)
		if len(seen) >= 6 {
			return
		}
		time.Sleep(120 * time.Millisecond)
	}
	t.Fatalf("flap recovery sync did not converge: got %d want 6", len(seen))
}

func TestHighVolumeRelayAcrossLongChain(t *testing.T) {
	cfgA, _, serverA, cancelA := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "hv-a"),
		BootstrapPeers:  nil,
		TargetPeerCount: 1,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelA()

	cfgB, _, serverB, cancelB := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "hv-b"),
		BootstrapPeers:  []string{cfgA.ListenAddress},
		TargetPeerCount: 1,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelB()

	cfgC, _, serverC, cancelC := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "hv-c"),
		BootstrapPeers:  []string{cfgB.ListenAddress},
		TargetPeerCount: 1,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelC()

	cfgD, _, serverD, cancelD := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "hv-d"),
		BootstrapPeers:  []string{cfgC.ListenAddress},
		TargetPeerCount: 1,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelD()

	_, storeE, serverE, cancelE := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "hv-e"),
		BootstrapPeers:  []string{cfgD.ListenAddress},
		TargetPeerCount: 1,
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	defer cancelE()

	serverA.ConnectKnownPeersOnce()
	serverB.ConnectKnownPeersOnce()
	serverC.ConnectKnownPeersOnce()
	serverD.ConnectKnownPeersOnce()
	serverE.ConnectKnownPeersOnce()
	time.Sleep(350 * time.Millisecond)

	expected := make(map[string]struct{})
	for i := 0; i < 12; i++ {
		msg := mustMineMessage(t, "bulk-"+time.Now().Add(time.Duration(i)*time.Second).Format(time.RFC3339Nano))
		expected[msg.H] = struct{}{}
		if err := SendMessage(testClientConfig(), nil, cfgA.ListenAddress, msg); err != nil {
			t.Fatalf("bulk send %d failed: %v", i, err)
		}
	}

	got := waitForMessagesWithin(t, storeE, 12, 5*time.Second)
	if len(got) != 12 {
		t.Fatalf("unexpected far-node count: got %d want 12", len(got))
	}
	for _, msg := range got {
		if _, ok := expected[msg.H]; !ok {
			t.Fatalf("unexpected high-volume relay hash: %s", msg.H)
		}
	}
}

func startTestServer(t *testing.T) (Config, *Store, context.CancelFunc) {
	cfg, store, _, cancel := startManagedTestServer(t, Config{
		ListenAddress:   nextListenAddress(t),
		DataDir:         filepath.Join(t.TempDir(), "node"),
		DevClearnet:     true,
		RequireTorProxy: false,
	})
	return cfg, store, cancel
}

func startManagedTestServer(t *testing.T, cfg Config) (Config, *Store, *PeerServer, context.CancelFunc) {
	t.Helper()

	store, err := OpenStore(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	book, err := OpenPeerBook(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	book.ApplyPolicy(cfg)
	if len(cfg.BootstrapPeers) > 0 {
		if _, err := book.Merge(cfg.BootstrapPeers...); err != nil {
			t.Fatal(err)
		}
	}

	server := NewPeerServer(cfg, store, book)
	if err := server.RestoreSeen(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_ = server.Listen(ctx)
	}()
	waitForListening(t, cfg.ListenAddress)
	return cfg, store, server, cancel
}

func waitForListening(t *testing.T, address string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server did not start listening on %s", address)
}

func waitForMessages(t *testing.T, store *Store, count int) []protocol.Message {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	return waitForMessagesUntil(t, store, count, deadline)
}

func waitForMessagesWithin(t *testing.T, store *Store, count int, timeout time.Duration) []protocol.Message {
	t.Helper()
	return waitForMessagesUntil(t, store, count, time.Now().Add(timeout))
}

func waitForMessagesUntil(t *testing.T, store *Store, count int, deadline time.Time) []protocol.Message {
	t.Helper()
	for time.Now().Before(deadline) {
		messages, err := store.LoadAll()
		if err == nil && len(messages) >= count {
			return messages
		}
		time.Sleep(20 * time.Millisecond)
	}
	messages, err := store.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("store did not reach %d messages, got %d", count, len(messages))
	return nil
}

func assertMessageCountStays(t *testing.T, store *Store, expected int, duration time.Duration) {
	t.Helper()
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		messages, err := store.LoadAll()
		if err != nil {
			t.Fatal(err)
		}
		if len(messages) != expected {
			t.Fatalf("expected message count to stay %d during partition window, got %d", expected, len(messages))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func loadSeenFromStore(t *testing.T, store *Store) map[string]struct{} {
	t.Helper()
	messages, err := store.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]struct{}, len(messages))
	for _, msg := range messages {
		seen[msg.H] = struct{}{}
	}
	return seen
}

func waitForPeerInBook(t *testing.T, root, peer string) {
	t.Helper()
	book, err := OpenPeerBook(root)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		peers, err := book.Load()
		if err == nil {
			for _, existing := range peers {
				if existing == peer {
					return
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	peers, err := book.Load()
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("peer book did not learn %s, got %v", peer, peers)
}

func waitForPeerBackoff(t *testing.T, book *PeerBook, peer string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if book.IsBackedOff(peer, time.Now()) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("peer %s was not backed off in time", peer)
}

func nextOrderedListenAddresses(t *testing.T) (string, string) {
	t.Helper()
	first := nextListenAddress(t)
	second := nextListenAddress(t)
	if first == second {
		t.Fatal("expected distinct listen addresses")
	}
	if first < second {
		return first, second
	}
	return second, first
}

func mustMineMessage(t *testing.T, content string) *protocol.Message {
	t.Helper()
	msg, err := protocol.NewMessage(content)
	if err != nil {
		t.Fatal(err)
	}
	nonce, err := protocol.MinePoW(msg.H, protocol.Difficulty)
	if err != nil {
		t.Fatal(err)
	}
	msg.P = nonce
	return msg
}

func testClientConfig() Config {
	return Config{
		DevClearnet:     true,
		RequireTorProxy: false,
	}
}

func nextListenAddress(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

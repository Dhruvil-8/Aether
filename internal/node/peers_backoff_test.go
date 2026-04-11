package node

import (
	"testing"
	"time"
)

func TestPeerBookBackoffEscalatesAndClearsOnSuccess(t *testing.T) {
	book, err := OpenPeerBook(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	book.ApplyPolicy(Config{PeerScoreDecay: time.Hour, PeerEvictionScore: 16})

	now := time.Unix(100, 0)
	book.MarkPenalty("peer.onion:9000", 1, time.Second, 8, time.Minute, now)
	if !book.IsBackedOff("peer.onion:9000", now.Add(500*time.Millisecond)) {
		t.Fatal("expected peer to be backed off after first failure")
	}

	book.MarkPenalty("peer.onion:9000", 1, time.Second, 8, time.Minute, now.Add(2*time.Second))
	if !book.IsBackedOff("peer.onion:9000", now.Add(3500*time.Millisecond)) {
		t.Fatal("expected peer to remain backed off after repeated failure")
	}
	if score := book.PenaltyScoreAt("peer.onion:9000", now.Add(3500*time.Millisecond)); score != 2 {
		t.Fatalf("unexpected penalty score before success: got %d want 2", score)
	}

	book.MarkSuccessAt("peer.onion:9000", now.Add(4*time.Second))
	if book.IsBackedOff("peer.onion:9000", now.Add(4*time.Second)) {
		t.Fatal("expected backoff to clear after success")
	}
	if score := book.PenaltyScoreAt("peer.onion:9000", now.Add(4*time.Second)); score != 1 {
		t.Fatalf("unexpected penalty score after success: got %d want 1", score)
	}
}

func TestPeerBookBansRepeatOffender(t *testing.T) {
	book, err := OpenPeerBook(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	book.ApplyPolicy(Config{PeerScoreDecay: time.Hour, PeerEvictionScore: 16})

	now := time.Unix(200, 0)
	book.MarkPenalty("peer.onion:9000", 3, time.Second, 6, 10*time.Minute, now)
	if book.IsBanned("peer.onion:9000", now) {
		t.Fatal("did not expect peer to be banned after first abuse strike")
	}

	book.MarkPenalty("peer.onion:9000", 3, time.Second, 6, 10*time.Minute, now.Add(time.Second))
	if !book.IsBanned("peer.onion:9000", now.Add(2*time.Second)) {
		t.Fatal("expected peer to be banned after repeated abuse")
	}
	if !book.IsBackedOff("peer.onion:9000", now.Add(2*time.Second)) {
		t.Fatal("expected banned peer to also be considered unavailable")
	}
}

func TestPeerBookBanExpires(t *testing.T) {
	book, err := OpenPeerBook(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	book.ApplyPolicy(Config{PeerScoreDecay: time.Hour, PeerEvictionScore: 16})

	now := time.Unix(300, 0)
	book.MarkPenalty("peer.onion:9000", 8, time.Second, 8, 5*time.Second, now)
	if !book.IsBanned("peer.onion:9000", now.Add(time.Second)) {
		t.Fatal("expected active ban")
	}
	if book.IsBanned("peer.onion:9000", now.Add(6*time.Second)) {
		t.Fatal("expected ban to expire")
	}
}

func TestPeerBookStatePersistsAcrossReopen(t *testing.T) {
	root := t.TempDir()
	book, err := OpenPeerBook(root)
	if err != nil {
		t.Fatal(err)
	}
	book.ApplyPolicy(Config{PeerScoreDecay: time.Hour, PeerEvictionScore: 16})

	now := time.Unix(400, 0)
	book.MarkPenalty("peer.onion:9000", 8, time.Second, 8, 10*time.Minute, now)

	reopened, err := OpenPeerBook(root)
	if err != nil {
		t.Fatal(err)
	}
	reopened.ApplyPolicy(Config{PeerScoreDecay: time.Hour, PeerEvictionScore: 16})
	if score := reopened.PenaltyScoreAt("peer.onion:9000", now.Add(time.Second)); score != 8 {
		t.Fatalf("unexpected persisted score: got %d want 8", score)
	}
	if !reopened.IsBanned("peer.onion:9000", now.Add(time.Second)) {
		t.Fatal("expected persisted ban after reopen")
	}
}

func TestPeerBookPenaltyDecaysOverTime(t *testing.T) {
	book, err := OpenPeerBook(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	book.ApplyPolicy(Config{PeerScoreDecay: 2 * time.Second, PeerEvictionScore: 16})

	now := time.Unix(500, 0)
	book.MarkPenalty("peer.onion:9000", 5, time.Second, 10, time.Minute, now)

	if score := book.PenaltyScoreAt("peer.onion:9000", now.Add(5*time.Second)); score != 3 {
		t.Fatalf("unexpected decayed score after 5s: got %d want 3", score)
	}
	if score := book.PenaltyScoreAt("peer.onion:9000", now.Add(11*time.Second)); score != 0 {
		t.Fatalf("unexpected decayed score after 11s: got %d want 0", score)
	}
}

func TestPeerBookEvictsPersistedPeerAtEvictionThreshold(t *testing.T) {
	root := t.TempDir()
	book, err := OpenPeerBook(root)
	if err != nil {
		t.Fatal(err)
	}
	book.ApplyPolicy(Config{PeerScoreDecay: time.Hour, PeerEvictionScore: 4})

	if _, err := book.Merge("peer.onion:9000", "other.onion:9000"); err != nil {
		t.Fatal(err)
	}

	now := time.Unix(600, 0)
	book.MarkPenalty("peer.onion:9000", 4, time.Second, 8, time.Minute, now)

	peers, err := book.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 || peers[0] != "other.onion:9000" {
		t.Fatalf("unexpected persisted peers after eviction: %v", peers)
	}
	if book.IsCandidateAllowed("peer.onion:9000", now.Add(time.Second)) {
		t.Fatal("expected evicted peer to be disallowed during eviction cooldown")
	}
}

func TestPeerBookSweepRemovesFullyDecayedState(t *testing.T) {
	root := t.TempDir()
	book, err := OpenPeerBook(root)
	if err != nil {
		t.Fatal(err)
	}
	book.ApplyPolicy(Config{PeerScoreDecay: time.Second, PeerEvictionScore: 16})

	now := time.Unix(700, 0)
	book.MarkPenalty("peer.onion:9000", 2, time.Second, 8, time.Minute, now)
	if err := book.Sweep(now.Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if score := book.PenaltyScoreAt("peer.onion:9000", now.Add(3*time.Second)); score != 0 {
		t.Fatalf("unexpected score after sweep: got %d want 0", score)
	}

	reopened, err := OpenPeerBook(root)
	if err != nil {
		t.Fatal(err)
	}
	reopened.ApplyPolicy(Config{PeerScoreDecay: time.Second, PeerEvictionScore: 16})
	if score := reopened.PenaltyScoreAt("peer.onion:9000", now.Add(3*time.Second)); score != 0 {
		t.Fatalf("unexpected reopened score after sweep: got %d want 0", score)
	}
}

func TestPeerBookEvictionCooldownBlocksImmediateReintroduction(t *testing.T) {
	root := t.TempDir()
	book, err := OpenPeerBook(root)
	if err != nil {
		t.Fatal(err)
	}
	book.ApplyPolicy(Config{
		PeerScoreDecay:       time.Hour,
		PeerEvictionScore:    4,
		PeerEvictionCooldown: 10 * time.Second,
	})
	if _, err := book.Merge("peer.onion:9000"); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	book.MarkPenalty("peer.onion:9000", 4, time.Second, 8, time.Minute, now)
	if _, err := book.Merge("peer.onion:9000"); err != nil {
		t.Fatal(err)
	}
	peers, err := book.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 0 {
		t.Fatalf("expected peer to stay evicted during cooldown, got %v", peers)
	}
	if !book.IsCandidateAllowed("peer.onion:9000", now.Add(11*time.Second)) {
		t.Fatal("expected peer to become candidate-eligible after eviction cooldown")
	}
}

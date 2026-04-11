package node

import (
	"testing"
	"time"
)

func TestTokenBucketAllowsBurstUpToCapacity(t *testing.T) {
	now := time.Unix(100, 0)
	bucket := newTokenBucket(2, now)
	if !bucket.allow(now) {
		t.Fatal("expected first token to pass")
	}
	if !bucket.allow(now) {
		t.Fatal("expected second token to pass")
	}
	if bucket.allow(now) {
		t.Fatal("expected third token to be rejected")
	}
}

func TestTokenBucketRefillsOverTime(t *testing.T) {
	now := time.Unix(100, 0)
	bucket := newTokenBucket(2, now)
	_ = bucket.allow(now)
	_ = bucket.allow(now)
	if bucket.allow(now) {
		t.Fatal("expected bucket to be empty")
	}
	if !bucket.allow(now.Add(600 * time.Millisecond)) {
		t.Fatal("expected partial refill to allow one message")
	}
}

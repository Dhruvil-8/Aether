package node

import "time"

const defaultMaxMessagesPerSecond = 20

type tokenBucket struct {
	capacity float64
	tokens   float64
	refill   float64
	last     time.Time
}

func newTokenBucket(ratePerSecond int, now time.Time) *tokenBucket {
	if ratePerSecond <= 0 {
		ratePerSecond = defaultMaxMessagesPerSecond
	}
	rate := float64(ratePerSecond)
	return &tokenBucket{
		capacity: rate,
		tokens:   rate,
		refill:   rate,
		last:     now,
	}
}

func (b *tokenBucket) allow(now time.Time) bool {
	elapsed := now.Sub(b.last).Seconds()
	b.last = now
	b.tokens += elapsed * b.refill
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

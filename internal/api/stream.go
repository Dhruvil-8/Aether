package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"aether/internal/node"
)

// StreamHub polls the store for new messages and broadcasts them to SSE clients.
type StreamHub struct {
	store    *node.Store
	interval time.Duration

	mu      sync.Mutex
	clients map[chan MessageView]struct{}
}

func NewStreamHub(store *node.Store, interval time.Duration) *StreamHub {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return &StreamHub{
		store:    store,
		interval: interval,
		clients:  make(map[chan MessageView]struct{}),
	}
}

// Run starts the polling loop. Blocks until ctx is cancelled.
func (h *StreamHub) Run(ctx context.Context) {
	lastCount := 0
	messages, err := h.store.LoadAll()
	if err == nil {
		lastCount = len(messages)
	}

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			messages, err := h.store.LoadAll()
			if err != nil {
				continue
			}
			if len(messages) <= lastCount {
				continue
			}
			newMessages := messages[lastCount:]
			lastCount = len(messages)

			h.mu.Lock()
			for _, msg := range newMessages {
				view := toMessageView(msg)
				for ch := range h.clients {
					select {
					case ch <- view:
					default:
						// slow client, skip
					}
				}
			}
			h.mu.Unlock()
		}
	}
}

func (h *StreamHub) subscribe() chan MessageView {
	ch := make(chan MessageView, 64)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *StreamHub) unsubscribe(ch chan MessageView) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
	close(ch)
}

// handleTimelineStream is the SSE endpoint for real-time message streaming.
func (s *Server) handleTimelineStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	flusher.Flush()

	ch := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

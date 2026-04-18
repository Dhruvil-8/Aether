package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"aether/internal/node"
	"aether/internal/protocol"
)

func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	messages, nextOffset, hasMore, err := s.store.LoadRange(offset, limit)
	if err != nil {
		http.Error(w, "failed to load messages", http.StatusInternalServerError)
		return
	}

	resp := TimelineResponse{
		Messages:   make([]MessageView, 0, len(messages)),
		NextOffset: nextOffset,
		HasMore:    hasMore,
	}
	for _, msg := range messages {
		resp.Messages = append(resp.Messages, toMessageView(msg))
	}

	writeJSON(w, resp)
}

func (s *Server) handlePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Content == "" {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}

	msg, err := protocol.NewMessage(req.Content)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid message: %v", err), http.StatusBadRequest)
		return
	}

	nonce, err := protocol.MinePoW(msg.H, protocol.Difficulty)
	if err != nil {
		http.Error(w, "mining failed", http.StatusInternalServerError)
		return
	}
	msg.P = nonce

	if err := node.SendMessage(s.cfg, s.peers, "", msg); err != nil {
		http.Error(w, fmt.Sprintf("relay failed: %v", err), http.StatusBadGateway)
		return
	}

	writeJSON(w, PostResponse{
		OK:      true,
		Message: toMessageView(*msg),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	messages, err := s.store.LoadAll()
	if err != nil {
		http.Error(w, "failed to load store", http.StatusInternalServerError)
		return
	}

	peerCount := 0
	if s.peers != nil {
		peers, err := s.peers.Load()
		if err == nil {
			peerCount = len(peers)
		}
	}

	m := node.Metrics()
	snapshot := m.Snapshot()

	resp := StatusResponse{
		MessageCount:     len(messages),
		PeerCount:        peerCount,
		TorRequired:      s.cfg.RequireTorProxy,
		DevClearnet:      s.cfg.DevClearnet,
		ArchiveMode:      s.cfg.ArchiveMode,
		ListenAddress:    s.cfg.ListenAddress,
		AdvertiseAddress: s.cfg.AdvertiseAddr,
		NodeVersion:      protocol.NodeVersion,
		Metrics: MetricsSnapshot{
			MessagesAccepted:  snapshot.MessagesAccepted,
			MessagesRelayed:   snapshot.MessagesRelayed,
			SyncBatches:       snapshot.SyncBatches,
			SyncMessages:      snapshot.SyncMessages,
			SyncFailures:      snapshot.SyncFailures,
			PeerDialFailures:  snapshot.PeerDialFailures,
			PeerAbuseEvents:   snapshot.PeerAbuseEvents,
			TorRecoveries:     snapshot.TorRecoveries,
			ConnectionRejects: snapshot.ConnectionRejects,
			ResourceRejects:   snapshot.ResourceRejects,
			OpenConnections:   snapshot.OpenConnections,
		},
	}

	writeJSON(w, resp)
}

func (s *Server) handlePeers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var peers []string
	if s.peers != nil {
		loaded, err := s.peers.Load()
		if err == nil {
			peers = loaded
		}
	}
	if peers == nil {
		peers = []string{}
	}

	now := time.Now()
	views := make([]PeerView, 0, len(peers))
	for _, addr := range peers {
		view := PeerView{
			Address:   addr,
			Score:     s.peers.PenaltyScoreAt(addr, now),
			Banned:    s.peers.IsBanned(addr, now),
			BackedOff: s.peers.IsBackedOff(addr, now),
		}
		views = append(views, view)
	}

	writeJSON(w, PeersResponse{Peers: views})
}

// --- View models ---

type TimelineResponse struct {
	Messages   []MessageView `json:"messages"`
	NextOffset int           `json:"next_offset"`
	HasMore    bool          `json:"has_more"`
}

type MessageView struct {
	Content   string `json:"content"`
	Hash      string `json:"hash"`
	Timestamp int64  `json:"timestamp"`
	TimeAgo   string `json:"time_ago"`
	Nonce     uint32 `json:"nonce"`
}

type PostRequest struct {
	Content string `json:"content"`
}

type PostResponse struct {
	OK      bool        `json:"ok"`
	Message MessageView `json:"message"`
}

type StatusResponse struct {
	MessageCount     int             `json:"message_count"`
	PeerCount        int             `json:"peer_count"`
	TorRequired      bool            `json:"tor_required"`
	DevClearnet      bool            `json:"dev_clearnet"`
	ArchiveMode      bool            `json:"archive_mode"`
	ListenAddress    string          `json:"listen_address"`
	AdvertiseAddress string          `json:"advertise_address"`
	NodeVersion      string          `json:"node_version"`
	Metrics          MetricsSnapshot `json:"metrics"`
}

type MetricsSnapshot struct {
	MessagesAccepted  uint64 `json:"messages_accepted"`
	MessagesRelayed   uint64 `json:"messages_relayed"`
	SyncBatches       uint64 `json:"sync_batches"`
	SyncMessages      uint64 `json:"sync_messages"`
	SyncFailures      uint64 `json:"sync_failures"`
	PeerDialFailures  uint64 `json:"peer_dial_failures"`
	PeerAbuseEvents   uint64 `json:"peer_abuse_events"`
	TorRecoveries     uint64 `json:"tor_recoveries"`
	ConnectionRejects uint64 `json:"connection_rejects"`
	ResourceRejects   uint64 `json:"resource_rejects"`
	OpenConnections   int64  `json:"open_connections"`
}

type PeerView struct {
	Address   string `json:"address"`
	Score     int    `json:"score"`
	Banned    bool   `json:"banned"`
	BackedOff bool   `json:"backed_off"`
}

type PeersResponse struct {
	Peers []PeerView `json:"peers"`
}

func toMessageView(msg protocol.Message) MessageView {
	return MessageView{
		Content:   msg.C,
		Hash:      msg.H,
		Timestamp: int64(msg.T),
		TimeAgo:   timeAgo(int64(msg.T)),
		Nonce:     msg.P,
	}
}

func timeAgo(unixSec int64) string {
	diff := time.Since(time.Unix(unixSec, 0))
	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		m := int(diff.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case diff < 24*time.Hour:
		h := int(diff.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		d := int(diff.Hours() / 24)
		if d == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", d)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

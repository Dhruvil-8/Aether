package node

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type RuntimeMetrics struct {
	messagesAccepted  atomic.Uint64
	messagesRelayed   atomic.Uint64
	syncBatches       atomic.Uint64
	syncMessages      atomic.Uint64
	syncFailures      atomic.Uint64
	peerDialFailures  atomic.Uint64
	peerAbuseEvents   atomic.Uint64
	torRecoveries     atomic.Uint64
	connectionRejects atomic.Uint64
	resourceRejects   atomic.Uint64
	openConnections   atomic.Int64
}

var runtimeMetrics = &RuntimeMetrics{}

func Metrics() *RuntimeMetrics {
	return runtimeMetrics
}

func (m *RuntimeMetrics) IncMessagesAccepted() {
	m.messagesAccepted.Add(1)
}

func (m *RuntimeMetrics) IncMessagesRelayed(n int) {
	if n <= 0 {
		return
	}
	m.messagesRelayed.Add(uint64(n))
}

func (m *RuntimeMetrics) IncSyncBatch(applied int) {
	m.syncBatches.Add(1)
	if applied > 0 {
		m.syncMessages.Add(uint64(applied))
	}
}

func (m *RuntimeMetrics) IncSyncFailure() {
	m.syncFailures.Add(1)
}

func (m *RuntimeMetrics) IncPeerDialFailure() {
	m.peerDialFailures.Add(1)
}

func (m *RuntimeMetrics) IncPeerAbuse() {
	m.peerAbuseEvents.Add(1)
}

func (m *RuntimeMetrics) IncTorRecovery() {
	m.torRecoveries.Add(1)
}

func (m *RuntimeMetrics) IncConnectionReject() {
	m.connectionRejects.Add(1)
}

func (m *RuntimeMetrics) IncResourceReject() {
	m.resourceRejects.Add(1)
}

func (m *RuntimeMetrics) SetOpenConnections(count int) {
	m.openConnections.Store(int64(count))
}

type RuntimeMetricsSnapshot struct {
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

func (m *RuntimeMetrics) Snapshot() RuntimeMetricsSnapshot {
	return RuntimeMetricsSnapshot{
		MessagesAccepted:  m.messagesAccepted.Load(),
		MessagesRelayed:   m.messagesRelayed.Load(),
		SyncBatches:       m.syncBatches.Load(),
		SyncMessages:      m.syncMessages.Load(),
		SyncFailures:      m.syncFailures.Load(),
		PeerDialFailures:  m.peerDialFailures.Load(),
		PeerAbuseEvents:   m.peerAbuseEvents.Load(),
		TorRecoveries:     m.torRecoveries.Load(),
		ConnectionRejects: m.connectionRejects.Load(),
		ResourceRejects:   m.resourceRejects.Load(),
		OpenConnections:   m.openConnections.Load(),
	}
}

func (m *RuntimeMetrics) RenderPrometheus() string {
	var b strings.Builder
	writeMetric := func(name string, value uint64) {
		b.WriteString(name)
		b.WriteByte(' ')
		b.WriteString(fmt.Sprintf("%d", value))
		b.WriteByte('\n')
	}

	writeMetric("aether_messages_accepted_total", m.messagesAccepted.Load())
	writeMetric("aether_messages_relayed_total", m.messagesRelayed.Load())
	writeMetric("aether_sync_batches_total", m.syncBatches.Load())
	writeMetric("aether_sync_messages_applied_total", m.syncMessages.Load())
	writeMetric("aether_sync_failures_total", m.syncFailures.Load())
	writeMetric("aether_peer_dial_failures_total", m.peerDialFailures.Load())
	writeMetric("aether_peer_abuse_events_total", m.peerAbuseEvents.Load())
	writeMetric("aether_tor_recoveries_total", m.torRecoveries.Load())
	writeMetric("aether_connection_rejects_total", m.connectionRejects.Load())
	writeMetric("aether_resource_rejects_total", m.resourceRejects.Load())
	b.WriteString("aether_open_connections ")
	b.WriteString(fmt.Sprintf("%d", m.openConnections.Load()))
	b.WriteByte('\n')
	return b.String()
}

func RunMetricsServer(ctx context.Context, cfg Config, logger *Logger) error {
	if !cfg.MetricsEnabled {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc(cfg.MetricsPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(Metrics().RenderPrometheus()))
	})

	srv := &http.Server{
		Addr:              cfg.MetricsListen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	ln, err := net.Listen("tcp", cfg.MetricsListen)
	if err != nil {
		return err
	}
	logger.Infof("metrics", "metrics server listening on %s%s", cfg.MetricsListen, cfg.MetricsPath)
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			logger.Warnf("metrics", "metrics server stopped: %v", err)
		}
	}()
	return nil
}

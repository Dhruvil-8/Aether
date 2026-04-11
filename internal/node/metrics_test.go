package node

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRuntimeMetricsRenderPrometheus(t *testing.T) {
	m := &RuntimeMetrics{}
	m.IncMessagesAccepted()
	m.IncMessagesRelayed(2)
	m.IncSyncBatch(3)
	m.IncSyncFailure()
	m.IncPeerDialFailure()
	m.IncPeerAbuse()
	m.IncTorRecovery()

	out := m.RenderPrometheus()
	for _, needle := range []string{
		"aether_messages_accepted_total 1",
		"aether_messages_relayed_total 2",
		"aether_sync_batches_total 1",
		"aether_sync_messages_applied_total 3",
		"aether_sync_failures_total 1",
		"aether_peer_dial_failures_total 1",
		"aether_peer_abuse_events_total 1",
		"aether_tor_recoveries_total 1",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("expected metrics output to contain %q, got:\n%s", needle, out)
		}
	}
}

func TestRunMetricsServer(t *testing.T) {
	cfg := Config{
		MetricsEnabled: true,
		MetricsListen:  nextListenAddress(t),
		MetricsPath:    "/metrics",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := RunMetricsServer(ctx, cfg, NewLogger("error")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://" + cfg.MetricsListen + cfg.MetricsPath)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected metrics status code: %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "aether_messages_accepted_total") {
		t.Fatalf("unexpected metrics body: %s", string(body))
	}
}

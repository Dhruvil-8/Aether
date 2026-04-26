package node

import (
	"os"
	"testing"
	"time"
)

func TestEnvSecondsFallback(t *testing.T) {
	got := envSeconds("AETHER_TEST_MISSING_SECONDS", 7)
	if got != 7*time.Second {
		t.Fatalf("unexpected fallback duration: %s", got)
	}
}

func TestPortFromAddress(t *testing.T) {
	got := portFromAddress("127.0.0.1:9000")
	if got != 9000 {
		t.Fatalf("unexpected port: %d", got)
	}
}

func TestDefaultConfigRateLimitFallback(t *testing.T) {
	original := os.Getenv("AETHER_MAX_MSG_PER_SEC")
	t.Cleanup(func() {
		if original == "" {
			_ = os.Unsetenv("AETHER_MAX_MSG_PER_SEC")
			return
		}
		_ = os.Setenv("AETHER_MAX_MSG_PER_SEC", original)
	})

	_ = os.Setenv("AETHER_MAX_MSG_PER_SEC", "0")
	cfg := DefaultConfig()
	if cfg.MaxMessagesPerSec != defaultMaxMessagesPerSecond {
		t.Fatalf("unexpected fallback rate limit: %d", cfg.MaxMessagesPerSec)
	}
}

func TestDefaultConfigTargetPeersFallback(t *testing.T) {
	original := os.Getenv("AETHER_TARGET_PEERS")
	t.Cleanup(func() {
		if original == "" {
			_ = os.Unsetenv("AETHER_TARGET_PEERS")
			return
		}
		_ = os.Setenv("AETHER_TARGET_PEERS", original)
	})

	_ = os.Setenv("AETHER_TARGET_PEERS", "0")
	cfg := DefaultConfig()
	if cfg.TargetPeerCount != 8 {
		t.Fatalf("unexpected target peer fallback: %d", cfg.TargetPeerCount)
	}
}

func TestDefaultConfigSyncResponseFallback(t *testing.T) {
	original := os.Getenv("AETHER_MAX_SYNC_RESPONSE_MSGS")
	t.Cleanup(func() {
		if original == "" {
			_ = os.Unsetenv("AETHER_MAX_SYNC_RESPONSE_MSGS")
			return
		}
		_ = os.Setenv("AETHER_MAX_SYNC_RESPONSE_MSGS", original)
	})

	_ = os.Setenv("AETHER_MAX_SYNC_RESPONSE_MSGS", "0")
	cfg := DefaultConfig()
	if cfg.MaxSyncResponseMsgs != 256 {
		t.Fatalf("unexpected sync response fallback: %d", cfg.MaxSyncResponseMsgs)
	}
}

func TestDefaultConfigSyncMetadataFallbacks(t *testing.T) {
	originalWindow := os.Getenv("AETHER_SYNC_WINDOW_SIZE")
	originalChunk := os.Getenv("AETHER_SYNC_CHUNK_SIZE")
	originalMetaOffsets := os.Getenv("AETHER_MAX_SYNC_META_OFFSETS")
	t.Cleanup(func() {
		if originalWindow == "" {
			_ = os.Unsetenv("AETHER_SYNC_WINDOW_SIZE")
		} else {
			_ = os.Setenv("AETHER_SYNC_WINDOW_SIZE", originalWindow)
		}
		if originalChunk == "" {
			_ = os.Unsetenv("AETHER_SYNC_CHUNK_SIZE")
		} else {
			_ = os.Setenv("AETHER_SYNC_CHUNK_SIZE", originalChunk)
		}
		if originalMetaOffsets == "" {
			_ = os.Unsetenv("AETHER_MAX_SYNC_META_OFFSETS")
		} else {
			_ = os.Setenv("AETHER_MAX_SYNC_META_OFFSETS", originalMetaOffsets)
		}
	})

	_ = os.Setenv("AETHER_SYNC_WINDOW_SIZE", "0")
	_ = os.Setenv("AETHER_SYNC_CHUNK_SIZE", "0")
	_ = os.Setenv("AETHER_MAX_SYNC_META_OFFSETS", "0")
	cfg := DefaultConfig()
	if cfg.SyncWindowSize != 32 {
		t.Fatalf("unexpected sync window fallback: %d", cfg.SyncWindowSize)
	}
	if cfg.SyncChunkSize != 256 {
		t.Fatalf("unexpected sync chunk fallback: %d", cfg.SyncChunkSize)
	}
	if cfg.MaxSyncMetaOffsets != 128 {
		t.Fatalf("unexpected sync meta offset fallback: %d", cfg.MaxSyncMetaOffsets)
	}
}

func TestDefaultConfigPeerExchangeFallbacks(t *testing.T) {
	originalPeers := os.Getenv("AETHER_MAX_PEERS_PER_RESPONSE")
	originalAnnouncements := os.Getenv("AETHER_MAX_PEER_ANNOUNCEMENTS")
	t.Cleanup(func() {
		if originalPeers == "" {
			_ = os.Unsetenv("AETHER_MAX_PEERS_PER_RESPONSE")
		} else {
			_ = os.Setenv("AETHER_MAX_PEERS_PER_RESPONSE", originalPeers)
		}
		if originalAnnouncements == "" {
			_ = os.Unsetenv("AETHER_MAX_PEER_ANNOUNCEMENTS")
		} else {
			_ = os.Setenv("AETHER_MAX_PEER_ANNOUNCEMENTS", originalAnnouncements)
		}
	})

	_ = os.Setenv("AETHER_MAX_PEERS_PER_RESPONSE", "0")
	_ = os.Setenv("AETHER_MAX_PEER_ANNOUNCEMENTS", "0")
	cfg := DefaultConfig()
	if cfg.MaxPeersPerResponse != 64 {
		t.Fatalf("unexpected max peers per response fallback: %d", cfg.MaxPeersPerResponse)
	}
	if cfg.MaxPeerAnnouncements != 128 {
		t.Fatalf("unexpected max peer announcements fallback: %d", cfg.MaxPeerAnnouncements)
	}
}

func TestDefaultConfigConnectionAndControlFallbacks(t *testing.T) {
	originalConns := os.Getenv("AETHER_MAX_OPEN_CONNECTIONS")
	originalControl := os.Getenv("AETHER_MAX_CONTROL_PER_SEC")
	t.Cleanup(func() {
		if originalConns == "" {
			_ = os.Unsetenv("AETHER_MAX_OPEN_CONNECTIONS")
		} else {
			_ = os.Setenv("AETHER_MAX_OPEN_CONNECTIONS", originalConns)
		}
		if originalControl == "" {
			_ = os.Unsetenv("AETHER_MAX_CONTROL_PER_SEC")
		} else {
			_ = os.Setenv("AETHER_MAX_CONTROL_PER_SEC", originalControl)
		}
	})

	_ = os.Setenv("AETHER_MAX_OPEN_CONNECTIONS", "0")
	_ = os.Setenv("AETHER_MAX_CONTROL_PER_SEC", "0")
	cfg := DefaultConfig()
	if cfg.MaxOpenConnections != 32 {
		t.Fatalf("unexpected max open connections fallback: %d", cfg.MaxOpenConnections)
	}
	if cfg.MaxControlPerSec != 30 {
		t.Fatalf("unexpected max control per second fallback: %d", cfg.MaxControlPerSec)
	}
}

func TestDefaultConfigPayloadAndIdleFallbacks(t *testing.T) {
	originalPayload := os.Getenv("AETHER_MAX_JSON_PAYLOAD_BYTES")
	originalConnBytes := os.Getenv("AETHER_MAX_CONN_BYTES")
	originalGlobalRead := os.Getenv("AETHER_MAX_GLOBAL_READ_PER_SEC")
	originalFramesPerConn := os.Getenv("AETHER_MAX_FRAMES_PER_CONN")
	originalSyncResponseBytes := os.Getenv("AETHER_MAX_SYNC_RESPONSE_BYTES")
	originalIdle := os.Getenv("AETHER_CONN_IDLE_TIMEOUT_SEC")
	t.Cleanup(func() {
		if originalPayload == "" {
			_ = os.Unsetenv("AETHER_MAX_JSON_PAYLOAD_BYTES")
		} else {
			_ = os.Setenv("AETHER_MAX_JSON_PAYLOAD_BYTES", originalPayload)
		}
		if originalConnBytes == "" {
			_ = os.Unsetenv("AETHER_MAX_CONN_BYTES")
		} else {
			_ = os.Setenv("AETHER_MAX_CONN_BYTES", originalConnBytes)
		}
		if originalGlobalRead == "" {
			_ = os.Unsetenv("AETHER_MAX_GLOBAL_READ_PER_SEC")
		} else {
			_ = os.Setenv("AETHER_MAX_GLOBAL_READ_PER_SEC", originalGlobalRead)
		}
		if originalFramesPerConn == "" {
			_ = os.Unsetenv("AETHER_MAX_FRAMES_PER_CONN")
		} else {
			_ = os.Setenv("AETHER_MAX_FRAMES_PER_CONN", originalFramesPerConn)
		}
		if originalSyncResponseBytes == "" {
			_ = os.Unsetenv("AETHER_MAX_SYNC_RESPONSE_BYTES")
		} else {
			_ = os.Setenv("AETHER_MAX_SYNC_RESPONSE_BYTES", originalSyncResponseBytes)
		}
		if originalIdle == "" {
			_ = os.Unsetenv("AETHER_CONN_IDLE_TIMEOUT_SEC")
		} else {
			_ = os.Setenv("AETHER_CONN_IDLE_TIMEOUT_SEC", originalIdle)
		}
	})

	_ = os.Setenv("AETHER_MAX_JSON_PAYLOAD_BYTES", "0")
	_ = os.Setenv("AETHER_MAX_CONN_BYTES", "0")
	_ = os.Setenv("AETHER_MAX_GLOBAL_READ_PER_SEC", "0")
	_ = os.Setenv("AETHER_MAX_FRAMES_PER_CONN", "0")
	_ = os.Setenv("AETHER_MAX_SYNC_RESPONSE_BYTES", "0")
	_ = os.Setenv("AETHER_CONN_IDLE_TIMEOUT_SEC", "0")
	cfg := DefaultConfig()
	if cfg.MaxJSONPayloadBytes != 1024*1024 {
		t.Fatalf("unexpected max json payload fallback: %d", cfg.MaxJSONPayloadBytes)
	}
	if cfg.MaxConnBytes != 8*1024*1024 {
		t.Fatalf("unexpected max conn bytes fallback: %d", cfg.MaxConnBytes)
	}
	if cfg.MaxGlobalReadPerSec != 32*1024*1024 {
		t.Fatalf("unexpected global read fallback: %d", cfg.MaxGlobalReadPerSec)
	}
	if cfg.MaxFramesPerConn != 4000 {
		t.Fatalf("unexpected max frames per conn fallback: %d", cfg.MaxFramesPerConn)
	}
	if cfg.MaxSyncResponseBytes != 2*1024*1024 {
		t.Fatalf("unexpected max sync response bytes fallback: %d", cfg.MaxSyncResponseBytes)
	}
	if cfg.ConnectionIdleTimeout != 90*time.Second {
		t.Fatalf("unexpected connection idle timeout fallback: %s", cfg.ConnectionIdleTimeout)
	}
}

func TestDefaultConfigRelayHistoryWindowFallback(t *testing.T) {
	original := os.Getenv("AETHER_RELAY_HISTORY_WINDOW")
	t.Cleanup(func() {
		if original == "" {
			_ = os.Unsetenv("AETHER_RELAY_HISTORY_WINDOW")
			return
		}
		_ = os.Setenv("AETHER_RELAY_HISTORY_WINDOW", original)
	})

	_ = os.Setenv("AETHER_RELAY_HISTORY_WINDOW", "0")
	cfg := DefaultConfig()
	if cfg.RelayHistoryWindow != 4096 {
		t.Fatalf("unexpected relay history window fallback: %d", cfg.RelayHistoryWindow)
	}
}

func TestDefaultConfigPeerBanFallbacks(t *testing.T) {
	originalThreshold := os.Getenv("AETHER_PEER_BAN_THRESHOLD")
	originalDuration := os.Getenv("AETHER_PEER_BAN_DURATION_SEC")
	t.Cleanup(func() {
		if originalThreshold == "" {
			_ = os.Unsetenv("AETHER_PEER_BAN_THRESHOLD")
		} else {
			_ = os.Setenv("AETHER_PEER_BAN_THRESHOLD", originalThreshold)
		}
		if originalDuration == "" {
			_ = os.Unsetenv("AETHER_PEER_BAN_DURATION_SEC")
		} else {
			_ = os.Setenv("AETHER_PEER_BAN_DURATION_SEC", originalDuration)
		}
	})

	_ = os.Setenv("AETHER_PEER_BAN_THRESHOLD", "0")
	_ = os.Setenv("AETHER_PEER_BAN_DURATION_SEC", "0")
	cfg := DefaultConfig()
	if cfg.PeerBanThreshold != 8 {
		t.Fatalf("unexpected peer ban threshold fallback: %d", cfg.PeerBanThreshold)
	}
	if cfg.PeerBanDuration != 15*time.Minute {
		t.Fatalf("unexpected peer ban duration fallback: %s", cfg.PeerBanDuration)
	}
}

func TestDefaultConfigPeerReputationFallbacks(t *testing.T) {
	originalDecay := os.Getenv("AETHER_PEER_SCORE_DECAY_SEC")
	originalEviction := os.Getenv("AETHER_PEER_EVICTION_SCORE")
	originalCooldown := os.Getenv("AETHER_PEER_EVICTION_COOLDOWN_SEC")
	t.Cleanup(func() {
		if originalDecay == "" {
			_ = os.Unsetenv("AETHER_PEER_SCORE_DECAY_SEC")
		} else {
			_ = os.Setenv("AETHER_PEER_SCORE_DECAY_SEC", originalDecay)
		}
		if originalEviction == "" {
			_ = os.Unsetenv("AETHER_PEER_EVICTION_SCORE")
		} else {
			_ = os.Setenv("AETHER_PEER_EVICTION_SCORE", originalEviction)
		}
		if originalCooldown == "" {
			_ = os.Unsetenv("AETHER_PEER_EVICTION_COOLDOWN_SEC")
		} else {
			_ = os.Setenv("AETHER_PEER_EVICTION_COOLDOWN_SEC", originalCooldown)
		}
	})

	_ = os.Setenv("AETHER_PEER_SCORE_DECAY_SEC", "0")
	_ = os.Setenv("AETHER_PEER_EVICTION_SCORE", "0")
	_ = os.Setenv("AETHER_PEER_EVICTION_COOLDOWN_SEC", "0")
	cfg := DefaultConfig()
	if cfg.PeerScoreDecay != time.Hour {
		t.Fatalf("unexpected peer score decay fallback: %s", cfg.PeerScoreDecay)
	}
	if cfg.PeerEvictionScore != 16 {
		t.Fatalf("unexpected peer eviction score fallback: %d", cfg.PeerEvictionScore)
	}
	if cfg.PeerEvictionCooldown != 30*time.Minute {
		t.Fatalf("unexpected peer eviction cooldown fallback: %s", cfg.PeerEvictionCooldown)
	}
}

func TestDefaultConfigTorDialTimeoutFallback(t *testing.T) {
	original := os.Getenv("AETHER_TOR_DIAL_TIMEOUT_SEC")
	t.Cleanup(func() {
		if original == "" {
			_ = os.Unsetenv("AETHER_TOR_DIAL_TIMEOUT_SEC")
			return
		}
		_ = os.Setenv("AETHER_TOR_DIAL_TIMEOUT_SEC", original)
	})

	_ = os.Setenv("AETHER_TOR_DIAL_TIMEOUT_SEC", "0")
	cfg := DefaultConfig()
	if cfg.TorDialTimeout != 5*time.Second {
		t.Fatalf("unexpected tor dial timeout fallback: %s", cfg.TorDialTimeout)
	}
}

func TestDefaultConfigManagedTorFallbacks(t *testing.T) {
	originalManage := os.Getenv("AETHER_MANAGE_TOR")
	originalBinary := os.Getenv("AETHER_TOR_BINARY")
	originalStartup := os.Getenv("AETHER_TOR_STARTUP_TIMEOUT_SEC")
	originalPass := os.Getenv("AETHER_TOR_CONTROL_PASS")
	t.Cleanup(func() {
		if originalManage == "" {
			_ = os.Unsetenv("AETHER_MANAGE_TOR")
		} else {
			_ = os.Setenv("AETHER_MANAGE_TOR", originalManage)
		}
		if originalBinary == "" {
			_ = os.Unsetenv("AETHER_TOR_BINARY")
		} else {
			_ = os.Setenv("AETHER_TOR_BINARY", originalBinary)
		}
		if originalStartup == "" {
			_ = os.Unsetenv("AETHER_TOR_STARTUP_TIMEOUT_SEC")
		} else {
			_ = os.Setenv("AETHER_TOR_STARTUP_TIMEOUT_SEC", originalStartup)
		}
		if originalPass == "" {
			_ = os.Unsetenv("AETHER_TOR_CONTROL_PASS")
		} else {
			_ = os.Setenv("AETHER_TOR_CONTROL_PASS", originalPass)
		}
	})

	_ = os.Setenv("AETHER_MANAGE_TOR", "true")
	_ = os.Setenv("AETHER_TOR_BINARY", "")
	_ = os.Setenv("AETHER_TOR_STARTUP_TIMEOUT_SEC", "0")
	_ = os.Setenv("AETHER_TOR_CONTROL_PASS", "should-clear")
	cfg := DefaultConfig()
	if !cfg.ManageTor {
		t.Fatal("expected managed tor mode enabled")
	}
	if cfg.TorBinary != "tor" {
		t.Fatalf("unexpected tor binary fallback: %s", cfg.TorBinary)
	}
	if cfg.TorStartupTimeout != 30*time.Second {
		t.Fatalf("unexpected tor startup timeout fallback: %s", cfg.TorStartupTimeout)
	}
	if cfg.TorControlPass != "" {
		t.Fatalf("expected managed tor mode to clear control password, got %q", cfg.TorControlPass)
	}
}

func TestDefaultConfigTorHealthIntervalFallback(t *testing.T) {
	original := os.Getenv("AETHER_TOR_HEALTH_INTERVAL_SEC")
	t.Cleanup(func() {
		if original == "" {
			_ = os.Unsetenv("AETHER_TOR_HEALTH_INTERVAL_SEC")
			return
		}
		_ = os.Setenv("AETHER_TOR_HEALTH_INTERVAL_SEC", original)
	})

	_ = os.Setenv("AETHER_TOR_HEALTH_INTERVAL_SEC", "0")
	cfg := DefaultConfig()
	if cfg.TorHealthInterval != 30*time.Second {
		t.Fatalf("unexpected tor health interval fallback: %s", cfg.TorHealthInterval)
	}
}

func TestDefaultConfigLogLevelFallback(t *testing.T) {
	original := os.Getenv("AETHER_LOG_LEVEL")
	t.Cleanup(func() {
		if original == "" {
			_ = os.Unsetenv("AETHER_LOG_LEVEL")
			return
		}
		_ = os.Setenv("AETHER_LOG_LEVEL", original)
	})

	_ = os.Setenv("AETHER_LOG_LEVEL", "")
	cfg := DefaultConfig()
	if cfg.LogLevel != "info" {
		t.Fatalf("unexpected log level fallback: %s", cfg.LogLevel)
	}
}

func TestDefaultConfigMetricsFallbacks(t *testing.T) {
	originalEnabled := os.Getenv("AETHER_METRICS_ENABLED")
	originalListen := os.Getenv("AETHER_METRICS_LISTEN")
	originalPath := os.Getenv("AETHER_METRICS_PATH")
	t.Cleanup(func() {
		if originalEnabled == "" {
			_ = os.Unsetenv("AETHER_METRICS_ENABLED")
		} else {
			_ = os.Setenv("AETHER_METRICS_ENABLED", originalEnabled)
		}
		if originalListen == "" {
			_ = os.Unsetenv("AETHER_METRICS_LISTEN")
		} else {
			_ = os.Setenv("AETHER_METRICS_LISTEN", originalListen)
		}
		if originalPath == "" {
			_ = os.Unsetenv("AETHER_METRICS_PATH")
		} else {
			_ = os.Setenv("AETHER_METRICS_PATH", originalPath)
		}
	})

	_ = os.Setenv("AETHER_METRICS_ENABLED", "true")
	_ = os.Setenv("AETHER_METRICS_LISTEN", "")
	_ = os.Setenv("AETHER_METRICS_PATH", "metrics")

	cfg := DefaultConfig()
	if !cfg.MetricsEnabled {
		t.Fatal("expected metrics enabled")
	}
	if cfg.MetricsListen != "127.0.0.1:9100" {
		t.Fatalf("unexpected metrics listen fallback: %s", cfg.MetricsListen)
	}
	if cfg.MetricsPath != "/metrics" {
		t.Fatalf("unexpected metrics path normalization: %s", cfg.MetricsPath)
	}
}

func TestDefaultConfigSyncTrustQuorumFallback(t *testing.T) {
	original := os.Getenv("AETHER_SYNC_TRUST_QUORUM")
	t.Cleanup(func() {
		if original == "" {
			_ = os.Unsetenv("AETHER_SYNC_TRUST_QUORUM")
			return
		}
		_ = os.Setenv("AETHER_SYNC_TRUST_QUORUM", original)
	})

	_ = os.Setenv("AETHER_SYNC_TRUST_QUORUM", "0")
	cfg := DefaultConfig()
	if cfg.SyncTrustQuorum != 1 {
		t.Fatalf("unexpected sync trust quorum fallback: %d", cfg.SyncTrustQuorum)
	}

	_ = os.Setenv("AETHER_SYNC_TRUST_QUORUM", "2")
	cfg = DefaultConfig()
	if cfg.SyncTrustQuorum != 2 {
		t.Fatalf("unexpected sync trust quorum: %d", cfg.SyncTrustQuorum)
	}
}

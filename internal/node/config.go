package node

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const DefaultTorSOCKSAddress = "127.0.0.1:9050"
const DefaultTorControlAddress = "127.0.0.1:9051"

type Config struct {
	ListenAddress         string
	AdvertiseAddr         string
	DataDir               string
	LogLevel              string
	MetricsEnabled        bool
	MetricsListen         string
	MetricsPath           string
	ManageTor             bool
	TorBinary             string
	TorStartupTimeout     time.Duration
	TorSOCKS              string
	TorControl            string
	TorControlPass        string
	BootstrapPeers        []string
	OnionVirtualPort      int
	BootstrapInterval     time.Duration
	PeerExchangeInterval  time.Duration
	SyncInterval          time.Duration
	SyncBatchSize         int
	SyncWindowSize        int
	MaxSyncResponseMsgs   int
	MaxSyncMetaOffsets    int
	TargetPeerCount       int
	MaxOpenConnections    int
	MaxPeersPerResponse   int
	MaxPeerAnnouncements  int
	PeerBackoff           time.Duration
	PeerScoreDecay        time.Duration
	PeerBanThreshold      int
	PeerBanDuration       time.Duration
	PeerEvictionScore     int
	PeerEvictionCooldown  time.Duration
	MaxMessagesPerSec     int
	MaxControlPerSec      int
	MaxJSONPayloadBytes   int
	ConnectionIdleTimeout time.Duration
	TorDialTimeout        time.Duration
	TorHealthInterval     time.Duration
	ArchiveMode           bool
	RelayHistoryWindow    int
	DevClearnet           bool
	RequireTorProxy       bool
}

func DefaultConfig() Config {
	cfg := Config{
		ListenAddress:         envOrDefault("AETHER_LISTEN_ADDR", "127.0.0.1:9000"),
		AdvertiseAddr:         strings.TrimSpace(os.Getenv("AETHER_ADVERTISE_ADDR")),
		DataDir:               envOrDefault("AETHER_DATA_DIR", ".aether"),
		LogLevel:              envOrDefault("AETHER_LOG_LEVEL", "info"),
		MetricsEnabled:        envBool("AETHER_METRICS_ENABLED", false),
		MetricsListen:         envOrDefault("AETHER_METRICS_LISTEN", "127.0.0.1:9100"),
		MetricsPath:           envOrDefault("AETHER_METRICS_PATH", "/metrics"),
		ManageTor:             envBool("AETHER_MANAGE_TOR", false),
		TorBinary:             envOrDefault("AETHER_TOR_BINARY", "tor"),
		TorStartupTimeout:     envSeconds("AETHER_TOR_STARTUP_TIMEOUT_SEC", 30),
		TorSOCKS:              envOrDefault("AETHER_TOR_SOCKS", DefaultTorSOCKSAddress),
		TorControl:            envOrDefault("AETHER_TOR_CONTROL", DefaultTorControlAddress),
		TorControlPass:        os.Getenv("AETHER_TOR_CONTROL_PASS"),
		BootstrapPeers:        splitCSV(os.Getenv("AETHER_BOOTSTRAP_PEERS")),
		OnionVirtualPort:      envInt("AETHER_ONION_PORT", 0),
		BootstrapInterval:     envSeconds("AETHER_BOOTSTRAP_INTERVAL_SEC", 300),
		PeerExchangeInterval:  envSeconds("AETHER_PEER_EXCHANGE_INTERVAL_SEC", 120),
		SyncInterval:          envSeconds("AETHER_SYNC_INTERVAL_SEC", 180),
		SyncBatchSize:         envInt("AETHER_SYNC_BATCH_SIZE", 256),
		SyncWindowSize:        envInt("AETHER_SYNC_WINDOW_SIZE", 32),
		MaxSyncResponseMsgs:   envInt("AETHER_MAX_SYNC_RESPONSE_MSGS", 256),
		MaxSyncMetaOffsets:    envInt("AETHER_MAX_SYNC_META_OFFSETS", 128),
		TargetPeerCount:       envInt("AETHER_TARGET_PEERS", 8),
		MaxOpenConnections:    envInt("AETHER_MAX_OPEN_CONNECTIONS", 32),
		MaxPeersPerResponse:   envInt("AETHER_MAX_PEERS_PER_RESPONSE", 64),
		MaxPeerAnnouncements:  envInt("AETHER_MAX_PEER_ANNOUNCEMENTS", 128),
		PeerBackoff:           envSeconds("AETHER_PEER_BACKOFF_SEC", 30),
		PeerScoreDecay:        envSeconds("AETHER_PEER_SCORE_DECAY_SEC", 3600),
		PeerBanThreshold:      envInt("AETHER_PEER_BAN_THRESHOLD", 8),
		PeerBanDuration:       envSeconds("AETHER_PEER_BAN_DURATION_SEC", 900),
		PeerEvictionScore:     envInt("AETHER_PEER_EVICTION_SCORE", 16),
		PeerEvictionCooldown:  envSeconds("AETHER_PEER_EVICTION_COOLDOWN_SEC", 1800),
		MaxMessagesPerSec:     envInt("AETHER_MAX_MSG_PER_SEC", 20),
		MaxControlPerSec:      envInt("AETHER_MAX_CONTROL_PER_SEC", 30),
		MaxJSONPayloadBytes:   envInt("AETHER_MAX_JSON_PAYLOAD_BYTES", 1048576),
		ConnectionIdleTimeout: envSeconds("AETHER_CONN_IDLE_TIMEOUT_SEC", 90),
		TorDialTimeout:        envSeconds("AETHER_TOR_DIAL_TIMEOUT_SEC", 5),
		TorHealthInterval:     envSeconds("AETHER_TOR_HEALTH_INTERVAL_SEC", 30),
		ArchiveMode:           envBool("AETHER_ARCHIVE_MODE", false),
		RelayHistoryWindow:    envInt("AETHER_RELAY_HISTORY_WINDOW", 4096),
		DevClearnet:           envBool("AETHER_DEV_CLEARNET", false),
		RequireTorProxy:       envBool("AETHER_REQUIRE_TOR", true),
	}
	if cfg.PeerBackoff <= 0 {
		cfg.PeerBackoff = 30 * time.Second
	}
	if cfg.PeerExchangeInterval <= 0 {
		cfg.PeerExchangeInterval = 120 * time.Second
	}
	if cfg.SyncBatchSize <= 0 {
		cfg.SyncBatchSize = 256
	}
	if cfg.SyncWindowSize <= 0 {
		cfg.SyncWindowSize = 32
	}
	if cfg.MaxSyncResponseMsgs <= 0 {
		cfg.MaxSyncResponseMsgs = 256
	}
	if cfg.MaxSyncMetaOffsets <= 0 {
		cfg.MaxSyncMetaOffsets = 128
	}
	if cfg.TargetPeerCount <= 0 {
		cfg.TargetPeerCount = 8
	}
	if cfg.MaxOpenConnections <= 0 {
		cfg.MaxOpenConnections = 32
	}
	if cfg.MaxPeersPerResponse <= 0 {
		cfg.MaxPeersPerResponse = 64
	}
	if cfg.MaxPeerAnnouncements <= 0 {
		cfg.MaxPeerAnnouncements = 128
	}
	if cfg.PeerScoreDecay <= 0 {
		cfg.PeerScoreDecay = time.Hour
	}
	if cfg.PeerBanThreshold <= 0 {
		cfg.PeerBanThreshold = 8
	}
	if cfg.PeerBanDuration <= 0 {
		cfg.PeerBanDuration = 15 * time.Minute
	}
	if cfg.PeerEvictionScore <= 0 {
		cfg.PeerEvictionScore = 16
	}
	if cfg.PeerEvictionCooldown <= 0 {
		cfg.PeerEvictionCooldown = 30 * time.Minute
	}
	if cfg.MaxMessagesPerSec <= 0 {
		cfg.MaxMessagesPerSec = defaultMaxMessagesPerSecond
	}
	if cfg.MaxControlPerSec <= 0 {
		cfg.MaxControlPerSec = 30
	}
	if cfg.MaxJSONPayloadBytes <= 0 {
		cfg.MaxJSONPayloadBytes = 1024 * 1024
	}
	if cfg.ConnectionIdleTimeout <= 0 {
		cfg.ConnectionIdleTimeout = 90 * time.Second
	}
	if cfg.TorDialTimeout <= 0 {
		cfg.TorDialTimeout = 5 * time.Second
	}
	if cfg.TorHealthInterval <= 0 {
		cfg.TorHealthInterval = 30 * time.Second
	}
	if cfg.TorStartupTimeout <= 0 {
		cfg.TorStartupTimeout = 30 * time.Second
	}
	if cfg.OnionVirtualPort == 0 {
		cfg.OnionVirtualPort = portFromAddress(cfg.ListenAddress)
	}
	if cfg.DevClearnet && cfg.AdvertiseAddr == "" {
		cfg.AdvertiseAddr = cfg.ListenAddress
	}
	if cfg.RelayHistoryWindow <= 0 {
		cfg.RelayHistoryWindow = 4096
	}
	if strings.TrimSpace(cfg.MetricsListen) == "" {
		cfg.MetricsListen = "127.0.0.1:9100"
	}
	if strings.TrimSpace(cfg.MetricsPath) == "" {
		cfg.MetricsPath = "/metrics"
	}
	if !strings.HasPrefix(cfg.MetricsPath, "/") {
		cfg.MetricsPath = "/" + cfg.MetricsPath
	}
	if cfg.ManageTor && strings.TrimSpace(cfg.TorControlPass) != "" {
		// Managed Tor mode uses unauthenticated local control by default.
		cfg.TorControlPass = ""
	}
	return cfg
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envSeconds(key string, fallbackSeconds int) time.Duration {
	seconds := envInt(key, fallbackSeconds)
	if seconds <= 0 {
		seconds = fallbackSeconds
	}
	return time.Duration(seconds) * time.Second
}

func splitCSV(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func portFromAddress(address string) int {
	if address == "" {
		return 0
	}
	idx := strings.LastIndex(address, ":")
	if idx < 0 || idx == len(address)-1 {
		return 0
	}
	port, err := strconv.Atoi(address[idx+1:])
	if err != nil {
		return 0
	}
	return port
}

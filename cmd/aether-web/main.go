package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"aether/internal/api"
	"aether/internal/node"
)

//go:embed web
var webAssets embed.FS

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := node.DefaultConfig()
	logger := node.NewLogger(cfg.LogLevel)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Start managed Tor if configured.
	var torSupervisor *node.TorSupervisor
	if cfg.ManageTor {
		supervisor, err := node.StartTorSupervisor(ctx, cfg, logger)
		if err != nil {
			return err
		}
		torSupervisor = supervisor
	}
	if torSupervisor != nil {
		defer torSupervisor.Close()
	}

	// Validate Tor runtime.
	if err := node.ValidateTorRuntime(cfg, true); err != nil {
		return err
	}

	// Open store and peer book.
	store, err := node.OpenStore(cfg.DataDir)
	if err != nil {
		return err
	}
	peerBook, err := node.OpenPeerBook(cfg.DataDir)
	if err != nil {
		return err
	}
	peerBook.ApplyPolicy(cfg)
	syncState, err := node.OpenSyncState(cfg.DataDir)
	if err != nil {
		return err
	}
	if len(cfg.BootstrapPeers) > 0 {
		_, _ = peerBook.Merge(cfg.BootstrapPeers...)
	}

	// Create onion service for inbound connections.
	var onionService *node.OnionService
	if cfg.RequireTorProxy && !cfg.DevClearnet {
		service, addr, err := node.EnsureOnionService(cfg)
		if err != nil {
			return err
		}
		onionService = service
		cfg.AdvertiseAddr = addr
	}

	// Start the P2P node.
	server := node.NewPeerServer(cfg, store, peerBook)
	if err := server.RestoreSeen(); err != nil {
		return err
	}
	syncer := node.NewSyncer(cfg, peerBook, store, syncState)

	var torRuntime *node.TorRuntime
	if onionService != nil {
		torRuntime = node.NewTorRuntime(cfg, onionService, logger)
		defer torRuntime.Close()
		go torRuntime.Run(ctx)
	}
	if err := node.RunMetricsServer(ctx, cfg, logger); err != nil {
		return err
	}

	go node.RunPeerConnector(ctx, cfg, server)
	go node.RunPeerExchange(ctx, cfg, server)
	go node.RunMaintenance(ctx, cfg, peerBook, syncer, node.NewGossip(), server.Seen, server.RestoreSeen)

	// Start the API + Web UI server.
	apiAddr := envOrDefault("AETHER_UI_LISTEN", "127.0.0.1:8080")
	apiServer := api.NewServer(cfg, store, peerBook, &webAssets)
	go func() {
		logger.Infof("ui", "web UI available at http://%s", apiAddr)
		if err := apiServer.ListenAndServe(ctx, apiAddr); err != nil {
			logger.Warnf("ui", "API server stopped: %v", err)
		}
	}()

	// Start P2P listener (blocks until shutdown).
	logger.Infof("serve", "aetherd listening on %s (advertise=%s, ui=%s)", cfg.ListenAddress, cfg.AdvertiseAddr, apiAddr)
	return server.Listen(ctx)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

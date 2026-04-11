package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"aether/internal/node"
	"aether/internal/protocol"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}

	cfg := node.DefaultConfig()

	switch args[0] {
	case "version":
		return runVersion()
	case "doctor":
		return runDoctor(cfg)
	case "mine":
		if len(args) < 2 {
			return errors.New("missing message content")
		}
		return runMine(args[1])
	case "post":
		if len(args) < 2 {
			return errors.New("missing message content")
		}
		target := ""
		content := args[1]
		if len(args) >= 3 {
			target = args[1]
			content = args[2]
		}
		return runPost(cfg, target, content)
	case "serve":
		return runServe(cfg)
	default:
		return usage()
	}
}

func runMine(content string) error {
	msg, err := protocol.NewMessage(content)
	if err != nil {
		return err
	}
	nonce, err := protocol.MinePoW(msg.H, protocol.Difficulty)
	if err != nil {
		return err
	}
	msg.P = nonce
	encoded, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

func runVersion() error {
	fmt.Println(protocol.NodeVersion)
	return nil
}

func runDoctor(cfg node.Config) error {
	logger := node.NewLogger(cfg.LogLevel)
	logger.Infof("doctor", "running preflight checks (dev_clearnet=%t, require_tor=%t, manage_tor=%t)", cfg.DevClearnet, cfg.RequireTorProxy, cfg.ManageTor)
	if err := node.ValidateTorRuntime(cfg, true); err != nil {
		return err
	}
	fmt.Println("ok: tor/runtime preflight passed")
	return nil
}

func runPost(cfg node.Config, target, content string) error {
	logger := node.NewLogger(cfg.LogLevel)
	if err := node.ValidateTorRuntime(cfg, false); err != nil {
		return err
	}
	peerBook, err := node.OpenPeerBook(cfg.DataDir)
	if err != nil {
		return err
	}
	peerBook.ApplyPolicy(cfg)
	_ = node.Bootstrap(cfg, peerBook)

	msg, err := protocol.NewMessage(content)
	if err != nil {
		return err
	}
	nonce, err := protocol.MinePoW(msg.H, protocol.Difficulty)
	if err != nil {
		return err
	}
	msg.P = nonce
	if err := node.SendMessage(cfg, peerBook, target, msg); err != nil {
		return err
	}
	logger.Infof("post", "message accepted by a peer (hash=%s)", msg.H)
	encoded, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

func runServe(cfg node.Config) error {
	logger := node.NewLogger(cfg.LogLevel)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

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

	if err := node.ValidateTorRuntime(cfg, true); err != nil {
		return err
	}
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

	var onionService *node.OnionService
	if cfg.RequireTorProxy && !cfg.DevClearnet {
		service, addr, err := node.EnsureOnionService(cfg)
		if err != nil {
			return err
		}
		onionService = service
		cfg.AdvertiseAddr = addr
	}

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

	logger.Infof("serve", "aetherd listening on %s (advertise=%s, dev_clearnet=%t, require_tor=%t, manage_tor=%t, tor_socks=%s, bootstrap_interval=%s, peer_exchange_interval=%s, sync_interval=%s)", cfg.ListenAddress, cfg.AdvertiseAddr, cfg.DevClearnet, cfg.RequireTorProxy, cfg.ManageTor, cfg.TorSOCKS, cfg.BootstrapInterval, cfg.PeerExchangeInterval, cfg.SyncInterval)
	return server.Listen(ctx)
}

func usage() error {
	fmt.Println("usage:")
	fmt.Println("  aetherd version")
	fmt.Println("  aetherd doctor")
	fmt.Println("  aetherd mine <message>")
	fmt.Println("  aetherd post <message>")
	fmt.Println("  aetherd post <peer> <message>")
	fmt.Println("  aetherd serve")
	return nil
}

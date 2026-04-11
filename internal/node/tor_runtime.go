package node

import (
	"context"
	"sync"
	"time"
)

type TorRuntime struct {
	cfg    Config
	logger *Logger

	mu        sync.Mutex
	service   *OnionService
	unhealthy bool

	checkSOCKS   func(string, time.Duration) error
	checkControl func(Config) error
	ensureOnion  func(Config) (*OnionService, string, error)
}

func NewTorRuntime(cfg Config, service *OnionService, logger *Logger) *TorRuntime {
	return &TorRuntime{
		cfg:          cfg,
		logger:       logger,
		service:      service,
		checkSOCKS:   CheckTorSOCKS,
		checkControl: CheckTorControl,
		ensureOnion:  EnsureManagedOnionService,
	}
}

func (t *TorRuntime) Run(ctx context.Context) {
	if t == nil || t.cfg.DevClearnet || !t.cfg.RequireTorProxy {
		return
	}

	interval := t.cfg.TorHealthInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.Step()
		}
	}
}

func (t *TorRuntime) Step() {
	if t == nil || t.cfg.DevClearnet || !t.cfg.RequireTorProxy {
		return
	}

	socksErr := t.checkSOCKS(t.cfg.TorSOCKS, t.cfg.TorDialTimeout)
	controlErr := t.checkControl(t.cfg)
	healthy := socksErr == nil && controlErr == nil

	t.mu.Lock()
	wasUnhealthy := t.unhealthy
	if !healthy {
		t.unhealthy = true
		t.mu.Unlock()
		if !wasUnhealthy {
			if socksErr != nil {
				t.logger.Warnf("tor", "tor socks health check failed: %v", socksErr)
			}
			if controlErr != nil {
				t.logger.Warnf("tor", "tor control health check failed: %v", controlErr)
			}
		}
		return
	}
	t.unhealthy = false
	t.mu.Unlock()

	if !wasUnhealthy {
		return
	}

	t.logger.Infof("tor", "tor connectivity restored, re-establishing managed onion service")
	if err := t.refreshService(); err != nil {
		t.logger.Errorf("tor", "failed to re-establish onion service: %v", err)
		return
	}
	Metrics().IncTorRecovery()
}

func (t *TorRuntime) refreshService() error {
	t.mu.Lock()
	prev := t.service
	t.service = nil
	t.mu.Unlock()

	if prev != nil {
		_ = prev.Close()
	}

	service, addr, err := t.ensureOnion(t.cfg)
	if err != nil {
		return err
	}

	t.mu.Lock()
	t.service = service
	t.mu.Unlock()

	if addr != "" {
		t.logger.Infof("tor", "managed onion service active at %s", addr)
	}
	return nil
}

func (t *TorRuntime) Close() error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	service := t.service
	t.service = nil
	t.mu.Unlock()
	if service == nil {
		return nil
	}
	return service.Close()
}

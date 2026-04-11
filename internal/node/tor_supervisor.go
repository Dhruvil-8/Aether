package node

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var ErrTorStartupTimeout = errors.New("managed tor startup timed out")

type TorSupervisor struct {
	logger *Logger
	cmd    *exec.Cmd
	done   chan error
}

var startTorCommand = func(ctx context.Context, cfg Config, args []string) *exec.Cmd {
	return exec.CommandContext(ctx, cfg.TorBinary, args...)
}

func StartTorSupervisor(ctx context.Context, cfg Config, logger *Logger) (*TorSupervisor, error) {
	if !cfg.ManageTor || cfg.DevClearnet || !cfg.RequireTorProxy {
		return nil, nil
	}

	// If Tor is already healthy at the configured addresses, do not spawn another instance.
	if err := ValidateTorRuntime(cfg, true); err == nil {
		logger.Infof("tor", "managed mode enabled; using already-running tor at socks=%s control=%s", cfg.TorSOCKS, cfg.TorControl)
		return nil, nil
	}

	runtimeDir := filepath.Join(cfg.DataDir, "tor", "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return nil, err
	}

	args := []string{
		"--SocksPort", cfg.TorSOCKS,
		"--ControlPort", cfg.TorControl,
		"--DataDirectory", runtimeDir,
		"--CookieAuthentication", "0",
		"--AvoidDiskWrites", "1",
		"--Log", "notice stdout",
	}

	cmd := startTorCommand(ctx, cfg, args)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	s := &TorSupervisor{
		logger: logger,
		cmd:    cmd,
		done:   make(chan error, 1),
	}
	go s.captureTorLogs(stdout, "stdout")
	go s.captureTorLogs(stderr, "stderr")
	go func() {
		s.done <- cmd.Wait()
		close(s.done)
	}()

	logger.Infof("tor", "started managed tor process pid=%d", cmd.Process.Pid)
	if err := s.waitHealthy(cfg); err != nil {
		_ = s.Close()
		return nil, err
	}
	logger.Infof("tor", "managed tor is healthy at socks=%s control=%s", cfg.TorSOCKS, cfg.TorControl)
	return s, nil
}

func (s *TorSupervisor) waitHealthy(cfg Config) error {
	timeout := cfg.TorStartupTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if err := ValidateTorRuntime(cfg, true); err == nil {
			return nil
		}

		select {
		case err := <-s.done:
			if err == nil {
				return errors.New("managed tor exited before becoming healthy")
			}
			return fmt.Errorf("managed tor exited before becoming healthy: %w", err)
		default:
		}
		time.Sleep(300 * time.Millisecond)
	}
	return ErrTorStartupTimeout
}

func (s *TorSupervisor) captureTorLogs(r io.Reader, stream string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		s.logger.Debugf("tor-process", "%s: %s", stream, line)
	}
	if err := scanner.Err(); err != nil {
		s.logger.Warnf("tor-process", "log stream %s error: %v", stream, err)
	}
}

func (s *TorSupervisor) Close() error {
	if s == nil || s.cmd == nil || s.cmd.Process == nil {
		return nil
	}

	_ = s.cmd.Process.Signal(os.Interrupt)
	select {
	case <-s.done:
		return nil
	case <-time.After(3 * time.Second):
		_ = s.cmd.Process.Kill()
	}

	select {
	case <-s.done:
		return nil
	case <-time.After(2 * time.Second):
	}

	select {
	case <-s.done:
		return nil
	default:
		return nil
	}
}

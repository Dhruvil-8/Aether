package node

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrTorControlRejected    = errors.New("tor control command rejected")
	ErrOnionServiceNotFound  = errors.New("tor control did not return onion service id")
	ErrTorControlUnreachable = errors.New("tor control port unreachable")
)

type OnionService struct {
	Address string
	conn    net.Conn
	id      string
}

func (s *OnionService) Close() error {
	if s == nil || s.conn == nil {
		return nil
	}
	_, _ = fmt.Fprintf(s.conn, "DEL_ONION %s\r\n", s.id)
	_ = drainTorControlResponse(bufio.NewReader(s.conn))
	err := s.conn.Close()
	s.conn = nil
	return err
}

func EnsureOnionService(cfg Config) (*OnionService, string, error) {
	return ensureOnionService(cfg, false)
}

func EnsureManagedOnionService(cfg Config) (*OnionService, string, error) {
	return ensureOnionService(cfg, true)
}

func ensureOnionService(cfg Config, forceManage bool) (*OnionService, string, error) {
	if cfg.DevClearnet {
		return nil, advertisedAddress(cfg), nil
	}
	if !forceManage && isOnionAddress(cfg.AdvertiseAddr) {
		return nil, cfg.AdvertiseAddr, nil
	}

	listenTarget, err := onionTarget(cfg.ListenAddress)
	if err != nil {
		return nil, "", err
	}

	keyPath := filepath.Join(cfg.DataDir, "tor", "onion.key")
	addressPath := filepath.Join(cfg.DataDir, "tor", "onion.addr")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return nil, "", err
	}
	keyMaterial, _ := os.ReadFile(keyPath)

	conn, err := dialTorControl(cfg.TorControl, cfg.TorDialTimeout)
	if err != nil {
		return nil, "", err
	}
	reader := bufio.NewReader(conn)

	if err := torAuthenticate(conn, reader, cfg.TorControlPass); err != nil {
		_ = conn.Close()
		return nil, "", err
	}

	service, privateKey, err := torAddOnion(conn, reader, strings.TrimSpace(string(keyMaterial)), cfg.OnionVirtualPort, listenTarget)
	if err != nil {
		_ = conn.Close()
		return nil, "", err
	}

	if privateKey != "" {
		if err := os.WriteFile(keyPath, []byte(privateKey), 0o600); err != nil {
			_ = service.Close()
			return nil, "", err
		}
	}
	if err := os.WriteFile(addressPath, []byte(service.Address), 0o600); err != nil {
		_ = service.Close()
		return nil, "", err
	}
	return service, service.Address, nil
}

func CheckTorControl(cfg Config) error {
	conn, err := dialTorControl(cfg.TorControl, cfg.TorDialTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	return torAuthenticate(conn, bufio.NewReader(conn), cfg.TorControlPass)
}

func ValidateTorRuntime(cfg Config, needControl bool) error {
	if cfg.DevClearnet || !cfg.RequireTorProxy {
		return nil
	}
	if err := CheckTorSOCKS(cfg.TorSOCKS, cfg.TorDialTimeout); err != nil {
		return fmt.Errorf("tor socks preflight failed: %w", err)
	}
	if needControl {
		if err := CheckTorControl(cfg); err != nil {
			return fmt.Errorf("tor control preflight failed: %w", err)
		}
	}
	return nil
}

func torAuthenticate(conn net.Conn, reader *bufio.Reader, password string) error {
	command := "AUTHENTICATE"
	if password != "" {
		command += fmt.Sprintf(" \"%s\"", escapeTorControlString(password))
	}
	if _, err := fmt.Fprintf(conn, "%s\r\n", command); err != nil {
		return err
	}
	return drainTorControlResponse(reader)
}

func torAddOnion(conn net.Conn, reader *bufio.Reader, keyMaterial string, virtualPort int, target string) (*OnionService, string, error) {
	keySpec := "NEW:ED25519-V3"
	if keyMaterial != "" {
		keySpec = keyMaterial
	}

	command := fmt.Sprintf("ADD_ONION %s Port=%d,%s", keySpec, virtualPort, target)
	if _, err := fmt.Fprintf(conn, "%s\r\n", command); err != nil {
		return nil, "", err
	}

	lines, err := readTorControlResponse(reader)
	if err != nil {
		return nil, "", err
	}

	var serviceID string
	var privateKey string
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "ServiceID="):
			serviceID = strings.TrimPrefix(line, "ServiceID=")
		case strings.HasPrefix(line, "PrivateKey="):
			privateKey = strings.TrimPrefix(line, "PrivateKey=")
		}
	}
	if serviceID == "" {
		return nil, "", ErrOnionServiceNotFound
	}

	address := fmt.Sprintf("%s.onion:%d", serviceID, virtualPort)
	return &OnionService{
		Address: address,
		conn:    conn,
		id:      serviceID,
	}, privateKey, nil
}

func readTorControlResponse(reader *bufio.Reader) ([]string, error) {
	var lines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if len(line) < 4 {
			return nil, ErrTorControlRejected
		}
		code := line[:3]
		sep := line[3]
		payload := ""
		if len(line) > 4 {
			payload = line[4:]
		}
		if code != "250" {
			return nil, fmt.Errorf("%w: %s", ErrTorControlRejected, line)
		}
		if payload != "OK" && payload != "" {
			lines = append(lines, payload)
		}
		if sep == ' ' {
			return lines, nil
		}
		if sep != '-' {
			return nil, ErrTorControlRejected
		}
	}
}

func drainTorControlResponse(reader *bufio.Reader) error {
	_, err := readTorControlResponse(reader)
	return err
}

func onionTarget(listenAddress string) (string, error) {
	host, port, err := net.SplitHostPort(listenAddress)
	if err != nil {
		return "", err
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port), nil
}

func escapeTorControlString(input string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return replacer.Replace(input)
}

func isOnionAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	return strings.HasSuffix(strings.ToLower(host), ".onion")
}

func dialTorControl(address string, timeout time.Duration) (net.Conn, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTorControlUnreachable, err)
	}
	return conn, nil
}

package node

import (
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestTorRuntimeReestablishesOnionAfterRecovery(t *testing.T) {
	cfg := Config{
		RequireTorProxy: true,
		TorSOCKS:        "127.0.0.1:9050",
		TorDialTimeout:  time.Second,
	}

	runtime := NewTorRuntime(cfg, &OnionService{Address: "old.onion:9000"}, NewLogger("debug"))

	var mu sync.Mutex
	socksErr := errors.New("socks down")
	controlErr := errors.New("control down")
	recovered := false
	ensureCalls := 0

	runtime.checkSOCKS = func(string, time.Duration) error {
		mu.Lock()
		defer mu.Unlock()
		return socksErr
	}
	runtime.checkControl = func(Config) error {
		mu.Lock()
		defer mu.Unlock()
		return controlErr
	}
	runtime.ensureOnion = func(Config) (*OnionService, string, error) {
		mu.Lock()
		defer mu.Unlock()
		ensureCalls++
		recovered = true
		return &OnionService{Address: "new.onion:9000"}, "new.onion:9000", nil
	}

	runtime.Step()

	mu.Lock()
	socksErr = nil
	controlErr = nil
	mu.Unlock()
	runtime.Step()

	mu.Lock()
	if !recovered {
		t.Fatal("expected onion service to be re-established after tor recovery")
	}
	if ensureCalls != 1 {
		t.Fatalf("unexpected ensure call count: got %d want 1", ensureCalls)
	}
	mu.Unlock()
}

func TestTorRuntimeDoesNotReestablishWhenHealthy(t *testing.T) {
	cfg := Config{
		RequireTorProxy: true,
		TorSOCKS:        "127.0.0.1:9050",
		TorDialTimeout:  time.Second,
	}
	runtime := NewTorRuntime(cfg, &OnionService{Address: "alive.onion:9000"}, NewLogger("debug"))

	ensureCalls := 0
	runtime.checkSOCKS = func(string, time.Duration) error { return nil }
	runtime.checkControl = func(Config) error { return nil }
	runtime.ensureOnion = func(Config) (*OnionService, string, error) {
		ensureCalls++
		return &OnionService{Address: "new.onion:9000"}, "new.onion:9000", nil
	}

	runtime.Step()
	if ensureCalls != 0 {
		t.Fatalf("expected no re-establish while healthy, got %d", ensureCalls)
	}
}

func TestTorRuntimeCloseClosesService(t *testing.T) {
	cfg := Config{RequireTorProxy: true}
	runtime := NewTorRuntime(cfg, &OnionService{Address: "close.onion:9000", conn: &eofConn{}, id: "x"}, NewLogger("debug"))
	if err := runtime.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}
}

type eofConn struct{}

func (c *eofConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *eofConn) Write(b []byte) (int, error)      { return len(b), nil }
func (c *eofConn) Close() error                     { return nil }
func (c *eofConn) LocalAddr() net.Addr              { return nil }
func (c *eofConn) RemoteAddr() net.Addr             { return nil }
func (c *eofConn) SetDeadline(time.Time) error      { return nil }
func (c *eofConn) SetReadDeadline(time.Time) error  { return nil }
func (c *eofConn) SetWriteDeadline(time.Time) error { return nil }

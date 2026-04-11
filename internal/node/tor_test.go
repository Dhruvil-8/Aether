package node

import (
	"bufio"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestCheckTorSOCKS(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 3)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte{0x05, 0x00})

		head := make([]byte, 5)
		_, _ = conn.Read(head)
		hostLen := int(head[4])
		discard := make([]byte, hostLen+2)
		_, _ = conn.Read(discard)

		_, _ = conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0x23, 0x28})
	}()

	if err := CheckTorSOCKS(ln.Addr().String(), time.Second); err != nil {
		t.Fatalf("unexpected tor socks check error: %v", err)
	}
	<-done
}

func TestCheckTorControl(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		if line != "AUTHENTICATE\r\n" {
			return
		}
		_, _ = fmt.Fprint(conn, "250 OK\r\n")
	}()

	cfg := Config{
		TorControl:     ln.Addr().String(),
		TorDialTimeout: time.Second,
	}
	if err := CheckTorControl(cfg); err != nil {
		t.Fatalf("unexpected tor control check error: %v", err)
	}
	<-done
}

func TestValidateTorRuntimeSkipsInDevClearnet(t *testing.T) {
	cfg := Config{
		DevClearnet:     true,
		RequireTorProxy: true,
	}
	if err := ValidateTorRuntime(cfg, true); err != nil {
		t.Fatalf("expected dev clearnet tor validation skip, got %v", err)
	}
}

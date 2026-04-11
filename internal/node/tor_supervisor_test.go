package node

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestStartTorSupervisorManagedHelper(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		DataDir:           t.TempDir(),
		ManageTor:         true,
		RequireTorProxy:   true,
		DevClearnet:       false,
		TorSOCKS:          nextFreeAddress(t),
		TorControl:        nextFreeAddress(t),
		TorDialTimeout:    time.Second,
		TorStartupTimeout: 5 * time.Second,
		TorBinary:         "tor",
	}

	prev := startTorCommand
	t.Cleanup(func() {
		startTorCommand = prev
	})
	startTorCommand = func(ctx context.Context, _ Config, args []string) *exec.Cmd {
		allArgs := []string{"-test.run=TestTorSupervisorHelperProcess", "--"}
		allArgs = append(allArgs, args...)
		cmd := exec.CommandContext(ctx, exe, allArgs...)
		cmd.Env = append(os.Environ(), "GO_WANT_TOR_HELPER=1")
		return cmd
	}

	supervisor, err := StartTorSupervisor(context.Background(), cfg, NewLogger("error"))
	if err != nil {
		t.Fatalf("unexpected managed tor startup error: %v", err)
	}
	if supervisor == nil {
		t.Fatal("expected non-nil supervisor")
	}
	if err := supervisor.Close(); err != nil {
		t.Fatalf("unexpected supervisor close error: %v", err)
	}
}

func TestStartTorSupervisorFailsWhenChildExitsEarly(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		DataDir:           t.TempDir(),
		ManageTor:         true,
		RequireTorProxy:   true,
		DevClearnet:       false,
		TorSOCKS:          nextFreeAddress(t),
		TorControl:        nextFreeAddress(t),
		TorDialTimeout:    time.Second,
		TorStartupTimeout: 2 * time.Second,
		TorBinary:         "tor",
	}

	prev := startTorCommand
	t.Cleanup(func() {
		startTorCommand = prev
	})
	startTorCommand = func(ctx context.Context, _ Config, _ []string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, exe, "-test.run=TestTorSupervisorExitHelperProcess")
		cmd.Env = append(os.Environ(), "GO_WANT_TOR_HELPER_EXIT=1")
		return cmd
	}

	supervisor, err := StartTorSupervisor(context.Background(), cfg, NewLogger("error"))
	if err == nil {
		if supervisor != nil {
			_ = supervisor.Close()
		}
		t.Fatal("expected startup error when helper exits early")
	}
}

func TestTorSupervisorHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_TOR_HELPER") != "1" {
		return
	}

	socks := ""
	control := ""
	args := os.Args
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--SocksPort" {
			socks = strings.TrimSpace(args[i+1])
		}
		if args[i] == "--ControlPort" {
			control = strings.TrimSpace(args[i+1])
		}
	}
	if socks == "" || control == "" {
		os.Exit(2)
	}

	socksLn, err := net.Listen("tcp", socks)
	if err != nil {
		os.Exit(3)
	}
	defer socksLn.Close()
	controlLn, err := net.Listen("tcp", control)
	if err != nil {
		os.Exit(4)
	}
	defer controlLn.Close()

	go acceptSOCKS(socksLn)
	go acceptControl(controlLn)

	select {}
}

func TestTorSupervisorExitHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_TOR_HELPER_EXIT") != "1" {
		return
	}
	os.Exit(0)
}

func acceptSOCKS(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			head := make([]byte, 3)
			if _, err := ioReadFull(c, head); err != nil {
				return
			}
			_, _ = c.Write([]byte{0x05, 0x00})

			reqHead := make([]byte, 5)
			if _, err := ioReadFull(c, reqHead); err != nil {
				return
			}
			hostLen := int(reqHead[4])
			buf := make([]byte, hostLen+2)
			if _, err := ioReadFull(c, buf); err != nil {
				return
			}
			_, _ = c.Write([]byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0x23, 0x28})
		}(conn)
	}
}

func acceptControl(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			reader := bufio.NewReader(c)
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				line = strings.TrimSpace(line)
				switch {
				case strings.HasPrefix(line, "AUTHENTICATE"):
					_, _ = fmt.Fprint(c, "250 OK\r\n")
				case strings.HasPrefix(line, "DEL_ONION"):
					_, _ = fmt.Fprint(c, "250 OK\r\n")
				default:
					_, _ = fmt.Fprint(c, "250 OK\r\n")
				}
			}
		}(conn)
	}
}

func nextFreeAddress(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func ioReadFull(conn net.Conn, b []byte) (int, error) {
	total := 0
	for total < len(b) {
		n, err := conn.Read(b[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

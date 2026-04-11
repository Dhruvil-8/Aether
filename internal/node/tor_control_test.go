package node

import (
	"bufio"
	"strings"
	"testing"
)

func TestReadTorControlResponse(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("250-ServiceID=exampleid\r\n250-PrivateKey=ED25519-V3:testkey\r\n250 OK\r\n"))
	lines, err := readTorControlResponse(reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("unexpected line count: %d", len(lines))
	}
	if lines[0] != "ServiceID=exampleid" {
		t.Fatalf("unexpected first line: %q", lines[0])
	}
	if lines[1] != "PrivateKey=ED25519-V3:testkey" {
		t.Fatalf("unexpected second line: %q", lines[1])
	}
}

func TestOnionTarget(t *testing.T) {
	got, err := onionTarget("0.0.0.0:9000")
	if err != nil {
		t.Fatal(err)
	}
	if got != "127.0.0.1:9000" {
		t.Fatalf("unexpected onion target: %s", got)
	}
}

func TestIsOnionAddress(t *testing.T) {
	if !isOnionAddress("exampleexampleexampleexampleexampleexampleexampleexample.onion:9000") {
		t.Fatal("expected onion address to be detected")
	}
	if isOnionAddress("127.0.0.1:9000") {
		t.Fatal("did not expect clearnet address to be detected as onion")
	}
}

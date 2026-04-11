package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestWriteAndReadFrame(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte("abc")
	if err := WriteFrame(&buf, FramePing, payload); err != nil {
		t.Fatal(err)
	}
	frame, err := ReadFrame(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Type != FramePing {
		t.Fatalf("unexpected type: %d", frame.Type)
	}
	if string(frame.Payload) != "abc" {
		t.Fatalf("unexpected payload: %q", frame.Payload)
	}
}

func TestNetworkIDDeterministic(t *testing.T) {
	got := NetworkID()
	want := "f6b20e638555bd9ceec773f4b37793cff5f551968cbdb3aa12478eb81e58f6d3"
	if got != want {
		t.Fatalf("network id mismatch: got %s want %s", got, want)
	}
}

func TestReadFrameRejectsOversizedFrame(t *testing.T) {
	var buf bytes.Buffer
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, MaxFrameSize+1)
	if _, err := buf.Write(header); err != nil {
		t.Fatal(err)
	}

	_, err := ReadFrame(&buf)
	if err != ErrFrameTooLarge {
		t.Fatalf("unexpected error: got %v want %v", err, ErrFrameTooLarge)
	}
}

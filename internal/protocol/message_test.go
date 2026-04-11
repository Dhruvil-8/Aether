package protocol

import (
	"encoding/hex"
	"testing"
)

func TestEncodeBody(t *testing.T) {
	msg := &Message{
		V: 1,
		T: 1710000000,
		C: "hello",
	}
	body, err := msg.EncodeBody()
	if err != nil {
		t.Fatal(err)
	}

	got := hex.EncodeToString(body)
	want := "010000000065ec8780000568656c6c6f"
	if got != want {
		t.Fatalf("body mismatch: got %s want %s", got, want)
	}
}

func TestHashRoundTrip(t *testing.T) {
	msg := &Message{
		V: 1,
		T: 1710000000,
		C: "hello",
	}
	msg.H = msg.ComputeHash()
	if err := msg.VerifyHash(); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalEquivalentUnicodeHashesMatch(t *testing.T) {
	composed := &Message{
		V: 1,
		T: 1710000000,
		C: "Cafe\u00e9",
	}
	decomposed := &Message{
		V: 1,
		T: 1710000000,
		C: "Cafee\u0301",
	}

	if err := composed.ValidateContent(); err != nil {
		t.Fatal(err)
	}
	if err := decomposed.ValidateContent(); err != nil {
		t.Fatal(err)
	}

	if composed.C != decomposed.C {
		t.Fatalf("expected normalized content to match: %q vs %q", composed.C, decomposed.C)
	}

	composed.H = composed.ComputeHash()
	decomposed.H = decomposed.ComputeHash()
	if composed.H != decomposed.H {
		t.Fatalf("expected matching hashes, got %s vs %s", composed.H, decomposed.H)
	}
}

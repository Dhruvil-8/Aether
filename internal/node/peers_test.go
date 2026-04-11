package node

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestPeerBookMergeAndLoad(t *testing.T) {
	root := t.TempDir()
	book, err := OpenPeerBook(root)
	if err != nil {
		t.Fatal(err)
	}

	merged, err := book.Merge(" b.onion:9000 ", "a.onion:9000", "a.onion:9000", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.onion:9000", "b.onion:9000"}
	if !reflect.DeepEqual(merged, want) {
		t.Fatalf("merged peers mismatch: got %v want %v", merged, want)
	}

	reopened, err := OpenPeerBook(root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, want) {
		t.Fatalf("loaded peers mismatch: got %v want %v", loaded, want)
	}
	if filepath.Base(reopened.path) != "peers.json" {
		t.Fatalf("unexpected peer book path: %s", reopened.path)
	}
}

func TestCandidatePeersMergesConfigAndStored(t *testing.T) {
	root := t.TempDir()
	book, err := OpenPeerBook(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := book.Merge("b.onion:9000"); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		BootstrapPeers: []string{"c.onion:9000", "a.onion:9000"},
	}
	got := candidatePeers(cfg, book)
	want := []string{"a.onion:9000", "b.onion:9000", "c.onion:9000"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidate peers mismatch: got %v want %v", got, want)
	}
}

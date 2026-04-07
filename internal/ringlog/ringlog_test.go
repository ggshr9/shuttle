package ringlog

import (
	"fmt"
	"testing"
)

type testEntry struct {
	Msg string `json:"msg"`
}

func TestStore_LogAndRecent(t *testing.T) {
	dir := t.TempDir()
	s, err := New[testEntry](Config{LogDir: dir, MaxEntries: 5, FilePrefix: "test-"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 10; i++ {
		s.Log(&testEntry{Msg: fmt.Sprintf("msg-%d", i)})
	}
	recent := s.Recent(3)
	if len(recent) != 3 {
		t.Fatalf("expected 3, got %d", len(recent))
	}
	if recent[0].Msg != "msg-9" {
		t.Errorf("expected msg-9, got %s", recent[0].Msg)
	}
}

func TestStore_MemoryOnly(t *testing.T) {
	s, err := New[testEntry](Config{MaxEntries: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Log(&testEntry{Msg: "a"})
	s.Log(&testEntry{Msg: "b"})
	recent := s.Recent(10)
	if len(recent) != 2 {
		t.Fatalf("expected 2, got %d", len(recent))
	}
}

func TestStore_RingOverflow(t *testing.T) {
	s, err := New[testEntry](Config{MaxEntries: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Log(&testEntry{Msg: "a"})
	s.Log(&testEntry{Msg: "b"})
	s.Log(&testEntry{Msg: "c"})
	s.Log(&testEntry{Msg: "d"}) // overwrites "a"
	recent := s.Recent(10)
	if len(recent) != 3 {
		t.Fatalf("expected 3, got %d", len(recent))
	}
	if recent[0].Msg != "d" {
		t.Errorf("most recent should be d, got %s", recent[0].Msg)
	}
}

package observe

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRecorderRecord(t *testing.T) {
	r := NewRecorderManual()
	r.Record(Event{Kind: "dial", From: "a", To: "b", Detail: "test"})
	r.RecordF("send", "a", "b", "payload %d bytes", 1024)

	events := r.Events()
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Kind != "dial" {
		t.Errorf("event[0].Kind = %q, want %q", events[0].Kind, "dial")
	}
	if events[1].Detail != "payload 1024 bytes" {
		t.Errorf("event[1].Detail = %q", events[1].Detail)
	}
}

func TestRecorderLen(t *testing.T) {
	r := NewRecorderManual()
	if r.Len() != 0 {
		t.Fatalf("empty recorder Len = %d", r.Len())
	}
	r.Record(Event{Kind: "test"})
	if r.Len() != 1 {
		t.Fatalf("Len = %d, want 1", r.Len())
	}
}

func TestRecorderFormatEmpty(t *testing.T) {
	r := NewRecorderManual()
	out := r.Format()
	if !strings.Contains(out, "0 events") {
		t.Fatalf("expected 0 events in output, got: %s", out)
	}
}

func TestRecorderFormat(t *testing.T) {
	r := NewRecorderManual()
	now := time.Now()
	r.Record(Event{Time: now, Kind: "dial", From: "client", To: "server", Detail: "h3 transport"})
	r.Record(Event{Time: now.Add(50 * time.Millisecond), Kind: "send", From: "client", To: "server", Size: 1024})
	r.Record(Event{Time: now.Add(100 * time.Millisecond), Kind: "drop", From: "link", Detail: "loss=0.1"})

	out := r.Format()
	if !strings.Contains(out, "3 events") {
		t.Errorf("missing event count in output")
	}
	if !strings.Contains(out, "dial") {
		t.Errorf("missing dial event")
	}
	if !strings.Contains(out, "1024 bytes") {
		t.Errorf("missing size in send event")
	}
	if !strings.Contains(out, "loss=0.1") {
		t.Errorf("missing drop detail")
	}
}

func TestRecorderConcurrent(t *testing.T) {
	r := NewRecorderManual()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Record(Event{Kind: "test"})
		}()
	}
	wg.Wait()
	if r.Len() != 100 {
		t.Fatalf("Len = %d, want 100", r.Len())
	}
}

func TestRecorderEventsCopy(t *testing.T) {
	r := NewRecorderManual()
	r.Record(Event{Kind: "a"})
	events := r.Events()
	events[0].Kind = "modified"
	// Original should be unchanged.
	if r.Events()[0].Kind != "a" {
		t.Fatal("Events() did not return a copy")
	}
}

func TestRecorderSizeInDetail(t *testing.T) {
	r := NewRecorderManual()
	r.Record(Event{Kind: "recv", From: "srv", To: "cli", Detail: "response", Size: 512})

	out := r.Format()
	if !strings.Contains(out, "response (512 bytes)") {
		t.Errorf("expected combined detail+size, got: %s", out)
	}
}

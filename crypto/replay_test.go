package crypto

import (
	"encoding/binary"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// newCuckooFilter — basic insert, lookup, count, capacity
// ---------------------------------------------------------------------------

func TestCuckooFilter_InsertAndLookup(t *testing.T) {
	cf := newCuckooFilter(1024)

	if cf.Lookup(42) {
		t.Fatal("expected Lookup to return false for value not yet inserted")
	}

	if !cf.Insert(42) {
		t.Fatal("expected Insert to succeed")
	}

	if !cf.Lookup(42) {
		t.Fatal("expected Lookup to return true after insert")
	}
}

func TestCuckooFilter_Count(t *testing.T) {
	cf := newCuckooFilter(1024)

	if cf.Count() != 0 {
		t.Fatalf("expected count 0, got %d", cf.Count())
	}

	cf.Insert(1)
	cf.Insert(2)
	cf.Insert(3)

	if cf.Count() != 3 {
		t.Fatalf("expected count 3, got %d", cf.Count())
	}
}

func TestCuckooFilter_Capacity(t *testing.T) {
	// A filter with capacity 16 should have numBuckets = nextPow2(16/4) = 4.
	cf := newCuckooFilter(16)
	if cf.numBuckets != 4 {
		t.Fatalf("expected numBuckets 4, got %d", cf.numBuckets)
	}

	// Zero capacity falls back to 1024 buckets.
	cf0 := newCuckooFilter(0)
	if cf0.numBuckets != 1024 {
		t.Fatalf("expected numBuckets 1024 for zero capacity, got %d", cf0.numBuckets)
	}
}

func TestCuckooFilter_ManyInserts(t *testing.T) {
	cf := newCuckooFilter(4096)
	inserted := 0
	for i := uint64(0); i < 2000; i++ {
		if cf.Insert(i) {
			inserted++
		}
	}
	if inserted == 0 {
		t.Fatal("expected at least some inserts to succeed")
	}
	if cf.Count() != inserted {
		t.Fatalf("count mismatch: Count()=%d, inserted=%d", cf.Count(), inserted)
	}
}

// ---------------------------------------------------------------------------
// nextPow2 — edge cases
// ---------------------------------------------------------------------------

func TestNextPow2(t *testing.T) {
	tests := []struct {
		input    uint32
		expected uint32
	}{
		{0, 0},     // underflows: 0-1 = max uint32, all bits set, +1 wraps to 0
		{1, 1},
		{2, 2},
		{3, 4},
		{4, 4},
		{5, 8},
		{7, 8},
		{8, 8},
		{9, 16},
		{255, 256},
		{256, 256},
		{257, 512},
		{1000, 1024},
		{1024, 1024},
		{1025, 2048},
	}

	for _, tt := range tests {
		got := nextPow2(tt.input)
		if got != tt.expected {
			t.Errorf("nextPow2(%d) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// ReplayFilter.Check — first call returns false, second returns true
// ---------------------------------------------------------------------------

func TestReplayFilter_Check_FirstAndSecond(t *testing.T) {
	rf := NewReplayFilter(10 * time.Second)

	nonce := uint64(123456789)

	if rf.Check(nonce) {
		t.Fatal("first Check should return false (not a replay)")
	}
	if !rf.Check(nonce) {
		t.Fatal("second Check should return true (replay detected)")
	}
}

func TestReplayFilter_Check_DifferentNonces(t *testing.T) {
	rf := NewReplayFilter(10 * time.Second)

	for i := uint64(0); i < 100; i++ {
		if rf.Check(i) {
			t.Fatalf("first Check(%d) should return false", i)
		}
	}

	for i := uint64(0); i < 100; i++ {
		if !rf.Check(i) {
			t.Fatalf("second Check(%d) should return true (replay)", i)
		}
	}
}

// ---------------------------------------------------------------------------
// ReplayFilter.CheckBytes — full 32-byte nonce usage
// ---------------------------------------------------------------------------

func TestReplayFilter_CheckBytes_32ByteNonce(t *testing.T) {
	rf := NewReplayFilter(10 * time.Second)

	nonce := make([]byte, 32)
	for i := range nonce {
		nonce[i] = byte(i + 1)
	}

	if rf.CheckBytes(nonce) {
		t.Fatal("first CheckBytes should return false")
	}
	if !rf.CheckBytes(nonce) {
		t.Fatal("second CheckBytes should return true (replay)")
	}
}

func TestReplayFilter_CheckBytes_DifferentAfterByte8(t *testing.T) {
	// Two nonces that share the same first 8 bytes but differ in bytes 8-31
	// must be treated as different nonces (the XOR folding incorporates all bytes).
	rf := NewReplayFilter(10 * time.Second)

	nonceA := make([]byte, 32)
	nonceB := make([]byte, 32)

	// Same first 8 bytes.
	binary.LittleEndian.PutUint64(nonceA[0:8], 0xDEADBEEFCAFEBABE)
	binary.LittleEndian.PutUint64(nonceB[0:8], 0xDEADBEEFCAFEBABE)

	// Different second 8 bytes.
	binary.LittleEndian.PutUint64(nonceA[8:16], 0x1111111111111111)
	binary.LittleEndian.PutUint64(nonceB[8:16], 0x2222222222222222)

	// Rest can be zero — the difference in bytes 8-15 is enough.

	if rf.CheckBytes(nonceA) {
		t.Fatal("first CheckBytes(nonceA) should return false")
	}
	if rf.CheckBytes(nonceB) {
		t.Fatal("first CheckBytes(nonceB) should return false — it differs from nonceA")
	}

	// Now both should be detected as replays.
	if !rf.CheckBytes(nonceA) {
		t.Fatal("second CheckBytes(nonceA) should return true (replay)")
	}
	if !rf.CheckBytes(nonceB) {
		t.Fatal("second CheckBytes(nonceB) should return true (replay)")
	}
}

func TestReplayFilter_CheckBytes_DifferentInLaterChunks(t *testing.T) {
	// Nonces identical in first 16 bytes but different in bytes 16-23.
	rf := NewReplayFilter(10 * time.Second)

	nonceA := make([]byte, 32)
	nonceB := make([]byte, 32)

	// Same first 16 bytes.
	binary.LittleEndian.PutUint64(nonceA[0:8], 0xAAAAAAAAAAAAAAAA)
	binary.LittleEndian.PutUint64(nonceA[8:16], 0xBBBBBBBBBBBBBBBB)
	copy(nonceB[0:16], nonceA[0:16])

	// Different bytes 16-23.
	binary.LittleEndian.PutUint64(nonceA[16:24], 0x0000000000000001)
	binary.LittleEndian.PutUint64(nonceB[16:24], 0x0000000000000002)

	if rf.CheckBytes(nonceA) {
		t.Fatal("first CheckBytes(nonceA) should return false")
	}
	if rf.CheckBytes(nonceB) {
		t.Fatal("first CheckBytes(nonceB) should return false")
	}
}

// ---------------------------------------------------------------------------
// ReplayFilter.CheckBytes — short nonces
// ---------------------------------------------------------------------------

func TestReplayFilter_CheckBytes_ShortNonce(t *testing.T) {
	rf := NewReplayFilter(10 * time.Second)

	// Short nonces (< 8 bytes) are now handled correctly via FNV-1a.
	short := []byte{1, 2, 3, 4, 5, 6, 7}
	if rf.CheckBytes(short) {
		t.Fatal("first CheckBytes with short nonce should return false")
	}
	// Second call should detect replay.
	if !rf.CheckBytes(short) {
		t.Fatal("second CheckBytes with short nonce should detect replay")
	}
}

func TestReplayFilter_CheckBytes_EmptyNonce(t *testing.T) {
	rf := NewReplayFilter(10 * time.Second)

	if rf.CheckBytes(nil) {
		t.Fatal("CheckBytes(nil) should return false")
	}
	if rf.CheckBytes([]byte{}) {
		t.Fatal("CheckBytes(empty) should return false")
	}
}

func TestReplayFilter_CheckBytes_Exactly8Bytes(t *testing.T) {
	rf := NewReplayFilter(10 * time.Second)

	nonce := make([]byte, 8)
	binary.LittleEndian.PutUint64(nonce, 0x1234567890ABCDEF)

	if rf.CheckBytes(nonce) {
		t.Fatal("first CheckBytes should return false")
	}
	if !rf.CheckBytes(nonce) {
		t.Fatal("second CheckBytes should return true (replay)")
	}
}

// ---------------------------------------------------------------------------
// ReplayFilter.Size — count tracking
// ---------------------------------------------------------------------------

func TestReplayFilter_Size(t *testing.T) {
	rf := NewReplayFilter(10 * time.Second)

	if rf.Size() != 0 {
		t.Fatalf("expected initial Size 0, got %d", rf.Size())
	}

	rf.Check(1)
	rf.Check(2)
	rf.Check(3)

	if rf.Size() != 3 {
		t.Fatalf("expected Size 3, got %d", rf.Size())
	}

	// Re-checking existing nonces should not increase size.
	rf.Check(1)
	rf.Check(2)

	if rf.Size() != 3 {
		t.Fatalf("expected Size still 3 after replays, got %d", rf.Size())
	}
}

// ---------------------------------------------------------------------------
// Concurrent access — multiple goroutines checking simultaneously
// ---------------------------------------------------------------------------

func TestReplayFilter_Concurrent(t *testing.T) {
	rf := NewReplayFilter(10 * time.Second)

	const numGoroutines = 50
	const checksPerGoroutine = 200

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Each goroutine uses its own range of nonces to avoid false positives
	// from the probabilistic cuckoo filter, and also shares some overlapping
	// nonces to exercise concurrent replay detection.
	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			base := uint64(id * checksPerGoroutine) //nolint:gosec // test code: id and checksPerGoroutine are small positive constants
			for i := uint64(0); i < checksPerGoroutine; i++ {
				rf.Check(base + i)
			}
		}(g)
	}

	wg.Wait()

	// The filter should have recorded entries without panicking or deadlocking.
	size := rf.Size()
	if size == 0 {
		t.Fatal("expected non-zero size after concurrent checks")
	}
}

func TestReplayFilter_ConcurrentCheckBytes(t *testing.T) {
	rf := NewReplayFilter(10 * time.Second)

	const numGoroutines = 30

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			nonce := make([]byte, 32)
			binary.LittleEndian.PutUint64(nonce[0:8], uint64(id))   //nolint:gosec // G115: test goroutine id, always non-negative
			binary.LittleEndian.PutUint64(nonce[8:16], uint64(id*17)) //nolint:gosec // G115: test goroutine id, always non-negative
			binary.LittleEndian.PutUint64(nonce[16:24], uint64(id*31)) //nolint:gosec // G115: test goroutine id, always non-negative
			binary.LittleEndian.PutUint64(nonce[24:32], uint64(id*53))

			// First check should not be a replay.
			rf.CheckBytes(nonce)
			// Second check should detect replay.
			rf.CheckBytes(nonce)
		}(g)
	}

	wg.Wait()

	size := rf.Size()
	if size == 0 {
		t.Fatal("expected non-zero size after concurrent CheckBytes")
	}
}

// ---------------------------------------------------------------------------
// ReplayFilter with default window
// ---------------------------------------------------------------------------

func TestReplayFilter_DefaultWindow(t *testing.T) {
	// Passing zero window should use the default (120s) without panic.
	rf := NewReplayFilter(0)

	if rf.Check(999) {
		t.Fatal("first Check should return false")
	}
	if !rf.Check(999) {
		t.Fatal("second Check should return true")
	}
}

// ---------------------------------------------------------------------------
// Cuckoo filter fingerprint never returns zero
// ---------------------------------------------------------------------------

func TestCuckooFilter_FingerprintNonZero(t *testing.T) {
	cf := newCuckooFilter(1024)

	// Test a range of values to ensure fingerprint is never zero
	// (zero is used as the empty sentinel in buckets).
	for i := uint64(0); i < 10000; i++ {
		fp := cf.fingerprint(i)
		if fp == 0 {
			t.Fatalf("fingerprint(%d) returned 0, expected non-zero", i)
		}
	}
}

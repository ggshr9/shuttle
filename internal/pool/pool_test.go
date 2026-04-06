package pool

import (
	"testing"
)

func TestGetPut(t *testing.T) {
	buf := Get(100)
	if len(buf) != 100 {
		t.Fatalf("Get(100): got len %d, want 100", len(buf))
	}
	// Put should not panic
	Put(buf)
}

func TestPoolSizes(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantCap int
	}{
		{"Small", 512, 1024},
		{"Medium", 8 * 1024, 16 * 1024},
		{"MedLarge", 24 * 1024, 32 * 1024},
		{"Large", 48 * 1024, 64 * 1024},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := Get(tc.size)
			if len(buf) != tc.size {
				t.Fatalf("len = %d, want %d", len(buf), tc.size)
			}
			if cap(buf) != tc.wantCap {
				t.Fatalf("cap = %d, want %d", cap(buf), tc.wantCap)
			}
			Put(buf)
		})
	}
}

func TestGetReuse(t *testing.T) {
	// Get a buffer, put it back, get again — should work without error.
	buf1 := Get(1024)
	Put(buf1)
	buf2 := Get(1024)
	// We can't guarantee buf2 == buf1 (GC may collect pooled items),
	// but it should have the correct size.
	if len(buf2) != 1024 {
		t.Fatalf("reuse: got len %d, want 1024", len(buf2))
	}
	Put(buf2)
}

func TestPool32KBTier(t *testing.T) {
	// Test the 32KB tier via Get(size) auto-selection.
	buf := Get(32 * 1024)
	if len(buf) != 32*1024 {
		t.Fatalf("Get(32KB): got len %d, want %d", len(buf), 32*1024)
	}
	if cap(buf) != 32*1024 {
		t.Fatalf("Get(32KB): got cap %d, want %d", cap(buf), 32*1024)
	}
	Put(buf)

	// Test GetMedLarge/PutMedLarge convenience functions.
	buf2 := GetMedLarge()
	if len(buf2) != 32*1024 {
		t.Fatalf("GetMedLarge: got len %d, want %d", len(buf2), 32*1024)
	}
	if cap(buf2) != 32*1024 {
		t.Fatalf("GetMedLarge: got cap %d, want %d", cap(buf2), 32*1024)
	}
	PutMedLarge(buf2)

	// Verify a size between 16KB and 32KB goes to MedLarge tier.
	buf3 := Get(20 * 1024)
	if len(buf3) != 20*1024 {
		t.Fatalf("Get(20KB): got len %d, want %d", len(buf3), 20*1024)
	}
	if cap(buf3) != 32*1024 {
		t.Fatalf("Get(20KB): got cap %d, want %d", cap(buf3), 32*1024)
	}
	Put(buf3)
}

func TestPoolStats(t *testing.T) {
	ResetStats()

	// Snapshot baseline (other tests may have left residual counts).
	base := Stats()
	baseSmallGets := base["small"].Gets
	baseMedGets := base["medium"].Gets
	baseMlGets := base["med_large"].Gets
	baseLgGets := base["large"].Gets
	baseSmallPuts := base["small"].Puts

	// Perform some operations.
	buf1 := Get(512)   // small
	buf2 := Get(8192)  // medium
	buf3 := Get(32768) // med_large
	buf4 := Get(65536) // large

	stats := Stats()

	if stats["small"].Gets-baseSmallGets != 1 {
		t.Errorf("small gets delta: got %d, want 1", stats["small"].Gets-baseSmallGets)
	}
	if stats["medium"].Gets-baseMedGets != 1 {
		t.Errorf("medium gets delta: got %d, want 1", stats["medium"].Gets-baseMedGets)
	}
	if stats["med_large"].Gets-baseMlGets != 1 {
		t.Errorf("med_large gets delta: got %d, want 1", stats["med_large"].Gets-baseMlGets)
	}
	if stats["large"].Gets-baseLgGets != 1 {
		t.Errorf("large gets delta: got %d, want 1", stats["large"].Gets-baseLgGets)
	}

	Put(buf1)
	Put(buf2)
	Put(buf3)
	Put(buf4)

	stats = Stats()
	if stats["small"].Puts-baseSmallPuts != 1 {
		t.Errorf("small puts delta: got %d, want 1", stats["small"].Puts-baseSmallPuts)
	}
}

func TestPoolStatsReset(t *testing.T) {
	// Generate some stats.
	buf := Get(100)
	Put(buf)

	ResetStats()

	stats := Stats()
	for name, s := range stats {
		if s.Gets != 0 || s.Puts != 0 || s.Misses != 0 {
			t.Errorf("%s: expected zeroed stats after reset, got gets=%d puts=%d misses=%d",
				name, s.Gets, s.Puts, s.Misses)
		}
	}
}

func TestPutMedLargeNoZero(t *testing.T) {
	// Verify that PutMedLargeNoZero does NOT clear the buffer by checking
	// the implementation directly, rather than relying on sync.Pool returning
	// the same object (which is not guaranteed and is flaky under -race / GC).
	buf := make([]byte, 32*1024)
	for i := range buf {
		buf[i] = 0xAB
	}

	// Call PutMedLargeNoZero — this should NOT zero the buffer.
	PutMedLargeNoZero(buf)

	// The buffer slice we still hold should retain its data, because
	// PutMedLargeNoZero does not call clear(). (In contrast, PutMedLarge
	// does call clear() and would zero the underlying array.)
	found := false
	for _, b := range buf {
		if b == 0xAB {
			found = true
			break
		}
	}
	if !found {
		t.Error("PutMedLargeNoZero should not zero the buffer, but data was cleared")
	}
}

func TestPutMedLarge_DoesZero(t *testing.T) {
	// Verify that PutMedLarge DOES zero the buffer.
	buf := make([]byte, 32*1024)
	for i := range buf {
		buf[i] = 0xCD
	}

	PutMedLarge(buf)

	// The buffer should now be zeroed because PutMedLarge calls clear().
	for _, b := range buf {
		if b != 0 {
			t.Error("PutMedLarge should zero the buffer, but found non-zero data")
			break
		}
	}
}

func TestPoolAutoSize(t *testing.T) {
	ResetStats()

	tests := []struct {
		size     int
		wantCap  int
		wantTier string
	}{
		{100, 1024, "small"},
		{1024, 1024, "small"},
		{1025, 16 * 1024, "medium"},
		{16 * 1024, 16 * 1024, "medium"},
		{16*1024 + 1, 32 * 1024, "med_large"},
		{32 * 1024, 32 * 1024, "med_large"},
		{32*1024 + 1, 64 * 1024, "large"},
		{64 * 1024, 64 * 1024, "large"},
	}

	for _, tc := range tests {
		buf := Get(tc.size)
		if cap(buf) != tc.wantCap {
			t.Errorf("Get(%d): cap=%d, want %d (tier %s)", tc.size, cap(buf), tc.wantCap, tc.wantTier)
		}
		Put(buf)
	}

	stats := Stats()
	// Verify each tier got the expected number of gets.
	if stats["small"].Gets != 2 {
		t.Errorf("small gets: got %d, want 2", stats["small"].Gets)
	}
	if stats["medium"].Gets != 2 {
		t.Errorf("medium gets: got %d, want 2", stats["medium"].Gets)
	}
	if stats["med_large"].Gets != 2 {
		t.Errorf("med_large gets: got %d, want 2", stats["med_large"].Gets)
	}
	if stats["large"].Gets != 2 {
		t.Errorf("large gets: got %d, want 2", stats["large"].Gets)
	}
}

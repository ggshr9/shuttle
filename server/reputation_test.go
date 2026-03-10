package server

import (
	"testing"
	"time"
)

func TestReputationIsBanned_NotBanned(t *testing.T) {
	r := NewReputation(DefaultReputationConfig())
	if r.IsBanned("1.2.3.4") {
		t.Error("new IP should not be banned")
	}
}

func TestReputationRecordFailure_UnderThreshold(t *testing.T) {
	r := NewReputation(ReputationConfig{MaxFailures: 5})
	for i := 0; i < 4; i++ {
		if banned := r.RecordFailure("1.2.3.4"); banned {
			t.Fatalf("should not be banned after %d failures", i+1)
		}
	}
	if r.IsBanned("1.2.3.4") {
		t.Error("IP should not be banned after 4 failures (threshold 5)")
	}
}

func TestReputationRecordFailure_AtThreshold(t *testing.T) {
	r := NewReputation(ReputationConfig{MaxFailures: 5})
	for i := 0; i < 4; i++ {
		r.RecordFailure("1.2.3.4")
	}
	if banned := r.RecordFailure("1.2.3.4"); !banned {
		t.Error("5th failure should trigger ban")
	}
	if !r.IsBanned("1.2.3.4") {
		t.Error("IP should be banned after 5 failures")
	}
}

func TestReputationBanEscalation(t *testing.T) {
	durations := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		40 * time.Millisecond,
	}
	r := NewReputation(ReputationConfig{
		MaxFailures:  2,
		WindowSize:   time.Minute,
		BanDurations: durations,
	})

	// First ban
	r.RecordFailure("1.2.3.4")
	r.RecordFailure("1.2.3.4")
	if !r.IsBanned("1.2.3.4") {
		t.Fatal("should be banned after first round")
	}

	// Wait for first ban to expire
	time.Sleep(15 * time.Millisecond)
	if r.IsBanned("1.2.3.4") {
		t.Fatal("first ban should have expired")
	}

	// Second ban — should use second (longer) duration
	r.RecordFailure("1.2.3.4")
	r.RecordFailure("1.2.3.4")
	if !r.IsBanned("1.2.3.4") {
		t.Fatal("should be banned after second round")
	}

	// Still banned after 15ms (second ban is 20ms)
	time.Sleep(15 * time.Millisecond)
	if !r.IsBanned("1.2.3.4") {
		t.Fatal("second ban should still be active (escalated duration)")
	}

	// Expired after another 10ms
	time.Sleep(10 * time.Millisecond)
	if r.IsBanned("1.2.3.4") {
		t.Fatal("second ban should have expired")
	}

	// Third ban — uses third duration
	r.RecordFailure("1.2.3.4")
	r.RecordFailure("1.2.3.4")
	if !r.IsBanned("1.2.3.4") {
		t.Fatal("should be banned after third round")
	}

	// Fourth ban — should cap at last duration index
	time.Sleep(50 * time.Millisecond)
	r.RecordFailure("1.2.3.4")
	r.RecordFailure("1.2.3.4")
	if !r.IsBanned("1.2.3.4") {
		t.Fatal("should be banned after fourth round")
	}
}

func TestReputationRecordSuccess(t *testing.T) {
	r := NewReputation(ReputationConfig{MaxFailures: 3})
	r.RecordFailure("1.2.3.4")
	r.RecordFailure("1.2.3.4")
	r.RecordSuccess("1.2.3.4")

	// After success, failures are reset — another 2 failures should NOT ban
	r.RecordFailure("1.2.3.4")
	r.RecordFailure("1.2.3.4")
	if r.IsBanned("1.2.3.4") {
		t.Error("IP should not be banned; success should have reset state")
	}

	// Third failure after reset should trigger ban
	if banned := r.RecordFailure("1.2.3.4"); !banned {
		t.Error("3rd failure after reset should trigger ban")
	}
}

func TestReputationBannedIPs(t *testing.T) {
	r := NewReputation(ReputationConfig{
		MaxFailures:  1,
		BanDurations: []time.Duration{time.Hour},
	})

	r.RecordFailure("1.1.1.1")
	r.RecordFailure("2.2.2.2")

	banned := r.BannedIPs()
	if len(banned) != 2 {
		t.Fatalf("BannedIPs() len = %d, want 2", len(banned))
	}
	if _, ok := banned["1.1.1.1"]; !ok {
		t.Error("1.1.1.1 should be in banned list")
	}
	if _, ok := banned["2.2.2.2"]; !ok {
		t.Error("2.2.2.2 should be in banned list")
	}

	// Unbanned IP should not be in the list
	r.RecordSuccess("1.1.1.1")
	banned = r.BannedIPs()
	if len(banned) != 1 {
		t.Fatalf("BannedIPs() len = %d after success, want 1", len(banned))
	}
}

func TestReputationCleanup(t *testing.T) {
	r := NewReputation(ReputationConfig{
		MaxFailures:  1,
		BanDurations: []time.Duration{10 * time.Millisecond},
	})

	r.RecordFailure("1.1.1.1")
	r.RecordFailure("2.2.2.2")

	// Both banned
	if len(r.BannedIPs()) != 2 {
		t.Fatal("expected 2 banned IPs")
	}

	// Wait for bans to expire
	time.Sleep(15 * time.Millisecond)

	// Cleanup should remove expired records
	r.Cleanup()

	r.mu.Lock()
	remaining := len(r.ips)
	r.mu.Unlock()

	if remaining != 0 {
		t.Errorf("after cleanup, %d records remain, want 0", remaining)
	}
}

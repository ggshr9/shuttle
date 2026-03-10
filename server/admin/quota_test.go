package admin

import (
	"testing"

	"github.com/shuttle-proxy/shuttle/config"
)

func TestQuotaExceeded_NoLimit(t *testing.T) {
	u := &UserState{User: config.User{MaxBytes: 0}}
	u.BytesSent.Store(999999)
	u.BytesRecv.Store(999999)
	if u.QuotaExceeded() {
		t.Error("MaxBytes=0 should mean no quota (never exceeded)")
	}
}

func TestQuotaExceeded_UnderLimit(t *testing.T) {
	u := &UserState{User: config.User{MaxBytes: 1000}}
	u.BytesSent.Store(300)
	u.BytesRecv.Store(400)
	if u.QuotaExceeded() {
		t.Errorf("total %d < max %d, should not be exceeded", u.TotalBytes(), u.MaxBytes)
	}
}

func TestQuotaExceeded_AtLimit(t *testing.T) {
	u := &UserState{User: config.User{MaxBytes: 1000}}
	u.BytesSent.Store(600)
	u.BytesRecv.Store(400)
	if !u.QuotaExceeded() {
		t.Errorf("total %d == max %d, should be exceeded", u.TotalBytes(), u.MaxBytes)
	}
}

func TestQuotaExceeded_OverLimit(t *testing.T) {
	u := &UserState{User: config.User{MaxBytes: 1000}}
	u.BytesSent.Store(800)
	u.BytesRecv.Store(500)
	if !u.QuotaExceeded() {
		t.Errorf("total %d > max %d, should be exceeded", u.TotalBytes(), u.MaxBytes)
	}
}

func TestTotalBytes(t *testing.T) {
	u := &UserState{}
	u.BytesSent.Store(123)
	u.BytesRecv.Store(456)
	got := u.TotalBytes()
	want := int64(579)
	if got != want {
		t.Errorf("TotalBytes() = %d, want %d", got, want)
	}
}

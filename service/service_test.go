package service

import "testing"

func TestStatusString(t *testing.T) {
	tests := []struct {
		s    Status
		want string
	}{
		{StatusRunning, "running"},
		{StatusStopped, "stopped"},
		{StatusNotInstalled, "not-installed"},
		{StatusUnknown, "unknown"},
	}
	for _, tc := range tests {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("Status(%d).String() = %q, want %q", tc.s, got, tc.want)
		}
	}
}

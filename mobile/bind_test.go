package mobile

import (
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// ValidateConfig tests
// ---------------------------------------------------------------------------

func TestValidateConfigValid(t *testing.T) {
	t.Parallel()
	cfg := `{
		"server": {"addr": "example.com:443", "password": "secret"},
		"transport": {"h3": {"enabled": true}}
	}`
	result := ValidateConfig(cfg)
	if result != "" {
		t.Fatalf("expected empty string for valid config, got %q", result)
	}
}

func TestValidateConfigMissingServerAddr(t *testing.T) {
	t.Parallel()
	cfg := `{
		"server": {"password": "secret"},
		"transport": {"h3": {"enabled": true}}
	}`
	result := ValidateConfig(cfg)
	if result == "" {
		t.Fatal("expected error for missing server address, got empty string")
	}
	if !strings.Contains(result, "server address") {
		t.Fatalf("expected error about server address, got %q", result)
	}
}

func TestValidateConfigMissingPassword(t *testing.T) {
	t.Parallel()
	cfg := `{
		"server": {"addr": "example.com:443"},
		"transport": {"h3": {"enabled": true}}
	}`
	result := ValidateConfig(cfg)
	if result == "" {
		t.Fatal("expected error for missing password, got empty string")
	}
	if !strings.Contains(result, "password") {
		t.Fatalf("expected error about password, got %q", result)
	}
}

func TestValidateConfigInvalidJSON(t *testing.T) {
	t.Parallel()
	result := ValidateConfig("{not valid json")
	if result == "" {
		t.Fatal("expected error for invalid JSON, got empty string")
	}
	if !strings.Contains(result, "invalid JSON") {
		t.Fatalf("expected 'invalid JSON' in error, got %q", result)
	}
}

func TestValidateConfigEmptyJSON(t *testing.T) {
	t.Parallel()
	result := ValidateConfig("{}")
	if result == "" {
		t.Fatal("expected error for empty config, got empty string")
	}
}

func TestValidateConfigNoTransportEnabled(t *testing.T) {
	t.Parallel()
	// Server fields present but no transport enabled — engine.ValidateConfig rejects this.
	cfg := `{
		"server": {"addr": "example.com:443", "password": "secret"},
		"transport": {}
	}`
	result := ValidateConfig(cfg)
	if result == "" {
		t.Fatal("expected error when no transport is enabled, got empty string")
	}
	if !strings.Contains(result, "transport") {
		t.Fatalf("expected error about transports, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// MobileError tests
// ---------------------------------------------------------------------------

func TestMobileErrorFormat(t *testing.T) {
	t.Parallel()
	err := NewMobileError(ErrInvalidConfig, "bad config")
	if err.Code != ErrInvalidConfig {
		t.Fatalf("expected code %d, got %d", ErrInvalidConfig, err.Code)
	}
	if err.Message != "bad config" {
		t.Fatalf("expected message %q, got %q", "bad config", err.Message)
	}
	expected := "[3] bad config"
	if err.Error() != expected {
		t.Fatalf("expected Error() = %q, got %q", expected, err.Error())
	}
}

func TestMobileErrorCodes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code int
		name string
	}{
		{ErrAlreadyRunning, "ErrAlreadyRunning"},
		{ErrNotRunning, "ErrNotRunning"},
		{ErrInvalidConfig, "ErrInvalidConfig"},
		{ErrStartFailed, "ErrStartFailed"},
		{ErrReloadFailed, "ErrReloadFailed"},
	}
	for _, tc := range tests {
		err := NewMobileError(tc.code, tc.name)
		if err.Code != tc.code {
			t.Errorf("%s: expected code %d, got %d", tc.name, tc.code, err.Code)
		}
		// The formatted string should contain the code in brackets.
		if !strings.Contains(err.Error(), "[") || !strings.Contains(err.Error(), "]") {
			t.Errorf("%s: Error() missing brackets: %q", tc.name, err.Error())
		}
	}
}

func TestMobileErrorImplementsError(t *testing.T) {
	t.Parallel()
	var err error = NewMobileError(ErrStartFailed, "test")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.Error() == "" {
		t.Fatal("expected non-empty error string")
	}
}

// ---------------------------------------------------------------------------
// LogBuffer tests
// ---------------------------------------------------------------------------

func TestLogBufferWriteAndRead(t *testing.T) {
	t.Parallel()
	buf := newLogBuffer(10)
	buf.write("line-1")
	buf.write("line-2")
	buf.write("line-3")

	result := buf.recent(0)
	lines := strings.Split(result, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), result)
	}
	if lines[0] != "line-1" || lines[1] != "line-2" || lines[2] != "line-3" {
		t.Fatalf("unexpected lines: %v", lines)
	}
}

func TestLogBufferMaxLines(t *testing.T) {
	t.Parallel()
	buf := newLogBuffer(10)
	for i := 0; i < 5; i++ {
		buf.write("line")
	}
	result := buf.recent(2)
	lines := strings.Split(result, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines with maxLines=2, got %d", len(lines))
	}
}

func TestLogBufferRingOverflow(t *testing.T) {
	t.Parallel()
	buf := newLogBuffer(3)
	buf.write("a")
	buf.write("b")
	buf.write("c")
	buf.write("d") // overwrites "a"
	buf.write("e") // overwrites "b"

	result := buf.recent(0)
	lines := strings.Split(result, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (ring size), got %d: %q", len(lines), result)
	}
	// After overflow, the most recent 3 are: c, d, e
	if lines[0] != "c" || lines[1] != "d" || lines[2] != "e" {
		t.Fatalf("expected [c, d, e], got %v", lines)
	}
}

func TestLogBufferEmpty(t *testing.T) {
	t.Parallel()
	buf := newLogBuffer(5)
	result := buf.recent(0)
	if result != "" {
		t.Fatalf("expected empty string for empty buffer, got %q", result)
	}
}

func TestLogBufferDefaultSize(t *testing.T) {
	t.Parallel()
	buf := newLogBuffer(0)
	if buf.size != defaultLogBufferSize {
		t.Fatalf("expected default size %d, got %d", defaultLogBufferSize, buf.size)
	}
}

func TestLogBufferConcurrency(t *testing.T) {
	t.Parallel()
	buf := newLogBuffer(100)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				buf.write("line")
			}
		}(i)
	}
	wg.Wait()
	// 200 writes into a buffer of 100 — should have exactly 100 lines.
	result := buf.recent(0)
	lines := strings.Split(result, "\n")
	if len(lines) != 100 {
		t.Fatalf("expected 100 lines after concurrent writes, got %d", len(lines))
	}
}

func TestGetRecentLogsIntegration(t *testing.T) {
	t.Parallel()
	// GetRecentLogs uses the package-level logs buffer.
	// We can't control its contents entirely but can verify it doesn't panic.
	result := GetRecentLogs(5)
	// Just verify the function doesn't panic and returns a string.
	_ = result
}

// ---------------------------------------------------------------------------
// Callback tests
// ---------------------------------------------------------------------------

// mockCallback records all callback invocations for verification.
type mockCallback struct {
	mu             sync.Mutex
	statusChanges  []string
	networkChanges int
	errors         []struct {
		code    int
		message string
	}
	speedUpdates []struct {
		upload, download int64
	}
}

func (m *mockCallback) OnStatusChange(state string) {
	m.mu.Lock()
	m.statusChanges = append(m.statusChanges, state)
	m.mu.Unlock()
}

func (m *mockCallback) OnNetworkChange() {
	m.mu.Lock()
	m.networkChanges++
	m.mu.Unlock()
}

func (m *mockCallback) OnError(code int, message string) {
	m.mu.Lock()
	m.errors = append(m.errors, struct {
		code    int
		message string
	}{code, message})
	m.mu.Unlock()
}

func (m *mockCallback) OnSpeedUpdate(upload, download int64) {
	m.mu.Lock()
	m.speedUpdates = append(m.speedUpdates, struct {
		upload, download int64
	}{upload, download})
	m.mu.Unlock()
}

func TestSetCallbackAndNotifyStatus(t *testing.T) {
	mock := &mockCallback{}

	// Set callback and verify status notification arrives.
	SetCallback(mock)
	defer SetCallback(nil) // clean up

	notifyStatus("starting")
	notifyStatus("running")

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.statusChanges) != 2 {
		t.Fatalf("expected 2 status changes, got %d", len(mock.statusChanges))
	}
	if mock.statusChanges[0] != "starting" {
		t.Errorf("expected first status 'starting', got %q", mock.statusChanges[0])
	}
	if mock.statusChanges[1] != "running" {
		t.Errorf("expected second status 'running', got %q", mock.statusChanges[1])
	}
}

func TestNotifyStatusNoCallback(t *testing.T) {
	// Ensure no panic when no callback is set.
	SetCallback(nil)
	notifyStatus("running") // should not panic
}

func TestNotifyError(t *testing.T) {
	mock := &mockCallback{}
	SetCallback(mock)
	defer SetCallback(nil)

	notifyError(ErrStartFailed, "engine failed")

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(mock.errors))
	}
	if mock.errors[0].code != ErrStartFailed {
		t.Errorf("expected code %d, got %d", ErrStartFailed, mock.errors[0].code)
	}
	if mock.errors[0].message != "engine failed" {
		t.Errorf("expected message %q, got %q", "engine failed", mock.errors[0].message)
	}
}

func TestGetCallbackReturnsNilWhenUnset(t *testing.T) {
	SetCallback(nil)
	cb := getCallback()
	if cb != nil {
		t.Fatal("expected nil callback when none is set")
	}
}

func TestSetCallbackOverwrite(t *testing.T) {
	mock1 := &mockCallback{}
	mock2 := &mockCallback{}

	SetCallback(mock1)
	notifyStatus("a")

	SetCallback(mock2)
	notifyStatus("b")

	SetCallback(nil)

	mock1.mu.Lock()
	if len(mock1.statusChanges) != 1 || mock1.statusChanges[0] != "a" {
		t.Errorf("mock1 should have received only 'a', got %v", mock1.statusChanges)
	}
	mock1.mu.Unlock()

	mock2.mu.Lock()
	if len(mock2.statusChanges) != 1 || mock2.statusChanges[0] != "b" {
		t.Errorf("mock2 should have received only 'b', got %v", mock2.statusChanges)
	}
	mock2.mu.Unlock()
}

func TestCallbackConcurrency(t *testing.T) {
	// This test cannot run in parallel with other callback tests because
	// SetCallback/getCallback use a package-level variable.
	mock := &mockCallback{}
	SetCallback(mock)
	defer SetCallback(nil)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			notifyStatus("running")
		}()
	}
	wg.Wait()

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.statusChanges) != 50 {
		t.Fatalf("expected 50 status changes from concurrent calls, got %d", len(mock.statusChanges))
	}
}

// ---------------------------------------------------------------------------
// SetAutoReconnect tests
// ---------------------------------------------------------------------------

func TestSetAutoReconnectToggle(t *testing.T) {
	// Default state: autoReconnect should be false.
	mu.Lock()
	initial := autoReconnect
	mu.Unlock()

	SetAutoReconnect(true)
	mu.Lock()
	afterEnable := autoReconnect
	mu.Unlock()
	if !afterEnable {
		t.Fatal("expected autoReconnect to be true after SetAutoReconnect(true)")
	}

	SetAutoReconnect(false)
	mu.Lock()
	afterDisable := autoReconnect
	mu.Unlock()
	if afterDisable {
		t.Fatal("expected autoReconnect to be false after SetAutoReconnect(false)")
	}

	// Restore original state.
	SetAutoReconnect(initial)
}

func TestSetAutoReconnectConcurrency(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			SetAutoReconnect(n%2 == 0)
		}(i)
	}
	wg.Wait()
	// Just verify no race/panic occurred. The final state is indeterminate.
}

// ---------------------------------------------------------------------------
// Status (engine not running)
// ---------------------------------------------------------------------------

func TestStatusWhenStopped(t *testing.T) {
	t.Parallel()
	// With no engine running, Status should return a JSON string indicating stopped.
	status := Status()
	if !strings.Contains(status, "stopped") {
		t.Fatalf("expected status to contain 'stopped', got %q", status)
	}
}

func TestIsRunningWhenStopped(t *testing.T) {
	t.Parallel()
	if IsRunning() {
		t.Fatal("expected IsRunning() to be false when engine is not started")
	}
}

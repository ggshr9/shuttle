package test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ggshr9/shuttle/crypto"
	"github.com/ggshr9/shuttle/obfs"
	"github.com/ggshr9/shuttle/transport/auth"
)

// ---------------------------------------------------------------------------
// TestHMACAuthTimingConsistency: all payloads are exactly 64 bytes,
// and nonces (first 32 bytes) are unique across 1000 iterations.
// ---------------------------------------------------------------------------

func TestHMACAuthTimingConsistency(t *testing.T) {
	const iterations = 1000
	password := "timing-test-password"

	seen := make(map[[32]byte]struct{}, iterations)

	for i := 0; i < iterations; i++ {
		payload, err := auth.GenerateHMAC(password)
		if err != nil {
			t.Fatalf("iteration %d: GenerateHMAC error: %v", i, err)
		}

		// Payload must be exactly 64 bytes (32-byte nonce + 32-byte HMAC).
		if len(payload) != 64 {
			t.Fatalf("iteration %d: payload length = %d, want 64", i, len(payload))
		}

		// Check nonce uniqueness.
		var nonce [32]byte
		copy(nonce[:], payload[:32])
		if _, exists := seen[nonce]; exists {
			t.Fatalf("iteration %d: duplicate nonce detected", i)
		}
		seen[nonce] = struct{}{}
	}

	if len(seen) != iterations {
		t.Errorf("expected %d unique nonces, got %d", iterations, len(seen))
	}
}

// ---------------------------------------------------------------------------
// TestHMACBruteForceResistance: correct password verifies, 100 random
// wrong passwords all fail.
// ---------------------------------------------------------------------------

func TestHMACBruteForceResistance(t *testing.T) {
	correctPassword := "super-secret-correct-password-42"

	payload, err := auth.GenerateHMAC(correctPassword)
	if err != nil {
		t.Fatalf("GenerateHMAC: %v", err)
	}

	nonce := payload[:32]
	mac := payload[32:]

	// Correct password must verify.
	if !auth.VerifyHMAC(nonce, mac, correctPassword) {
		t.Fatal("correct password should verify")
	}

	// 100 random wrong passwords must all fail.
	for i := 0; i < 100; i++ {
		wrong := fmt.Sprintf("wrong-password-%d-%d", i, rand.Int64())
		if auth.VerifyHMAC(nonce, mac, wrong) {
			t.Fatalf("wrong password %q should not verify", wrong)
		}
	}
}

// ---------------------------------------------------------------------------
// TestReplayFilterConcurrent: submit the same nonce from 10 goroutines,
// exactly 1 must succeed (Check returns false) and 9 must be rejected.
// ---------------------------------------------------------------------------

func TestReplayFilterConcurrent(t *testing.T) {
	rf := crypto.NewReplayFilter(10 * time.Second)

	const goroutines = 10
	nonce := uint64(0xDEADBEEF)

	var accepted atomic.Int32
	var rejected atomic.Int32

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Use a channel as a starting gun so all goroutines race simultaneously.
	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			<-start
			if !rf.Check(nonce) {
				accepted.Add(1)
			} else {
				rejected.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	a := accepted.Load()
	r := rejected.Load()
	if a != 1 {
		t.Errorf("expected exactly 1 accepted, got %d", a)
	}
	if r != int32(goroutines-1) {
		t.Errorf("expected %d rejected, got %d", goroutines-1, r)
	}
}

// ---------------------------------------------------------------------------
// TestReplayFilterExpiry: the dual-buffer swap mechanism should eventually
// discard old entries after the window elapses, preventing unbounded memory.
// ---------------------------------------------------------------------------

func TestReplayFilterExpiry(t *testing.T) {
	// Use a very short window so the test completes quickly.
	// The swap happens at window/2, and after two swaps the original
	// "current" filter moves to "previous" and then gets replaced.
	window := 100 * time.Millisecond
	rf := crypto.NewReplayFilter(window)

	// Insert a batch of nonces.
	const batch = 500
	for i := uint64(0); i < batch; i++ {
		rf.Check(i)
	}

	initialSize := rf.Size()
	if initialSize < batch {
		// Cuckoo filter is probabilistic; just ensure most were inserted.
		t.Logf("initial size %d (expected ~%d, cuckoo is probabilistic)", initialSize, batch)
	}

	// Wait for two full swap intervals (window/2 each = window total)
	// plus a small margin so both swaps have occurred.
	time.Sleep(window + 50*time.Millisecond)

	// Trigger a new Check to force the internal maybeSwap.
	rf.Check(0xFFFFFFFF)

	// Wait again for the second swap.
	time.Sleep(window/2 + 50*time.Millisecond)
	rf.Check(0xFFFFFFFE)

	finalSize := rf.Size()
	// After two swaps the original entries should have been discarded.
	// The filter should now contain at most the two trigger entries
	// plus whatever leaked across the probabilistic structure.
	if finalSize >= initialSize {
		t.Errorf("expected size to decrease after expiry window; initial=%d, final=%d", initialSize, finalSize)
	}
	t.Logf("size reduced from %d to %d after expiry", initialSize, finalSize)
}

// ---------------------------------------------------------------------------
// TestSelfSignedCertProperties: generate a cert and validate its crypto
// properties — validity, key usage, key type, and minimum key size.
// ---------------------------------------------------------------------------

func TestSelfSignedCertProperties(t *testing.T) {
	validFor := 24 * time.Hour
	certPEM, keyPEM, err := crypto.GenerateSelfSignedCert(
		[]string{"test.example.com", "10.0.0.1"}, validFor,
	)
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert: %v", err)
	}

	// Parse the certificate.
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("failed to decode certificate PEM")
		return
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}

	// Not expired: NotBefore <= now <= NotAfter.
	now := time.Now()
	if now.Before(cert.NotBefore) {
		t.Errorf("certificate not yet valid: NotBefore=%v, now=%v", cert.NotBefore, now)
	}
	if now.After(cert.NotAfter) {
		t.Errorf("certificate expired: NotAfter=%v, now=%v", cert.NotAfter, now)
	}

	// Correct key usage flags.
	if cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		t.Error("missing KeyUsageDigitalSignature")
	}
	if cert.KeyUsage&x509.KeyUsageKeyEncipherment == 0 {
		t.Error("missing KeyUsageKeyEncipherment")
	}

	// Extended key usage: ServerAuth.
	foundServerAuth := false
	for _, eku := range cert.ExtKeyUsage {
		if eku == x509.ExtKeyUsageServerAuth {
			foundServerAuth = true
			break
		}
	}
	if !foundServerAuth {
		t.Error("missing ExtKeyUsageServerAuth")
	}

	// Key type must be ECDSA with P-256 (minimum 256-bit key).
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		t.Fatal("failed to decode key PEM")
		return
	}
	ecKey, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		t.Fatalf("parse EC private key: %v", err)
	}
	if ecKey.Curve != elliptic.P256() {
		t.Errorf("expected P-256 curve, got %v", ecKey.Curve.Params().Name)
	}
	bitSize := ecKey.Curve.Params().BitSize
	if bitSize < 256 {
		t.Errorf("key size %d bits is below minimum 256", bitSize)
	}

	// Verify the public key in the cert matches the private key.
	certPub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		t.Fatal("certificate public key is not ECDSA")
	}
	if !certPub.Equal(&ecKey.PublicKey) {
		t.Error("certificate public key does not match private key")
	}
}

// ---------------------------------------------------------------------------
// TestCertWatchReload: write an expiring cert, start CertWatcher, verify
// the OnRenew callback fires and the cert file is replaced.
// ---------------------------------------------------------------------------

func TestCertWatchReload(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")

	// Generate a cert that expires in 30 minutes — well within the
	// default 7-day renewBefore window, so it triggers immediate renewal.
	certPEM, keyPEM, err := crypto.GenerateSelfSignedCert([]string{"localhost"}, 30*time.Minute)
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	// Record original cert content for comparison.
	origCert := make([]byte, len(certPEM))
	copy(origCert, certPEM)

	var renewed atomic.Bool
	w := crypto.NewCertWatcher(&crypto.CertWatcherConfig{
		CertFile:      certFile,
		KeyFile:       keyFile,
		Hosts:         []string{"localhost"},
		ValidFor:      30 * 24 * time.Hour,
		RenewBefore:   7 * 24 * time.Hour,
		CheckInterval: 1 * time.Hour, // won't tick during test
		OnRenew:       func() { renewed.Store(true) },
	}, slog.Default())

	// Start triggers an immediate check+renew cycle.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	w.Start(ctx)

	// Give the background goroutine a moment to run.
	time.Sleep(200 * time.Millisecond)
	w.Stop()

	if !renewed.Load() {
		t.Fatal("expected OnRenew callback to fire for expiring cert")
	}

	// Verify the cert file was replaced with a new certificate.
	newCertPEM, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatalf("read renewed cert: %v", err)
	}
	if bytes.Equal(origCert, newCertPEM) {
		t.Error("cert file was not replaced after renewal")
	}

	// Verify the new cert has a much longer validity.
	expiry, err := crypto.CertExpiry(newCertPEM)
	if err != nil {
		t.Fatalf("parse renewed cert: %v", err)
	}
	remaining := time.Until(expiry)
	if remaining < 29*24*time.Hour {
		t.Errorf("renewed cert should be valid for ~30 days, got %v", remaining)
	}
}

// ---------------------------------------------------------------------------
// TestPaddingObfuscation: verify variable-length output and correct
// round-trip after stripping padding.
// ---------------------------------------------------------------------------

func TestPaddingObfuscation(t *testing.T) {
	p := obfs.NewPadder(0)

	// Pad the same input multiple times and collect lengths.
	input := []byte("test payload for obfuscation validation")
	const iterations = 50
	lengths := make(map[int]struct{}, iterations)

	for i := 0; i < iterations; i++ {
		padded, err := p.Pad(input)
		if err != nil {
			t.Fatalf("Pad iteration %d: %v", i, err)
		}

		lengths[len(padded)] = struct{}{}

		// Verify round-trip: unpadding must recover the original data.
		recovered, err := p.Unpad(padded)
		if err != nil {
			t.Fatalf("Unpad iteration %d: %v", i, err)
		}
		if !bytes.Equal(recovered, input) {
			t.Fatalf("iteration %d: round-trip mismatch: got %q, want %q", i, recovered, input)
		}
	}

	// With random padding targets, we expect multiple distinct lengths.
	// The Padder randomizes between minTarget and maxTarget, so different
	// iterations should produce different padded sizes.
	if len(lengths) < 2 {
		t.Errorf("expected variable padded lengths, got only %d distinct length(s): %v",
			len(lengths), lengths)
	}
	t.Logf("observed %d distinct padded lengths across %d iterations", len(lengths), iterations)

	// All padded outputs must be strictly larger than the input.
	for l := range lengths {
		if l <= len(input) {
			t.Errorf("padded length %d is not larger than input length %d", l, len(input))
		}
	}

	// Verify that the padded content is not identical across runs
	// (random padding bytes should differ).
	padded1, _ := p.Pad(input)
	padded2, _ := p.Pad(input)
	if bytes.Equal(padded1, padded2) {
		t.Error("two pads of the same input produced identical output; expected random padding bytes")
	}
}

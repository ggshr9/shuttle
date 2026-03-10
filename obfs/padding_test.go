package obfs

import (
	"bytes"
	"testing"
)

func TestPadUnpad(t *testing.T) {
	p := NewPadder(0)
	original := []byte("hello, shuttle proxy")

	padded, err := p.Pad(original)
	if err != nil {
		t.Fatalf("Pad: %v", err)
	}
	if len(padded) <= len(original) {
		t.Fatalf("padded length %d should exceed original %d", len(padded), len(original))
	}

	result, err := p.Unpad(padded)
	if err != nil {
		t.Fatalf("Unpad: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Fatalf("round-trip mismatch: got %q, want %q", result, original)
	}
}

func TestPadMinSize(t *testing.T) {
	minTarget := 500
	p := NewPadder(minTarget)
	data := []byte("short")

	padded, err := p.Pad(data)
	if err != nil {
		t.Fatalf("Pad: %v", err)
	}
	if len(padded) < minTarget {
		t.Fatalf("padded length %d < minTarget %d", len(padded), minTarget)
	}
}

func TestPadRandomness(t *testing.T) {
	p := NewPadder(0)
	data := []byte("deterministic input")

	padded1, err := p.Pad(data)
	if err != nil {
		t.Fatalf("Pad 1: %v", err)
	}
	padded2, err := p.Pad(data)
	if err != nil {
		t.Fatalf("Pad 2: %v", err)
	}

	// The random padding bytes (after headerSize+len(data)) should differ.
	// There's a vanishingly small probability this fails by chance.
	if bytes.Equal(padded1, padded2) {
		t.Fatal("two pads of the same data produced identical output — expected random padding")
	}
}

func TestUnpadInvalid(t *testing.T) {
	p := NewPadder(0)

	// Too short
	_, err := p.Unpad([]byte{0x00})
	if err == nil {
		t.Fatal("expected error for too-short frame")
	}

	// Corrupted origLen: claim origLen is larger than frame
	bad := []byte{0x00, 0x04, 0xFF, 0xFF, 0x00, 0x00}
	_, err = p.Unpad(bad)
	if err == nil {
		t.Fatal("expected error for corrupted origLen")
	}
}

func TestReadWriteFrame(t *testing.T) {
	p := NewPadder(0)
	original := []byte("frame round-trip test data")

	padded, err := p.Pad(original)
	if err != nil {
		t.Fatalf("Pad: %v", err)
	}

	// Write the padded frame into a buffer and read it back with ReadFrame
	var buf bytes.Buffer
	buf.Write(padded)

	result, err := p.ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if !bytes.Equal(result, original) {
		t.Fatalf("ReadFrame mismatch: got %q, want %q", result, original)
	}
}

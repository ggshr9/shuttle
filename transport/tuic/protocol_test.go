package tuic_test

import (
	"bytes"
	"testing"

	"github.com/shuttleX/shuttle/transport/tuic"
)

func TestParseUUID(t *testing.T) {
	tests := []struct {
		input string
		want  [tuic.UUIDLen]byte
		ok    bool
	}{
		{
			"550e8400-e29b-41d4-a716-446655440000",
			[16]byte{0x55, 0x0e, 0x84, 0x00, 0xe2, 0x9b, 0x41, 0xd4, 0xa7, 0x16, 0x44, 0x66, 0x55, 0x44, 0x00, 0x00},
			true,
		},
		{
			"550e8400e29b41d4a716446655440000",
			[16]byte{0x55, 0x0e, 0x84, 0x00, 0xe2, 0x9b, 0x41, 0xd4, 0xa7, 0x16, 0x44, 0x66, 0x55, 0x44, 0x00, 0x00},
			true,
		},
		{"short", [16]byte{}, false},
		{"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", [16]byte{}, false},
	}

	for _, tc := range tests {
		got, err := tuic.ParseUUID(tc.input)
		if tc.ok {
			if err != nil {
				t.Errorf("ParseUUID(%q): unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseUUID(%q): got %x, want %x", tc.input, got, tc.want)
			}
		} else {
			if err == nil {
				t.Errorf("ParseUUID(%q): expected error", tc.input)
			}
		}
	}
}

func TestComputeToken(t *testing.T) {
	uuid, err := tuic.ParseUUID("550e8400-e29b-41d4-a716-446655440000")
	if err != nil {
		t.Fatal(err)
	}

	token1 := tuic.ComputeToken(uuid, []byte("password"))
	token2 := tuic.ComputeToken(uuid, []byte("password"))
	if token1 != token2 {
		t.Fatal("same inputs should produce same token")
	}

	token3 := tuic.ComputeToken(uuid, []byte("different"))
	if token1 == token3 {
		t.Fatal("different passwords should produce different tokens")
	}
}

func TestAuthRequest_EncodeDecode(t *testing.T) {
	uuid, _ := tuic.ParseUUID("550e8400-e29b-41d4-a716-446655440000")
	token := tuic.ComputeToken(uuid, []byte("secret"))

	req := &tuic.AuthRequest{UUID: uuid, Token: token}
	data := req.Encode()

	if len(data) != tuic.UUIDLen+tuic.TokenLen {
		t.Fatalf("encoded length: got %d, want %d", len(data), tuic.UUIDLen+tuic.TokenLen)
	}

	decoded, err := tuic.DecodeAuth(data)
	if err != nil {
		t.Fatal(err)
	}

	if decoded.UUID != req.UUID {
		t.Errorf("UUID mismatch: got %x, want %x", decoded.UUID, req.UUID)
	}
	if decoded.Token != req.Token {
		t.Errorf("Token mismatch: got %x, want %x", decoded.Token, req.Token)
	}
}

func TestDecodeAuth_TooShort(t *testing.T) {
	_, err := tuic.DecodeAuth([]byte{1, 2, 3})
	if err == nil {
		t.Fatal("expected error for short datagram")
	}
}

func TestConnectHeader_IPv4(t *testing.T) {
	var buf bytes.Buffer
	if err := tuic.EncodeConnectHeader(&buf, tuic.CmdConnect, "127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}

	cmd, addr, err := tuic.DecodeConnectHeader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if cmd != tuic.CmdConnect {
		t.Errorf("cmd: got 0x%02x, want 0x%02x", cmd, tuic.CmdConnect)
	}
	if addr != "127.0.0.1:8080" {
		t.Errorf("addr: got %q, want %q", addr, "127.0.0.1:8080")
	}
}

func TestConnectHeader_IPv6(t *testing.T) {
	var buf bytes.Buffer
	if err := tuic.EncodeConnectHeader(&buf, tuic.CmdConnect, "[::1]:443"); err != nil {
		t.Fatal(err)
	}

	cmd, addr, err := tuic.DecodeConnectHeader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if cmd != tuic.CmdConnect {
		t.Errorf("cmd: got 0x%02x, want 0x%02x", cmd, tuic.CmdConnect)
	}
	if addr != "[::1]:443" {
		t.Errorf("addr: got %q, want %q", addr, "[::1]:443")
	}
}

func TestConnectHeader_Domain(t *testing.T) {
	var buf bytes.Buffer
	if err := tuic.EncodeConnectHeader(&buf, tuic.CmdConnect, "example.com:443"); err != nil {
		t.Fatal(err)
	}

	cmd, addr, err := tuic.DecodeConnectHeader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if cmd != tuic.CmdConnect {
		t.Errorf("cmd: got 0x%02x, want 0x%02x", cmd, tuic.CmdConnect)
	}
	if addr != "example.com:443" {
		t.Errorf("addr: got %q, want %q", addr, "example.com:443")
	}
}

func TestConnectHeader_UDPAssoc(t *testing.T) {
	var buf bytes.Buffer
	if err := tuic.EncodeConnectHeader(&buf, tuic.CmdUDPAssoc, "10.0.0.1:53"); err != nil {
		t.Fatal(err)
	}

	cmd, addr, err := tuic.DecodeConnectHeader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if cmd != tuic.CmdUDPAssoc {
		t.Errorf("cmd: got 0x%02x, want 0x%02x", cmd, tuic.CmdUDPAssoc)
	}
	if addr != "10.0.0.1:53" {
		t.Errorf("addr: got %q, want %q", addr, "10.0.0.1:53")
	}
}

func TestDecodeConnectHeader_BadVersion(t *testing.T) {
	// version=0x04 instead of 0x05
	data := []byte{0x04, 0x01, 0x01, 127, 0, 0, 1, 0x1F, 0x90}
	_, _, err := tuic.DecodeConnectHeader(bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error for bad version")
	}
}

func TestDecodeConnectHeader_Truncated(t *testing.T) {
	_, _, err := tuic.DecodeConnectHeader(bytes.NewReader([]byte{0x05, 0x01}))
	if err == nil {
		t.Fatal("expected error for truncated header")
	}
}

func TestConnectHeader_AllCommands(t *testing.T) {
	cmds := []byte{tuic.CmdConnect, tuic.CmdUDPAssoc, tuic.CmdDissociate, tuic.CmdHeartbeat}
	for _, cmd := range cmds {
		var buf bytes.Buffer
		if err := tuic.EncodeConnectHeader(&buf, cmd, "1.2.3.4:80"); err != nil {
			t.Fatalf("cmd 0x%02x: encode: %v", cmd, err)
		}
		gotCmd, _, err := tuic.DecodeConnectHeader(&buf)
		if err != nil {
			t.Fatalf("cmd 0x%02x: decode: %v", cmd, err)
		}
		if gotCmd != cmd {
			t.Errorf("cmd: got 0x%02x, want 0x%02x", gotCmd, cmd)
		}
	}
}

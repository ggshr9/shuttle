package config

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate test key: %v", err)
	}
	return key
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := testKey(t)
	original := "super-secret-password-123!"

	encrypted, err := Encrypt(original, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	if !IsEncrypted(encrypted) {
		t.Fatalf("encrypted value missing ENC: prefix: %q", encrypted)
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if decrypted != original {
		t.Errorf("round-trip mismatch: got %q, want %q", decrypted, original)
	}
}

func TestEncryptedValueYAML(t *testing.T) {
	key := testKey(t)

	// Set up keystore with test key.
	dir := t.TempDir()
	keyPath := filepath.Join(dir, keyFileName)
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	ResetKeyStore()
	defer ResetKeyStore()
	if err := InitKeyStore(dir); err != nil {
		t.Fatalf("InitKeyStore: %v", err)
	}

	// Create a client config with a plaintext password.
	cfg := DefaultClientConfig()
	cfg.Server.Addr = "example.com:443"
	cfg.Server.Password = "my-secret"
	cfg.Transport.CDN.Enabled = false     // avoid validation requiring CDN domain
	cfg.Transport.Reality.Enabled = false // avoid validation requiring Reality public key

	// Save (encrypts) and reload (decrypts).
	cfgPath := filepath.Join(dir, "client.yaml")
	if err := SaveClientConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveClientConfig: %v", err)
	}

	// Read raw YAML and check the password is encrypted on disk.
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	var onDisk ClientConfig
	if err := yaml.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if !IsEncrypted(onDisk.Server.Password) {
		t.Errorf("on-disk password should be encrypted, got %q", onDisk.Server.Password)
	}

	// Load config (should auto-decrypt).
	loaded, err := LoadClientConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}
	if loaded.Server.Password != "my-secret" {
		t.Errorf("loaded password = %q, want %q", loaded.Server.Password, "my-secret")
	}

	// Verify original config was not mutated.
	if cfg.Server.Password != "my-secret" {
		t.Errorf("original config mutated: password = %q", cfg.Server.Password)
	}
}

func TestBackwardCompat(t *testing.T) {
	key := testKey(t)

	// A plaintext value should pass through DecryptIfEncrypted unchanged.
	plain := "plaintext-password"
	result, err := DecryptIfEncrypted(plain, key)
	if err != nil {
		t.Fatalf("DecryptIfEncrypted: %v", err)
	}
	if result != plain {
		t.Errorf("got %q, want %q", result, plain)
	}
}

func TestBackwardCompatLoad(t *testing.T) {
	// A config file with plaintext passwords should load fine even with
	// keystore initialized.
	dir := t.TempDir()
	keyPath := filepath.Join(dir, keyFileName)
	key := testKey(t)
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	ResetKeyStore()
	defer ResetKeyStore()
	if err := InitKeyStore(dir); err != nil {
		t.Fatalf("InitKeyStore: %v", err)
	}

	yamlContent := `
version: 1
server:
  addr: "example.com:443"
  password: "plain-text-pass"
transport:
  preferred: auto
routing:
  default: proxy
`
	cfgPath := filepath.Join(dir, "client.yaml")
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadClientConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}
	if cfg.Server.Password != "plain-text-pass" {
		t.Errorf("password = %q, want %q", cfg.Server.Password, "plain-text-pass")
	}
}

func TestIsEncrypted(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"ENC:abc123", true},
		{"ENC:", true},
		{"enc:abc", false},
		{"plaintext", false},
		{"", false},
		{"ENC", false},
	}
	for _, tt := range tests {
		if got := IsEncrypted(tt.input); got != tt.want {
			t.Errorf("IsEncrypted(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestKeyStoreInitAndGet(t *testing.T) {
	dir := t.TempDir()

	ResetKeyStore()
	defer ResetKeyStore()

	// First init should create a key file.
	if err := InitKeyStore(dir); err != nil {
		t.Fatalf("InitKeyStore: %v", err)
	}

	key1, err := GetKey()
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	if len(key1) != 32 {
		t.Fatalf("key length = %d, want 32", len(key1))
	}

	// Verify key file exists with correct permissions.
	keyPath := filepath.Join(dir, keyFileName)
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("key file perms = %o, want 0600", info.Mode().Perm())
	}

	// Reset and re-init should load the same key.
	ResetKeyStore()
	if err := InitKeyStore(dir); err != nil {
		t.Fatalf("second InitKeyStore: %v", err)
	}
	key2, err := GetKey()
	if err != nil {
		t.Fatalf("second GetKey: %v", err)
	}

	if string(key1) != string(key2) {
		t.Error("key changed after re-init from same file")
	}
}

func TestGetKeyWithoutInit(t *testing.T) {
	ResetKeyStore()
	defer ResetKeyStore()

	_, err := GetKey()
	if err == nil {
		t.Fatal("GetKey should fail without InitKeyStore")
	}
}

func TestEncryptDifferentNonce(t *testing.T) {
	key := testKey(t)
	plaintext := "same-input"

	enc1, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("first Encrypt: %v", err)
	}
	enc2, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("second Encrypt: %v", err)
	}

	if enc1 == enc2 {
		t.Error("two encryptions of the same plaintext produced identical ciphertext; nonce should differ")
	}

	// Both should decrypt to the same value.
	dec1, _ := Decrypt(enc1, key)
	dec2, _ := Decrypt(enc2, key)
	if dec1 != plaintext || dec2 != plaintext {
		t.Errorf("decryption failed: got %q and %q", dec1, dec2)
	}
}

func TestDecryptTampered(t *testing.T) {
	key := testKey(t)
	encrypted, err := Encrypt("secret", key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Tamper with the ciphertext (flip a character in the base64 portion).
	tampered := encrypted[:len(encrypted)-2] + "XX"

	_, err = Decrypt(tampered, key)
	if err == nil {
		t.Fatal("Decrypt of tampered ciphertext should fail")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := testKey(t)
	key2 := testKey(t)

	encrypted, err := Encrypt("secret", key1)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	_, err = Decrypt(encrypted, key2)
	if err == nil {
		t.Fatal("Decrypt with wrong key should fail")
	}
}

func TestEncryptBadKeySize(t *testing.T) {
	_, err := Encrypt("test", []byte("short-key"))
	if err == nil {
		t.Fatal("Encrypt with short key should fail")
	}
}

func TestServerConfigEncryptRoundTrip(t *testing.T) {
	dir := t.TempDir()
	key := testKey(t)
	keyPath := filepath.Join(dir, keyFileName)
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	ResetKeyStore()
	defer ResetKeyStore()
	if err := InitKeyStore(dir); err != nil {
		t.Fatalf("InitKeyStore: %v", err)
	}

	cfg := DefaultServerConfig()
	cfg.Auth.Password = "server-pass"
	cfg.Auth.PrivateKey = "deadbeef1234"
	cfg.Admin.Token = "admin-token-xyz"
	cfg.Cluster.Secret = "cluster-secret"

	cfgPath := filepath.Join(dir, "server.yaml")
	if err := SaveServerConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveServerConfig: %v", err)
	}

	// Verify on-disk values are encrypted.
	raw, _ := os.ReadFile(cfgPath)
	var onDisk ServerConfig
	yaml.Unmarshal(raw, &onDisk)
	if !IsEncrypted(onDisk.Auth.Password) {
		t.Errorf("on-disk auth.password should be encrypted")
	}
	if !IsEncrypted(onDisk.Auth.PrivateKey) {
		t.Errorf("on-disk auth.private_key should be encrypted")
	}
	if !IsEncrypted(onDisk.Admin.Token) {
		t.Errorf("on-disk admin.token should be encrypted")
	}

	// Verify original config is not mutated.
	if cfg.Auth.Password != "server-pass" {
		t.Errorf("original config mutated")
	}

	// Reload and verify decryption.
	loaded, err := LoadServerConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadServerConfig: %v", err)
	}
	if loaded.Auth.Password != "server-pass" {
		t.Errorf("loaded password = %q, want %q", loaded.Auth.Password, "server-pass")
	}
	if loaded.Auth.PrivateKey != "deadbeef1234" {
		t.Errorf("loaded private_key = %q, want %q", loaded.Auth.PrivateKey, "deadbeef1234")
	}
	if loaded.Admin.Token != "admin-token-xyz" {
		t.Errorf("loaded admin.token = %q, want %q", loaded.Admin.Token, "admin-token-xyz")
	}
}

func TestEncryptEmptyFields(t *testing.T) {
	key := testKey(t)

	cfg := &ClientConfig{}
	// All sensitive fields are empty — should not fail.
	if err := encryptSensitiveFields(cfg, key); err != nil {
		t.Fatalf("encryptSensitiveFields with empty fields: %v", err)
	}
	if cfg.Server.Password != "" {
		t.Errorf("empty password should stay empty, got %q", cfg.Server.Password)
	}
}

func TestEncryptAlreadyEncrypted(t *testing.T) {
	key := testKey(t)
	original := "my-pass"

	enc, err := Encrypt(original, key)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &ClientConfig{}
	cfg.Server.Password = enc

	// Should not double-encrypt.
	if err := encryptSensitiveFields(cfg, key); err != nil {
		t.Fatalf("encryptSensitiveFields: %v", err)
	}
	if cfg.Server.Password != enc {
		t.Errorf("already-encrypted value was modified")
	}
}

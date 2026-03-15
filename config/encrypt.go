package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
)

const encPrefix = "ENC:"

// IsEncrypted returns true if the string has the "ENC:" prefix indicating
// it is an AES-256-GCM encrypted value.
func IsEncrypted(s string) bool {
	return strings.HasPrefix(s, encPrefix)
}

// Encrypt encrypts plaintext using AES-256-GCM and returns a string with
// the "ENC:" prefix followed by base64-encoded nonce+ciphertext.
// The key must be exactly 32 bytes (AES-256).
func Encrypt(plaintext string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	return encPrefix + encoded, nil
}

// Decrypt decrypts a value produced by Encrypt. It expects the "ENC:" prefix
// followed by base64-encoded nonce+ciphertext. Returns the original plaintext.
func Decrypt(encoded string, key []byte) (string, error) {
	if !IsEncrypted(encoded) {
		return "", fmt.Errorf("value does not have %s prefix", encPrefix)
	}
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
	}

	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, encPrefix))
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}

// DecryptIfEncrypted decrypts the value if it has the "ENC:" prefix,
// otherwise returns it as-is. This provides backward compatibility
// with plaintext config values.
func DecryptIfEncrypted(value string, key []byte) (string, error) {
	if !IsEncrypted(value) {
		return value, nil
	}
	return Decrypt(value, key)
}

// encryptFields encrypts a list of string fields in place using AES-256-GCM.
// Fields that are empty or already encrypted (have "ENC:" prefix) are skipped.
func encryptFields(fields []*string, key []byte) error {
	for _, f := range fields {
		if *f == "" || IsEncrypted(*f) {
			continue
		}
		enc, err := Encrypt(*f, key)
		if err != nil {
			return err
		}
		*f = enc
	}
	return nil
}

// decryptFields decrypts a list of string fields in place.
// Plaintext values (no "ENC:" prefix) are left unchanged for backward compat.
func decryptFields(fields []*string, key []byte) error {
	for _, f := range fields {
		v, err := DecryptIfEncrypted(*f, key)
		if err != nil {
			return err
		}
		*f = v
	}
	return nil
}

// clientSensitiveFields returns pointers to all sensitive string fields in a client config.
func clientSensitiveFields(cfg *ClientConfig) []*string {
	fields := []*string{
		&cfg.Server.Password,
	}
	for i := range cfg.Servers {
		fields = append(fields, &cfg.Servers[i].Password)
	}
	fields = append(fields, &cfg.Transport.WebRTC.TURNPass)
	return fields
}

// serverSensitiveFields returns pointers to all sensitive string fields in a server config.
func serverSensitiveFields(cfg *ServerConfig) []*string {
	fields := []*string{
		&cfg.Auth.Password,
		&cfg.Auth.PrivateKey,
		&cfg.Admin.Token,
		&cfg.Cluster.Secret,
	}
	for i := range cfg.Admin.Users {
		fields = append(fields, &cfg.Admin.Users[i].Token)
	}
	fields = append(fields, &cfg.Transport.WebRTC.TURNPass)
	return fields
}

// encryptSensitiveFields encrypts sensitive string fields in a client config.
// Fields that are already encrypted (have "ENC:" prefix) are left unchanged.
// Empty fields are left unchanged.
func encryptSensitiveFields(cfg *ClientConfig, key []byte) error {
	return encryptFields(clientSensitiveFields(cfg), key)
}

// decryptSensitiveFields decrypts sensitive string fields in a client config.
// Plaintext values (no "ENC:" prefix) are left unchanged for backward compat.
func decryptSensitiveFields(cfg *ClientConfig, key []byte) error {
	return decryptFields(clientSensitiveFields(cfg), key)
}

// encryptServerSensitiveFields encrypts sensitive fields in a server config.
func encryptServerSensitiveFields(cfg *ServerConfig, key []byte) error {
	return encryptFields(serverSensitiveFields(cfg), key)
}

// decryptServerSensitiveFields decrypts sensitive fields in a server config.
func decryptServerSensitiveFields(cfg *ServerConfig, key []byte) error {
	return decryptFields(serverSensitiveFields(cfg), key)
}

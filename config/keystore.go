package config

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	keyFileName = "encrypt.key"
	keySize     = 32 // AES-256
)

// KeyStore manages the master encryption key for config encryption at rest.
type KeyStore struct {
	key []byte
}

var (
	globalKeyStore *KeyStore
	keyStoreMu     sync.Mutex
)

// InitKeyStore loads or generates the master encryption key from the given
// data directory. The key is stored in dataDir/encrypt.key with 0600
// permissions. If the key file does not exist, a new 32-byte random key
// is generated via crypto/rand. This function is safe to call multiple times;
// subsequent calls are no-ops if the key is already loaded.
func InitKeyStore(dataDir string) error {
	keyStoreMu.Lock()
	defer keyStoreMu.Unlock()

	if globalKeyStore != nil {
		return nil
	}

	keyPath := filepath.Join(dataDir, keyFileName)

	key, err := loadOrCreateKey(keyPath)
	if err != nil {
		return fmt.Errorf("init keystore: %w", err)
	}

	globalKeyStore = &KeyStore{key: key}
	return nil
}

// GetKey returns the master encryption key. InitKeyStore must be called first.
func GetKey() ([]byte, error) {
	keyStoreMu.Lock()
	defer keyStoreMu.Unlock()

	if globalKeyStore == nil {
		return nil, fmt.Errorf("keystore not initialized; call InitKeyStore first")
	}

	// Return a copy to prevent external mutation.
	cp := make([]byte, len(globalKeyStore.key))
	copy(cp, globalKeyStore.key)
	return cp, nil
}

// ResetKeyStore clears the global keystore. This is primarily intended for
// testing and should not be called in production code.
func ResetKeyStore() {
	keyStoreMu.Lock()
	defer keyStoreMu.Unlock()
	globalKeyStore = nil
}

func loadOrCreateKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		if len(data) != keySize {
			return nil, fmt.Errorf("key file %s has invalid size %d (expected %d)", path, len(data), keySize)
		}
		return data, nil
	}

	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read key file: %w", err)
	}

	// Key does not exist — generate a new one.
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create key directory: %w", err)
	}

	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate random key: %w", err)
	}

	if err := os.WriteFile(path, key, 0600); err != nil {
		return nil, fmt.Errorf("write key file: %w", err)
	}

	return key, nil
}

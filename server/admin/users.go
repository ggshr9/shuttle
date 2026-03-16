package admin

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"sync/atomic"

	"github.com/shuttleX/shuttle/config"
)

// UserStore manages users and tracks per-user traffic.
type UserStore struct {
	mu    sync.RWMutex
	users map[string]*UserState // token → state
}

// UserState tracks a single user's runtime state.
type UserState struct {
	config.User
	BytesSent   atomic.Int64
	BytesRecv   atomic.Int64
	ActiveConns atomic.Int64
}

// TotalBytes returns the sum of bytes sent and received.
func (u *UserState) TotalBytes() int64 {
	return u.BytesSent.Load() + u.BytesRecv.Load()
}

// QuotaExceeded returns true if the user has a quota (MaxBytes > 0) and
// total traffic has reached or exceeded it.
func (u *UserState) QuotaExceeded() bool {
	return u.MaxBytes > 0 && u.TotalBytes() >= u.MaxBytes
}

// NewUserStore creates a store from config users.
func NewUserStore(users []config.User) *UserStore {
	s := &UserStore{users: make(map[string]*UserState, len(users))}
	for _, u := range users {
		s.users[u.Token] = &UserState{User: u}
	}
	return s
}

// HasUsers returns true if any users are configured.
func (s *UserStore) HasUsers() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.users) > 0
}

// Authenticate returns the user for a token, or nil if not found/disabled.
func (s *UserStore) Authenticate(token string) *UserState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u := s.users[token]
	if u == nil || !u.Enabled {
		return nil
	}
	return u
}

// Add adds a new user. Returns the generated token.
func (s *UserStore) Add(name string, maxBytes int64) (config.User, error) {
	token, err := generateToken()
	if err != nil {
		return config.User{}, err
	}
	u := config.User{
		Name:     name,
		Token:    token,
		MaxBytes: maxBytes,
		Enabled:  true,
	}
	s.mu.Lock()
	s.users[token] = &UserState{User: u}
	s.mu.Unlock()
	return u, nil
}

// Remove removes a user by token.
func (s *UserStore) Remove(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[token]; !ok {
		return false
	}
	delete(s.users, token)
	return true
}

// List returns all users with their traffic stats.
func (s *UserStore) List() []UserInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]UserInfo, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, UserInfo{
			Name:        u.Name,
			Token:       u.Token,
			MaxBytes:    u.MaxBytes,
			Enabled:     u.Enabled,
			BytesSent:   u.BytesSent.Load(),
			BytesRecv:   u.BytesRecv.Load(),
			ActiveConns: u.ActiveConns.Load(),
		})
	}
	return out
}

// SetEnabled enables or disables a user.
func (s *UserStore) SetEnabled(token string, enabled bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.users[token]
	if u == nil {
		return false
	}
	u.Enabled = enabled
	return true
}

// ReplaceAll clears all users and replaces them with the given list.
func (s *UserStore) ReplaceAll(users []config.User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users = make(map[string]*UserState, len(users))
	for _, u := range users {
		s.users[u.Token] = &UserState{User: u}
	}
}

// ToConfig returns the current users as config entries.
func (s *UserStore) ToConfig() []config.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]config.User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u.User)
	}
	return out
}

// UserInfo is a JSON-serializable user snapshot.
type UserInfo struct {
	Name        string `json:"name"`
	Token       string `json:"token"`
	MaxBytes    int64  `json:"max_bytes"`
	Enabled     bool   `json:"enabled"`
	BytesSent   int64  `json:"bytes_sent"`
	BytesRecv   int64  `json:"bytes_recv"`
	ActiveConns int64  `json:"active_conns"`
}

func generateToken() (string, error) {
	b := make([]byte, 32) // 256-bit token
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

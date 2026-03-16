package subscription

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/shuttleX/shuttle/config"
)

// Subscription represents a subscription source.
type Subscription struct {
	ID        string                   `json:"id"`
	Name      string                   `json:"name"`
	URL       string                   `json:"url"`
	Servers   []config.ServerEndpoint  `json:"servers"`
	UpdatedAt time.Time                `json:"updated_at"`
	Error     string                   `json:"error,omitempty"`
}

// Manager handles subscription management.
type Manager struct {
	mu            sync.RWMutex
	subscriptions map[string]*Subscription
	client        *http.Client

	autoMu      sync.Mutex
	autoCancel  context.CancelFunc
	autoRunning bool
}

// NewManager creates a new subscription manager.
func NewManager() *Manager {
	return &Manager{
		subscriptions: make(map[string]*Subscription),
		client: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) > 5 {
					return fmt.Errorf("too many redirects (max 5)")
				}
				return nil
			},
		},
	}
}

// Add adds a new subscription.
func (m *Manager) Add(name, url string) (*Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := generateID()
	sub := &Subscription{
		ID:   id,
		Name: name,
		URL:  url,
	}
	m.subscriptions[id] = sub
	return sub, nil
}

// Remove removes a subscription.
func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.subscriptions[id]; !ok {
		return fmt.Errorf("subscription not found: %s", id)
	}
	delete(m.subscriptions, id)
	return nil
}

// Get returns a subscription by ID.
func (m *Manager) Get(id string) (*Subscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sub, ok := m.subscriptions[id]
	if !ok {
		return nil, fmt.Errorf("subscription not found: %s", id)
	}
	return sub, nil
}

// List returns all subscriptions.
func (m *Manager) List() []*Subscription {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*Subscription, 0, len(m.subscriptions))
	for _, sub := range m.subscriptions {
		list = append(list, sub)
	}
	return list
}

// Refresh fetches and parses a subscription.
func (m *Manager) Refresh(ctx context.Context, id string) (*Subscription, error) {
	m.mu.RLock()
	sub, ok := m.subscriptions[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("subscription not found: %s", id)
	}

	servers, err := m.fetch(ctx, sub.URL)

	m.mu.Lock()
	defer m.mu.Unlock()

	if err != nil {
		sub.Error = err.Error()
		sub.UpdatedAt = time.Now()
		return sub, err
	}

	sub.Servers = servers
	sub.Error = ""
	sub.UpdatedAt = time.Now()
	return sub, nil
}

// RefreshAll refreshes all subscriptions.
func (m *Manager) RefreshAll(ctx context.Context) {
	m.mu.RLock()
	ids := make([]string, 0, len(m.subscriptions))
	for id := range m.subscriptions {
		ids = append(ids, id)
	}
	m.mu.RUnlock()

	for _, id := range ids {
		m.Refresh(ctx, id)
	}
}

// fetch downloads and parses a subscription URL.
func (m *Manager) fetch(ctx context.Context, url string) ([]config.ServerEndpoint, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", "Shuttle/1.0")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return ParseSubscription(string(body))
}

// LoadFromConfig loads subscriptions from config.
func (m *Manager) LoadFromConfig(subs []config.SubscriptionConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, sc := range subs {
		if sc.ID == "" {
			sc.ID = generateID()
		}
		m.subscriptions[sc.ID] = &Subscription{
			ID:   sc.ID,
			Name: sc.Name,
			URL:  sc.URL,
		}
	}
}

// ToConfig exports subscriptions to config format.
func (m *Manager) ToConfig() []config.SubscriptionConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	configs := make([]config.SubscriptionConfig, 0, len(m.subscriptions))
	for _, sub := range m.subscriptions {
		configs = append(configs, config.SubscriptionConfig{
			ID:   sub.ID,
			Name: sub.Name,
			URL:  sub.URL,
		})
	}
	return configs
}

// StartAutoRefresh starts a background goroutine that periodically calls RefreshAll.
// Only one auto-refresh goroutine runs at a time; calling StartAutoRefresh again
// stops the previous goroutine before starting a new one.
func (m *Manager) StartAutoRefresh(ctx context.Context, interval time.Duration) {
	m.autoMu.Lock()
	defer m.autoMu.Unlock()

	// Stop existing goroutine if running.
	if m.autoCancel != nil {
		m.autoCancel()
	}

	autoCtx, cancel := context.WithCancel(ctx)
	m.autoCancel = cancel
	m.autoRunning = true

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-autoCtx.Done():
				m.autoMu.Lock()
				m.autoRunning = false
				m.autoMu.Unlock()
				return
			case <-ticker.C:
				m.RefreshAll(autoCtx)
			}
		}
	}()
}

// StopAutoRefresh stops the auto-refresh goroutine if one is running.
func (m *Manager) StopAutoRefresh() {
	m.autoMu.Lock()
	defer m.autoMu.Unlock()

	if m.autoCancel != nil {
		m.autoCancel()
		m.autoCancel = nil
	}
}

// GetAllServers returns all servers from all subscriptions.
func (m *Manager) GetAllServers() []config.ServerEndpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var all []config.ServerEndpoint
	for _, sub := range m.subscriptions {
		all = append(all, sub.Servers...)
	}
	return all
}

// ParseSubscription parses subscription content.
// Supports: base64, JSON, SIP008, shuttle:// URIs
func ParseSubscription(content string) ([]config.ServerEndpoint, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("empty content")
	}

	// Try base64 decode first
	if decoded, err := base64.StdEncoding.DecodeString(content); err == nil {
		content = string(decoded)
	} else if decoded, err := base64.RawStdEncoding.DecodeString(content); err == nil {
		content = string(decoded)
	}

	// Try SIP008 format (subscription-specific)
	if servers, err := parseSIP008(content); err == nil {
		return servers, nil
	}

	// Delegate to common import logic for shuttle://, JSON, etc.
	result, err := config.ImportConfig(content)
	if err != nil {
		return nil, err
	}
	return result.Servers, nil
}

// parseSIP008 parses SIP008 subscription format.
func parseSIP008(data string) ([]config.ServerEndpoint, error) {
	var sip008 struct {
		Version int `json:"version"`
		Servers []struct {
			Server     string `json:"server"`
			ServerPort int    `json:"server_port"`
			Password   string `json:"password"`
			Method     string `json:"method"`
			Remarks    string `json:"remarks"`
		} `json:"servers"`
	}
	if err := json.Unmarshal([]byte(data), &sip008); err != nil {
		return nil, err
	}
	if len(sip008.Servers) == 0 {
		return nil, fmt.Errorf("no servers in SIP008")
	}
	servers := make([]config.ServerEndpoint, 0, len(sip008.Servers))
	for _, s := range sip008.Servers {
		servers = append(servers, config.ServerEndpoint{
			Addr:     fmt.Sprintf("%s:%d", s.Server, s.ServerPort),
			Name:     s.Remarks,
			Password: s.Password,
		})
	}
	return servers, nil
}

var idCounter int64
var idMu sync.Mutex

func generateID() string {
	idMu.Lock()
	defer idMu.Unlock()
	idCounter++
	return fmt.Sprintf("sub_%d_%d", time.Now().UnixNano(), idCounter)
}

# Sub-1: Subscription System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Multi-format subscription parsing (Clash YAML, sing-box JSON, Shuttle native), scheduled + CB-triggered updates, auto-grouping with server speed testing.

**Architecture:** SubscriptionManager coordinates fetch/parse/apply. Parser interface enables format auto-detection. ServerNode is the unified intermediate representation. CB-triggered update is wired into CircuitBreaker's OnStateChange callback.

**Tech Stack:** Go 1.24+, `gopkg.in/yaml.v3`, `encoding/json`, existing OutboundGroup + ProxyOutbound factory

---

## File Structure

- Create: `subscription/manager.go` — SubscriptionManager (fetch, update, apply)
- Create: `subscription/parser.go` — Parser interface + auto-detect
- Create: `subscription/parser_clash.go` — Clash YAML parser
- Create: `subscription/parser_singbox.go` — sing-box JSON parser
- Create: `subscription/parser_shuttle.go` — Shuttle native YAML parser
- Create: `subscription/node.go` — ServerNode unified type
- Create: `subscription/speedtest.go` — server latency testing
- Test files for each
- Modify: `engine/engine.go` — add SubscriptionManager
- Modify: `engine/engine_lifecycle.go` — wire auto-update + CB trigger
- Modify: `config/config.go` — enhance SubscriptionConfig

---

## Task 1: ServerNode + Parser Interface

**Files:**
- Create: `subscription/node.go`
- Create: `subscription/parser.go`
- Create: `subscription/node_test.go`

- [ ] **Step 1: Define ServerNode**

```go
// subscription/node.go
package subscription

// ServerNode is the unified intermediate representation for a proxy server.
type ServerNode struct {
    Name      string         `json:"name"`
    Server    string         `json:"server"`     // host:port
    Transport string         `json:"transport"`  // h3, reality, cdn, webrtc, ss, vmess, trojan
    Group     string         `json:"group"`      // region/purpose grouping
    Settings  map[string]any `json:"settings"`   // transport-specific params
}
```

- [ ] **Step 2: Define Parser interface**

```go
// subscription/parser.go
package subscription

// Parser parses subscription data into ServerNodes.
type Parser interface {
    // CanParse returns true if the data appears to be in this format.
    CanParse(data []byte) bool
    // Parse extracts server nodes from the data.
    Parse(data []byte) ([]ServerNode, error)
}

// AutoParse tries all registered parsers and uses the first that matches.
func AutoParse(data []byte) ([]ServerNode, error) {
    parsers := []Parser{
        &ShuttleParser{},
        &ClashParser{},
        &SingboxParser{},
    }
    for _, p := range parsers {
        if p.CanParse(data) {
            return p.Parse(data)
        }
    }
    return nil, fmt.Errorf("unrecognized subscription format")
}
```

- [ ] **Step 3: Test, commit**

Commit: `feat(subscription): add ServerNode type and Parser interface with auto-detect`

---

## Task 2: Clash YAML Parser

**Files:**
- Create: `subscription/parser_clash.go`
- Create: `subscription/parser_clash_test.go`

- [ ] **Step 1: Implement Clash parser**

Parse Clash YAML format:
```yaml
proxies:
  - name: "US-01"
    type: ss
    server: us1.example.com
    port: 443
    cipher: aes-256-gcm
    password: "xxx"
  - name: "JP-01"  
    type: trojan
    server: jp1.example.com
    port: 443
    password: "yyy"

proxy-groups:
  - name: "Auto"
    type: url-test
    proxies: ["US-01", "JP-01"]
```

**CanParse**: Check for `proxies:` key in YAML.

**Parse**: Extract proxies into ServerNodes. Map Clash types to Shuttle transports where possible (ss→generic, trojan→generic, vmess→generic). For types Shuttle doesn't natively support (ss, vmess, trojan), store full config in `Settings` for potential future protocol plugins.

**Mapping**:
- Clash `proxy.server` + `proxy.port` → `ServerNode.Server` = "host:port"
- Clash `proxy.name` → `ServerNode.Name`
- Clash `proxy-group.name` → `ServerNode.Group` for member proxies
- Clash `proxy.type` → `ServerNode.Transport` (with mapping table)

- [ ] **Step 2: Write comprehensive tests with real Clash YAML samples**

- [ ] **Step 3: Commit**

Commit: `feat(subscription): add Clash YAML subscription parser`

---

## Task 3: sing-box JSON Parser

**Files:**
- Create: `subscription/parser_singbox.go`
- Create: `subscription/parser_singbox_test.go`

- [ ] **Step 1: Implement sing-box parser**

Parse sing-box JSON format:
```json
{
  "outbounds": [
    {"type": "shadowsocks", "tag": "us-01", "server": "us1.example.com", "server_port": 443, ...},
    {"type": "trojan", "tag": "jp-01", "server": "jp1.example.com", "server_port": 443, ...}
  ]
}
```

**CanParse**: Check for `"outbounds"` key in JSON with array value.

**Parse**: Extract outbounds into ServerNodes.

- [ ] **Step 2: Test, commit**

Commit: `feat(subscription): add sing-box JSON subscription parser`

---

## Task 4: Shuttle Native Parser

**Files:**
- Create: `subscription/parser_shuttle.go`
- Create: `subscription/parser_shuttle_test.go`

- [ ] **Step 1: Implement Shuttle parser**

Parse Shuttle's own YAML format — essentially a list of outbound configs:
```yaml
version: 1
nodes:
  - name: "US H3"
    server: "us.example.com:443"
    transport: h3
    group: "US"
  - name: "JP Reality"
    server: "jp.example.com:443"
    transport: reality
    group: "JP"
    settings:
      public_key: "abc123"
```

**CanParse**: Check for `nodes:` or `version:` key.

- [ ] **Step 2: Test, commit**

Commit: `feat(subscription): add Shuttle native YAML parser`

---

## Task 5: SubscriptionManager

**Files:**
- Create: `subscription/manager.go`
- Create: `subscription/manager_test.go`
- Modify: `config/config.go` — enhance SubscriptionConfig

- [ ] **Step 1: Enhance config**

```go
type SubscriptionConfig struct {
    ID             string `yaml:"id" json:"id"`
    Name           string `yaml:"name" json:"name"`
    URL            string `yaml:"url" json:"url"`
    UpdateInterval string `yaml:"update_interval" json:"update_interval"` // e.g., "6h", default "24h"
    AutoUpdate     bool   `yaml:"auto_update" json:"auto_update"`         // default true
}
```

- [ ] **Step 2: Implement SubscriptionManager**

```go
type SubscriptionManager struct {
    subscriptions []config.SubscriptionConfig
    nodes         []ServerNode  // latest parsed nodes
    mu            sync.RWMutex
    logger        *slog.Logger
    onUpdate      func([]ServerNode) // callback when nodes change
    httpClient    *http.Client
}

func NewSubscriptionManager(subs []config.SubscriptionConfig, logger *slog.Logger) *SubscriptionManager
func (sm *SubscriptionManager) Start(ctx context.Context)     // starts periodic update loops
func (sm *SubscriptionManager) ForceUpdate(ctx context.Context) error  // immediate update (for CB trigger)
func (sm *SubscriptionManager) Nodes() []ServerNode            // current nodes
func (sm *SubscriptionManager) Stop()
```

**Fetch logic**:
1. HTTP GET subscription URL (follow up to 5 redirects, as per existing subscription security)
2. AutoParse response body
3. Merge nodes from all subscriptions
4. Call onUpdate callback if nodes changed

**Periodic update**: goroutine per subscription with `time.Ticker(updateInterval)`

- [ ] **Step 3: Test with mock HTTP server, commit**

Commit: `feat(subscription): add SubscriptionManager with periodic fetch and auto-parse`

---

## Task 6: Wire into Engine + CB Trigger

**Files:**
- Modify: `engine/engine.go` — add subscriptionManager field
- Modify: `engine/engine_lifecycle.go` — start SubscriptionManager, wire CB trigger
- Modify: `engine/engine_inbound.go` — apply subscription nodes as outbounds

- [ ] **Step 1: Add to Engine and start in startInternal**

```go
// In startInternal, after building outbounds:
if len(cfgSnap.Subscriptions) > 0 {
    sm := subscription.NewSubscriptionManager(cfgSnap.Subscriptions, e.logger)
    sm.OnUpdate(func(nodes []subscription.ServerNode) {
        // Convert nodes to outbound configs and reload
        e.applySubscriptionNodes(nodes)
    })
    sm.Start(ctx)
    e.subscriptionManager = sm
}
```

- [ ] **Step 2: Wire CB trigger**

In startInternal where CircuitBreaker is created, add to OnStateChange:

```go
e.circuitBreaker = NewCircuitBreaker(CircuitBreakerConfig{
    OnStateChange: func(state CircuitState, cooldown time.Duration) {
        if state == CircuitOpen {
            e.obs.Emit(Event{Type: EventCircuitBreaker, ...})
            // Trigger subscription update when all connections fail
            if sm := e.subscriptionManager; sm != nil {
                go sm.ForceUpdate(context.Background())
            }
        }
    },
})
```

- [ ] **Step 3: Implement applySubscriptionNodes**

Convert `[]ServerNode` into `[]config.OutboundConfig` + `[]config.OutboundConfig{type:"group"}` and apply via Engine.Reload or direct outbound rebuild.

- [ ] **Step 4: Run all tests, commit**

Commit: `feat(engine): wire SubscriptionManager with CB-triggered smart update`

---

## Task 7: Speed Test

**Files:**
- Create: `subscription/speedtest.go`
- Create: `subscription/speedtest_test.go`

- [ ] **Step 1: Implement latency test**

```go
// SpeedTest measures connection latency to a server node.
func SpeedTest(ctx context.Context, node ServerNode, timeout time.Duration) (time.Duration, error) {
    start := time.Now()
    conn, err := net.DialTimeout("tcp", node.Server, timeout)
    if err != nil {
        return 0, err
    }
    conn.Close()
    return time.Since(start), nil
}

// SpeedTestAll tests all nodes in parallel, returns sorted by latency.
func SpeedTestAll(ctx context.Context, nodes []ServerNode, timeout time.Duration, concurrency int) []SpeedResult {
    // Semaphore-limited parallel testing
}

type SpeedResult struct {
    Node    ServerNode
    Latency time.Duration
    Error   error
}
```

- [ ] **Step 2: Test, commit**

Commit: `feat(subscription): add server speed test with parallel execution`

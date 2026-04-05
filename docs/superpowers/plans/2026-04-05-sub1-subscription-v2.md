# Sub-1: Subscription System Enhancement Plan (v2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Clash YAML + sing-box JSON format support to the existing subscription system, wire subscription servers into the outbound system, add CB-triggered smart update, and server speed test.

**Existing infrastructure** (NOT to be rewritten):
- `subscription/subscription.go` — Manager with Add/Remove/Refresh/RefreshAll/Auto-refresh
- `subscription/subscription.go` — ParseSubscription() with base64, SIP008, shuttle:// support
- `config.ServerEndpoint{Addr, Name, Password, SNI}` — existing server representation
- `gui/api/routes_subscription.go` — REST API for subscription CRUD

**Architecture:** Extend ParseSubscription() with Clash + sing-box parsers. Add a converter from ServerEndpoint[] to OutboundConfig[]. Wire into Engine for CB-triggered updates.

**Tech Stack:** Go 1.24+, `gopkg.in/yaml.v3`, existing subscription.Manager

---

## File Structure

- Create: `subscription/parser_clash.go` — Clash YAML parser
- Create: `subscription/parser_clash_test.go`
- Create: `subscription/parser_singbox.go` — sing-box JSON parser
- Create: `subscription/parser_singbox_test.go`
- Create: `subscription/converter.go` — ServerEndpoint → OutboundConfig converter
- Create: `subscription/converter_test.go`
- Create: `subscription/speedtest.go` — server latency testing
- Create: `subscription/speedtest_test.go`
- Modify: `subscription/subscription.go` — integrate new parsers into ParseSubscription()
- Modify: `engine/engine_lifecycle.go` — wire CB-triggered ForceUpdate
- Modify: `engine/engine.go` — add subscriptionManager field

---

## Task 1: Clash YAML Parser

**Files:**
- Create: `subscription/parser_clash.go`
- Create: `subscription/parser_clash_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestParseClash_Basic(t *testing.T) {
    data := `
proxies:
  - name: "US-01"
    type: ss
    server: us1.example.com
    port: 443
    cipher: aes-256-gcm
    password: "testpass"
  - name: "JP-01"
    type: trojan
    server: jp1.example.com
    port: 443
    password: "testpass2"
    sni: jp1.example.com
`
    servers, err := parseClash([]byte(data))
    if err != nil { t.Fatal(err) }
    if len(servers) != 2 { t.Fatalf("got %d servers", len(servers)) }
    if servers[0].Name != "US-01" { t.Errorf("name = %q", servers[0].Name) }
    if servers[0].Addr != "us1.example.com:443" { t.Errorf("addr = %q", servers[0].Addr) }
}

func TestIsClashFormat(t *testing.T) {
    clash := []byte("proxies:\n  - name: test\n")
    notClash := []byte(`{"outbounds":[]}`)
    if !isClashFormat(clash) { t.Error("should detect clash") }
    if isClashFormat(notClash) { t.Error("should not detect JSON as clash") }
}
```

- [ ] **Step 2: Implement Clash parser**

```go
// subscription/parser_clash.go
package subscription

import (
    "fmt"
    "github.com/shuttleX/shuttle/config"
    "gopkg.in/yaml.v3"
)

// clashConfig represents the subset of Clash YAML we parse.
type clashConfig struct {
    Proxies []clashProxy `yaml:"proxies"`
}

type clashProxy struct {
    Name     string `yaml:"name"`
    Type     string `yaml:"type"`
    Server   string `yaml:"server"`
    Port     int    `yaml:"port"`
    Password string `yaml:"password"`
    Cipher   string `yaml:"cipher"`
    SNI      string `yaml:"sni"`
    // Add more fields as needed for specific proxy types
}

func isClashFormat(data []byte) bool {
    // Check if it looks like Clash YAML (has "proxies:" key)
    var probe struct {
        Proxies []any `yaml:"proxies"`
    }
    if err := yaml.Unmarshal(data, &probe); err != nil {
        return false
    }
    return len(probe.Proxies) > 0
}

func parseClash(data []byte) ([]config.ServerEndpoint, error) {
    var cc clashConfig
    if err := yaml.Unmarshal(data, &cc); err != nil {
        return nil, fmt.Errorf("clash parse: %w", err)
    }
    if len(cc.Proxies) == 0 {
        return nil, fmt.Errorf("clash: no proxies found")
    }
    servers := make([]config.ServerEndpoint, 0, len(cc.Proxies))
    for _, p := range cc.Proxies {
        servers = append(servers, config.ServerEndpoint{
            Addr:     fmt.Sprintf("%s:%d", p.Server, p.Port),
            Name:     p.Name,
            Password: p.Password,
            SNI:      p.SNI,
        })
    }
    return servers, nil
}
```

- [ ] **Step 3: Run tests, commit**

Run: `./scripts/test.sh --run TestParseClash --pkg ./subscription/`
Commit: `feat(subscription): add Clash YAML format parser`

---

## Task 2: sing-box JSON Parser

**Files:**
- Create: `subscription/parser_singbox.go`
- Create: `subscription/parser_singbox_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestParseSingbox_Basic(t *testing.T) {
    data := `{
        "outbounds": [
            {"type":"shadowsocks","tag":"us-01","server":"us1.example.com","server_port":443,"method":"aes-256-gcm","password":"testpass"},
            {"type":"trojan","tag":"jp-01","server":"jp1.example.com","server_port":443,"password":"testpass2"}
        ]
    }`
    servers, err := parseSingbox([]byte(data))
    if err != nil { t.Fatal(err) }
    if len(servers) != 2 { t.Fatalf("got %d", len(servers)) }
    if servers[0].Name != "us-01" { t.Errorf("name = %q", servers[0].Name) }
    if servers[0].Addr != "us1.example.com:443" { t.Errorf("addr = %q", servers[0].Addr) }
}

func TestIsSingboxFormat(t *testing.T) {
    sb := []byte(`{"outbounds":[{"type":"ss"}]}`)
    notSb := []byte("proxies:\n  - name: test")
    if !isSingboxFormat(sb) { t.Error("should detect singbox") }
    if isSingboxFormat(notSb) { t.Error("should not detect YAML as singbox") }
}
```

- [ ] **Step 2: Implement sing-box parser**

```go
// subscription/parser_singbox.go
package subscription

import (
    "encoding/json"
    "fmt"
    "github.com/shuttleX/shuttle/config"
)

type singboxConfig struct {
    Outbounds []singboxOutbound `json:"outbounds"`
}

type singboxOutbound struct {
    Type       string `json:"type"`
    Tag        string `json:"tag"`
    Server     string `json:"server"`
    ServerPort int    `json:"server_port"`
    Password   string `json:"password"`
    Method     string `json:"method"`
    TLS        *struct {
        ServerName string `json:"server_name"`
    } `json:"tls"`
}

func isSingboxFormat(data []byte) bool {
    var probe struct {
        Outbounds []json.RawMessage `json:"outbounds"`
    }
    if err := json.Unmarshal(data, &probe); err != nil {
        return false
    }
    return len(probe.Outbounds) > 0
}

func parseSingbox(data []byte) ([]config.ServerEndpoint, error) {
    var sc singboxConfig
    if err := json.Unmarshal(data, &sc); err != nil {
        return nil, fmt.Errorf("singbox parse: %w", err)
    }
    // Filter out non-proxy outbounds (direct, block, dns, selector, urltest)
    skip := map[string]bool{"direct": true, "block": true, "dns": true, "selector": true, "urltest": true}
    var servers []config.ServerEndpoint
    for _, ob := range sc.Outbounds {
        if skip[ob.Type] || ob.Server == "" {
            continue
        }
        sni := ""
        if ob.TLS != nil {
            sni = ob.TLS.ServerName
        }
        servers = append(servers, config.ServerEndpoint{
            Addr:     fmt.Sprintf("%s:%d", ob.Server, ob.ServerPort),
            Name:     ob.Tag,
            Password: ob.Password,
            SNI:      sni,
        })
    }
    if len(servers) == 0 {
        return nil, fmt.Errorf("singbox: no proxy outbounds found")
    }
    return servers, nil
}
```

- [ ] **Step 3: Run tests, commit**

Run: `./scripts/test.sh --run TestParseSingbox --pkg ./subscription/`
Commit: `feat(subscription): add sing-box JSON format parser`

---

## Task 3: Integrate Parsers into ParseSubscription

**Files:**
- Modify: `subscription/subscription.go` — update ParseSubscription()

- [ ] **Step 1: Update ParseSubscription**

After base64 decode and before SIP008, add Clash and sing-box detection:

```go
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

    raw := []byte(content)

    // Try Clash YAML format
    if isClashFormat(raw) {
        return parseClash(raw)
    }

    // Try sing-box JSON format
    if isSingboxFormat(raw) {
        return parseSingbox(raw)
    }

    // Try SIP008 format
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
```

- [ ] **Step 2: Test auto-detection**

```go
func TestParseSubscription_ClashAutoDetect(t *testing.T) {
    data := "proxies:\n  - name: test\n    type: ss\n    server: 1.2.3.4\n    port: 443\n    password: pass\n    cipher: aes-256-gcm\n"
    servers, err := ParseSubscription(data)
    if err != nil { t.Fatal(err) }
    if len(servers) != 1 { t.Fatalf("got %d", len(servers)) }
}

func TestParseSubscription_SingboxAutoDetect(t *testing.T) {
    data := `{"outbounds":[{"type":"shadowsocks","tag":"test","server":"1.2.3.4","server_port":443,"password":"pass","method":"aes-256-gcm"}]}`
    servers, err := ParseSubscription(data)
    if err != nil { t.Fatal(err) }
    if len(servers) != 1 { t.Fatalf("got %d", len(servers)) }
}
```

- [ ] **Step 3: Run tests, commit**

Run: `./scripts/test.sh --pkg ./subscription/`
Commit: `feat(subscription): integrate Clash + sing-box parsers into auto-detection`

---

## Task 4: ServerEndpoint → OutboundConfig Converter

**Files:**
- Create: `subscription/converter.go`
- Create: `subscription/converter_test.go`

- [ ] **Step 1: Implement converter**

```go
// subscription/converter.go
package subscription

import (
    "encoding/json"
    "fmt"
    "strings"

    "github.com/shuttleX/shuttle/config"
)

// ToOutboundConfigs converts ServerEndpoints to OutboundConfigs for the engine.
// Each server becomes a "proxy" type outbound with the server address in options.
func ToOutboundConfigs(servers []config.ServerEndpoint) []config.OutboundConfig {
    var outbounds []config.OutboundConfig
    for _, s := range servers {
        tag := sanitizeTag(s.Name)
        if tag == "" {
            tag = sanitizeTag(s.Addr)
        }
        opts, _ := json.Marshal(map[string]string{
            "server": s.Addr,
        })
        outbounds = append(outbounds, config.OutboundConfig{
            Tag:     tag,
            Type:    "proxy",
            Options: opts,
        })
    }
    return deduplicateTags(outbounds)
}

// ToGroupConfig creates an OutboundConfig for a quality-based group of all subscription servers.
func ToGroupConfig(tag string, outbounds []config.OutboundConfig) config.OutboundConfig {
    tags := make([]string, len(outbounds))
    for i, ob := range outbounds {
        tags[i] = ob.Tag
    }
    opts, _ := json.Marshal(map[string]any{
        "strategy":  "quality",
        "outbounds": tags,
        "max_latency": "500ms",
        "max_loss_rate": 0.05,
    })
    return config.OutboundConfig{
        Tag:     tag,
        Type:    "group",
        Options: opts,
    }
}

func sanitizeTag(s string) string {
    s = strings.ReplaceAll(s, " ", "-")
    s = strings.ReplaceAll(s, ":", "-")
    s = strings.ToLower(s)
    return s
}

func deduplicateTags(outbounds []config.OutboundConfig) []config.OutboundConfig {
    seen := make(map[string]int)
    for i := range outbounds {
        tag := outbounds[i].Tag
        if n, ok := seen[tag]; ok {
            outbounds[i].Tag = fmt.Sprintf("%s-%d", tag, n+1)
            seen[tag] = n + 1
        } else {
            seen[tag] = 1
        }
    }
    return outbounds
}
```

- [ ] **Step 2: Test, commit**

Commit: `feat(subscription): add ServerEndpoint to OutboundConfig converter`

---

## Task 5: Wire into Engine + CB Trigger

**Files:**
- Modify: `engine/engine.go` — add subscriptionManager field
- Modify: `engine/engine_lifecycle.go` — start subscription manager, wire CB trigger

- [ ] **Step 1: Add field and wire in startInternal**

```go
// In engine.go
subscriptionManager *subscription.Manager

// In startInternal, after everything else starts:
if len(cfgSnap.Subscriptions) > 0 {
    sm := subscription.NewManager()
    sm.LoadFromConfig(cfgSnap.Subscriptions)
    sm.StartAutoRefresh(ctx, 24*time.Hour) // default daily
    e.subscriptionManager = sm
}
```

- [ ] **Step 2: Wire CB trigger**

In the CircuitBreaker OnStateChange callback, add:

```go
if state == CircuitOpen && e.subscriptionManager != nil {
    go e.subscriptionManager.RefreshAll(context.Background())
}
```

- [ ] **Step 3: Clean up in stopInternal**

```go
if e.subscriptionManager != nil {
    e.subscriptionManager.StopAutoRefresh()
    e.subscriptionManager = nil
}
```

- [ ] **Step 4: Run tests, commit**

Commit: `feat(engine): wire subscription manager with CB-triggered smart update`

---

## Task 6: Speed Test

**Files:**
- Create: `subscription/speedtest.go`
- Create: `subscription/speedtest_test.go`

- [ ] **Step 1: Implement**

```go
package subscription

import (
    "context"
    "net"
    "sort"
    "sync"
    "time"

    "github.com/shuttleX/shuttle/config"
)

type SpeedResult struct {
    Server  config.ServerEndpoint
    Latency time.Duration
    Error   error
}

// SpeedTestAll tests all servers in parallel with limited concurrency.
func SpeedTestAll(ctx context.Context, servers []config.ServerEndpoint, timeout time.Duration, concurrency int) []SpeedResult {
    if concurrency <= 0 {
        concurrency = 10
    }
    results := make([]SpeedResult, len(servers))
    sem := make(chan struct{}, concurrency)
    var wg sync.WaitGroup

    for i, s := range servers {
        wg.Add(1)
        go func(idx int, srv config.ServerEndpoint) {
            defer wg.Done()
            sem <- struct{}{}
            defer func() { <-sem }()

            start := time.Now()
            conn, err := net.DialTimeout("tcp", srv.Addr, timeout)
            if err != nil {
                results[idx] = SpeedResult{Server: srv, Error: err}
                return
            }
            conn.Close()
            results[idx] = SpeedResult{Server: srv, Latency: time.Since(start)}
        }(i, s)
    }
    wg.Wait()

    // Sort by latency (errors last)
    sort.Slice(results, func(i, j int) bool {
        if results[i].Error != nil { return false }
        if results[j].Error != nil { return true }
        return results[i].Latency < results[j].Latency
    })
    return results
}
```

- [ ] **Step 2: Test with localhost listener, commit**

Commit: `feat(subscription): add server speed test with parallel execution`

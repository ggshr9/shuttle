package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/shuttleX/shuttle/provider"
)

// ── Info types for API responses ─────────────────────────────────────────────

// GroupInfo describes an outbound group's state for the API layer.
type GroupInfo struct {
	Tag      string            `json:"tag"`
	Strategy string            `json:"strategy"`
	Members  []string          `json:"members"`
	Selected string            `json:"selected,omitempty"`
	Latencies map[string]int64 `json:"latencies,omitempty"` // tag → latency in ms
}

// ProviderInfo describes a provider's state for the API layer.
type ProviderInfo struct {
	Name      string    `json:"name"`
	Type      string    `json:"type"` // "proxy" or "rule"
	NodeCount int       `json:"node_count,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	Error     string    `json:"error,omitempty"`
}

// TestResult holds the health-check result for a single outbound member.
type TestResult struct {
	Tag       string `json:"tag"`
	Latency   int64  `json:"latency_ms"` // -1 means unreachable
	Available bool   `json:"available"`
}

// ── Group API methods ────────────────────────────────────────────────────────

// ListGroups returns info for every OutboundGroup registered in the engine.
func (e *Engine) ListGroups() []GroupInfo {
	e.mu.RLock()
	outbounds := e.outbounds
	e.mu.RUnlock()

	groups := make([]GroupInfo, 0, len(outbounds))
	for _, ob := range outbounds {
		grp, ok := ob.(*OutboundGroup)
		if !ok {
			continue
		}
		groups = append(groups, groupToInfo(grp))
	}
	return groups
}

// GetGroup returns info for a single OutboundGroup by tag.
func (e *Engine) GetGroup(tag string) (GroupInfo, error) {
	grp, err := e.findGroup(tag)
	if err != nil {
		return GroupInfo{}, err
	}
	return groupToInfo(grp), nil
}

// SelectGroupOutbound selects an outbound in a select-strategy group.
func (e *Engine) SelectGroupOutbound(groupTag, outboundTag string) error {
	grp, err := e.findGroup(groupTag)
	if err != nil {
		return err
	}
	return grp.SelectOutbound(outboundTag)
}

// TestGroup triggers an immediate health check for the group and returns results.
func (e *Engine) TestGroup(tag string) ([]TestResult, error) {
	grp, err := e.findGroup(tag)
	if err != nil {
		return nil, err
	}

	// If the group has a url-test state with a checker, use its results.
	if grp.urlTest != nil && grp.urlTest.checker != nil {
		// Run a fresh check.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		grp.urlTest.checkAll(ctx, grp.outbounds)

		results := grp.urlTest.checker.Results()
		var out []TestResult
		for _, ob := range grp.outbounds {
			r, ok := results[ob.Tag()]
			tr := TestResult{Tag: ob.Tag(), Latency: -1}
			if ok {
				tr.Available = r.Available
				if r.Available {
					tr.Latency = r.Latency.Milliseconds()
				}
			}
			out = append(out, tr)
		}
		return out, nil
	}

	// For non-url-test groups, return basic member list with unknown latency.
	out := make([]TestResult, 0, len(grp.outbounds))
	for _, ob := range grp.outbounds {
		out = append(out, TestResult{Tag: ob.Tag(), Latency: -1, Available: true})
	}
	return out, nil
}

// findGroup looks up an OutboundGroup by tag in the engine's outbounds map.
func (e *Engine) findGroup(tag string) (*OutboundGroup, error) {
	e.mu.RLock()
	outbounds := e.outbounds
	e.mu.RUnlock()

	if outbounds == nil {
		return nil, fmt.Errorf("engine not running")
	}
	ob, ok := outbounds[tag]
	if !ok {
		return nil, fmt.Errorf("group %q not found", tag)
	}
	grp, ok := ob.(*OutboundGroup)
	if !ok {
		return nil, fmt.Errorf("%q is not a group", tag)
	}
	return grp, nil
}

// groupToInfo converts an OutboundGroup to GroupInfo.
func groupToInfo(grp *OutboundGroup) GroupInfo {
	info := GroupInfo{
		Tag:      grp.tag,
		Strategy: string(grp.strategy),
	}
	for _, ob := range grp.outbounds {
		info.Members = append(info.Members, ob.Tag())
	}

	// Report selected outbound based on strategy.
	switch grp.strategy {
	case GroupSelect:
		info.Selected = grp.SelectedOutbound()
	case GroupURLTest:
		info.Selected = grp.URLTestSelected()
	}

	// Include latencies from url-test checker if available.
	if grp.urlTest != nil && grp.urlTest.checker != nil {
		results := grp.urlTest.checker.Results()
		latencies := make(map[string]int64, len(results))
		for tag, r := range results {
			if r.Available {
				latencies[tag] = r.Latency.Milliseconds()
			} else {
				latencies[tag] = -1
			}
		}
		if len(latencies) > 0 {
			info.Latencies = latencies
		}
	}

	return info
}

// ── Provider API methods ─────────────────────────────────────────────────────

// refreshableProvider is the shared interface satisfied by both ProxyProvider and RuleProvider.
type refreshableProvider interface {
	Name() string
	Refresh(ctx context.Context) error
	UpdatedAt() time.Time
	Error() error
}

// listProviders returns ProviderInfo for a slice of refreshable providers.
func listProviders[T refreshableProvider](providers []T, toInfo func(T) ProviderInfo) []ProviderInfo {
	out := make([]ProviderInfo, 0, len(providers))
	for _, p := range providers {
		out = append(out, toInfo(p))
	}
	return out
}

// refreshProvider finds and refreshes a named provider from the slice.
func refreshProvider[T refreshableProvider](providers []T, kind, name string, ctx context.Context) error {
	for _, p := range providers {
		if p.Name() == name {
			return p.Refresh(ctx)
		}
	}
	return fmt.Errorf("%s provider %q not found", kind, name)
}

// ListProxyProviders returns info for all proxy providers.
func (e *Engine) ListProxyProviders() []ProviderInfo {
	e.mu.RLock()
	providers := e.proxyProviders
	e.mu.RUnlock()
	return listProviders(providers, proxyProviderToInfo)
}

// RefreshProxyProvider triggers a manual refresh of the named proxy provider.
func (e *Engine) RefreshProxyProvider(ctx context.Context, name string) error {
	e.mu.RLock()
	providers := e.proxyProviders
	e.mu.RUnlock()
	return refreshProvider(providers, "proxy", name, ctx)
}

// ListRuleProviders returns info for all rule providers.
func (e *Engine) ListRuleProviders() []ProviderInfo {
	e.mu.RLock()
	providers := e.ruleProviders
	e.mu.RUnlock()
	return listProviders(providers, ruleProviderToInfo)
}

// RefreshRuleProvider triggers a manual refresh of the named rule provider.
func (e *Engine) RefreshRuleProvider(ctx context.Context, name string) error {
	e.mu.RLock()
	providers := e.ruleProviders
	e.mu.RUnlock()
	return refreshProvider(providers, "rule", name, ctx)
}

// proxyProviderToInfo converts a ProxyProvider to ProviderInfo.
func proxyProviderToInfo(p *provider.ProxyProvider) ProviderInfo {
	info := ProviderInfo{
		Name:      p.Name(),
		Type:      "proxy",
		NodeCount: len(p.Nodes()),
		UpdatedAt: p.UpdatedAt(),
	}
	if err := p.Error(); err != nil {
		info.Error = err.Error()
	}
	return info
}

// ruleProviderToInfo converts a RuleProvider to ProviderInfo.
func ruleProviderToInfo(p *provider.RuleProvider) ProviderInfo {
	info := ProviderInfo{
		Name:      p.Name(),
		Type:      "rule",
		UpdatedAt: p.UpdatedAt(),
	}
	if err := p.Error(); err != nil {
		info.Error = err.Error()
	}
	return info
}

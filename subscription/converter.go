package subscription

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shuttleX/shuttle/config"
)

// sanitizeTag converts a name to a valid outbound tag:
// lowercase, spaces and colons replaced with dashes.
func sanitizeTag(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, ":", "-")
	return s
}

// ToOutboundConfigs converts ServerEndpoints to OutboundConfigs.
// Each server becomes a "proxy" type outbound with server address in options.
// Tags are derived from the server name (sanitized), falling back to the addr.
// Duplicate tags are deduplicated by appending "-2", "-3", etc.
func ToOutboundConfigs(servers []config.ServerEndpoint) []config.OutboundConfig {
	out := make([]config.OutboundConfig, 0, len(servers))
	seen := make(map[string]int, len(servers))

	for _, s := range servers {
		base := sanitizeTag(s.Name)
		if base == "" {
			base = sanitizeTag(s.Addr)
		}

		tag := base
		if count, ok := seen[base]; ok {
			// count tracks how many times we have already emitted this base.
			// The first duplicate becomes "-2", the next "-3", etc.
			count++
			seen[base] = count
			tag = fmt.Sprintf("%s-%d", base, count)
		} else {
			seen[base] = 1
		}

		opts, _ := json.Marshal(map[string]string{"server": s.Addr})

		out = append(out, config.OutboundConfig{
			Tag:     tag,
			Type:    "proxy",
			Options: json.RawMessage(opts),
		})
	}

	return out
}

// groupOptions is the JSON structure written into a group OutboundConfig.
type groupOptions struct {
	Strategy   string   `json:"strategy"`
	Outbounds  []string `json:"outbounds"`
	MaxLatency string   `json:"max_latency"`
	MaxLossRate float64  `json:"max_loss_rate"`
}

// ToGroupConfig creates a quality-group OutboundConfig wrapping all given outbounds.
func ToGroupConfig(tag string, outbounds []config.OutboundConfig) config.OutboundConfig {
	tags := make([]string, len(outbounds))
	for i, ob := range outbounds {
		tags[i] = ob.Tag
	}

	opts, _ := json.Marshal(groupOptions{
		Strategy:    "quality",
		Outbounds:   tags,
		MaxLatency:  "500ms",
		MaxLossRate: 0.05,
	})

	return config.OutboundConfig{
		Tag:     tag,
		Type:    "group",
		Options: json.RawMessage(opts),
	}
}

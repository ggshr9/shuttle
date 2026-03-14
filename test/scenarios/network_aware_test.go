package scenarios

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/shuttle-proxy/shuttle/internal/netmon"
	"github.com/shuttle-proxy/shuttle/router"
	"github.com/shuttle-proxy/shuttle/testkit/vnet"
)

// ---------------------------------------------------------------------------
// TestNetworkTypeRoutingSwitch
//
// Creates a router with network-type-specific rules and verifies that
// switching the network type changes routing decisions:
// - WiFi: proxy domains go direct (save bandwidth on fast WiFi)
// - Cellular: everything proxied (censorship evasion priority)
// ---------------------------------------------------------------------------

func TestNetworkTypeRoutingSwitch(t *testing.T) {
	t.Parallel()

	r := router.NewRouter(&router.RouterConfig{
		Rules: []router.Rule{
			// WiFi rule: proxy domains go direct to save bandwidth.
			{Type: "domain", Values: []string{"youtube.com", "netflix.com", "+.cdn.example.com"}, Action: router.ActionDirect, NetworkType: "wifi"},
			// Cellular rule: everything goes through proxy for censorship evasion.
			{Type: "domain", Values: []string{"+.example.com", "+.google.com", "+.cdn.example.com"}, Action: router.ActionProxy, NetworkType: "cellular"},
			// Regular (non-network-type) rule: some domains always direct.
			{Type: "domain", Values: []string{"localhost.internal"}, Action: router.ActionDirect},
		},
		DefaultAction: router.ActionProxy,
	}, nil, nil, nil)

	// --- No network type set: network-type rules should not match ---
	t.Run("no_network_type", func(t *testing.T) {
		got := r.Match("youtube.com", nil, "", "")
		if got != router.ActionProxy {
			t.Errorf("Match(youtube.com) without network type = %q, want %q", got, router.ActionProxy)
		}
		got = r.Match("localhost.internal", nil, "", "")
		if got != router.ActionDirect {
			t.Errorf("Match(localhost.internal) without network type = %q, want %q", got, router.ActionDirect)
		}
	})

	// --- WiFi: proxy domains should go direct ---
	t.Run("wifi", func(t *testing.T) {
		r.SetNetworkType("wifi")

		cases := []struct {
			domain string
			want   router.Action
		}{
			{"youtube.com", router.ActionDirect},
			{"netflix.com", router.ActionDirect},
			{"sub.cdn.example.com", router.ActionDirect},
			{"unknown-domain.org", router.ActionProxy}, // no wifi rule, falls to default
			{"localhost.internal", router.ActionDirect}, // regular rule still works
		}
		for _, tc := range cases {
			got := r.Match(tc.domain, nil, "", "")
			if got != tc.want {
				t.Errorf("WiFi Match(%q) = %q, want %q", tc.domain, got, tc.want)
			}
		}
	})

	// --- Cellular: everything proxied ---
	t.Run("cellular", func(t *testing.T) {
		r.SetNetworkType("cellular")

		cases := []struct {
			domain string
			want   router.Action
		}{
			{"sub.example.com", router.ActionProxy},
			{"mail.google.com", router.ActionProxy},
			{"sub.cdn.example.com", router.ActionProxy}, // cellular rule overrides
			{"youtube.com", router.ActionProxy},          // no cellular rule for this exact domain, falls to default
			{"localhost.internal", router.ActionDirect},   // regular rule still works
		}
		for _, tc := range cases {
			got := r.Match(tc.domain, nil, "", "")
			if got != tc.want {
				t.Errorf("Cellular Match(%q) = %q, want %q", tc.domain, got, tc.want)
			}
		}
	})

	// --- Switch back to WiFi ---
	t.Run("switch_back_to_wifi", func(t *testing.T) {
		r.SetNetworkType("wifi")
		got := r.Match("youtube.com", nil, "", "")
		if got != router.ActionDirect {
			t.Errorf("After switch back to WiFi, Match(youtube.com) = %q, want %q", got, router.ActionDirect)
		}
	})

	// --- Clear network type ---
	t.Run("clear_network_type", func(t *testing.T) {
		r.SetNetworkType("")
		got := r.Match("youtube.com", nil, "", "")
		if got != router.ActionProxy {
			t.Errorf("After clearing network type, Match(youtube.com) = %q, want %q", got, router.ActionProxy)
		}
	})
}

// ---------------------------------------------------------------------------
// TestNetworkTypeWithVnetHandoff
//
// Combines network type routing with vnet:
// - Set up phone->server link on WiFi preset
// - Router configured with wifi-specific rules
// - Switch link to LTE, update router network type
// - Verify routing decision changes affect traffic flow
// ---------------------------------------------------------------------------

func TestNetworkTypeWithVnetHandoff(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := NewEnv(t, vnet.WithSeed(300))
	defer env.Close()

	phone := env.Net.AddNode("phone")
	server := env.Net.AddNode("server")

	// Start on WiFi.
	env.Net.Link(phone, server, vnet.WiFi())

	srv := newVnetServer(env.Net, server, "h3", "server:5000")
	if err := srv.Listen(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	echoServer(ctx, t, srv)

	// Create router with network-type rules.
	rtr := router.NewRouter(&router.RouterConfig{
		Rules: []router.Rule{
			// On WiFi: streaming goes direct.
			{Type: "domain", Values: []string{"streaming.example.com"}, Action: router.ActionDirect, NetworkType: "wifi"},
			// On cellular: streaming goes through proxy.
			{Type: "domain", Values: []string{"streaming.example.com"}, Action: router.ActionProxy, NetworkType: "cellular"},
		},
		DefaultAction: router.ActionProxy,
	}, nil, nil, nil)

	// Helper: dial and echo through vnet, checking router decision.
	dialAndVerify := func(domain string, expectAction router.Action) {
		t.Helper()
		action := rtr.Match(domain, nil, "", "")
		if action != expectAction {
			t.Fatalf("router Match(%q) = %q, want %q", domain, action, expectAction)
		}

		// Verify the vnet link works by sending data through.
		ct := newVnetClient(env.Net, phone, "h3")
		ct.dialFn = func(ctx context.Context, addr string) (net.Conn, error) {
			return env.Net.Dial(ctx, phone, "server:5000")
		}
		conn, err := ct.Dial(ctx, "server:5000")
		if err != nil {
			t.Fatalf("Dial: %v", err)
		}
		defer conn.Close()

		stream, err := conn.OpenStream(ctx)
		if err != nil {
			t.Fatalf("OpenStream: %v", err)
		}
		defer stream.Close()

		payload := []byte("handoff-test-" + domain)
		if _, err := stream.Write(payload); err != nil {
			t.Fatalf("Write: %v", err)
		}
		buf := make([]byte, len(payload))
		if _, err := io.ReadFull(stream, buf); err != nil {
			t.Fatalf("ReadFull: %v", err)
		}
		if !bytes.Equal(buf, payload) {
			t.Fatalf("echo mismatch: got %q, want %q", buf, payload)
		}
	}

	// Phase 1: WiFi — streaming goes direct.
	rtr.SetNetworkType("wifi")
	dialAndVerify("streaming.example.com", router.ActionDirect)
	t.Log("Phase 1 (WiFi): streaming.example.com -> direct, vnet echo OK")

	// Phase 2: Handoff blip then LTE.
	env.Net.UpdateLink(phone, server, vnet.HandoffBlip())
	env.Net.UpdateLink(server, phone, vnet.HandoffBlip())
	env.Net.UpdateLink(phone, server, vnet.LTE())
	env.Net.UpdateLink(server, phone, vnet.LTE())

	// Update router to cellular.
	rtr.SetNetworkType("cellular")
	dialAndVerify("streaming.example.com", router.ActionProxy)
	t.Log("Phase 2 (LTE/Cellular): streaming.example.com -> proxy, vnet echo OK")

	// Phase 3: Back to WiFi.
	env.Net.UpdateLink(phone, server, vnet.WiFi())
	env.Net.UpdateLink(server, phone, vnet.WiFi())
	rtr.SetNetworkType("wifi")
	dialAndVerify("streaming.example.com", router.ActionDirect)
	t.Log("Phase 3 (WiFi again): streaming.example.com -> direct, vnet echo OK")
}

// ---------------------------------------------------------------------------
// TestNetworkTypePriorityOverRegularRules
//
// Network-type rules should have higher priority than regular rules.
// - Regular rule says domain X goes "direct"
// - Network-type rule says when cellular, domain X goes "proxy"
// - Verify cellular overrides the regular rule
// ---------------------------------------------------------------------------

func TestNetworkTypePriorityOverRegularRules(t *testing.T) {
	t.Parallel()

	r := router.NewRouter(&router.RouterConfig{
		Rules: []router.Rule{
			// Regular rule: example.com goes direct.
			{Type: "domain", Values: []string{"example.com"}, Action: router.ActionDirect},
			// Network-type rule: on cellular, example.com goes proxy.
			{Type: "domain", Values: []string{"example.com"}, Action: router.ActionProxy, NetworkType: "cellular"},
			// Network-type rule: on cellular, secure.example.com gets rejected.
			{Type: "domain", Values: []string{"secure.example.com"}, Action: router.ActionReject, NetworkType: "cellular"},
			// Regular rule: secure.example.com goes direct.
			{Type: "domain", Values: []string{"secure.example.com"}, Action: router.ActionDirect},
		},
		DefaultAction: router.ActionProxy,
	}, nil, nil, nil)

	// Without network type: regular rules apply.
	t.Run("no_network_type_uses_regular", func(t *testing.T) {
		got := r.Match("example.com", nil, "", "")
		if got != router.ActionDirect {
			t.Errorf("Match(example.com) without net type = %q, want %q", got, router.ActionDirect)
		}
		got = r.Match("secure.example.com", nil, "", "")
		if got != router.ActionDirect {
			t.Errorf("Match(secure.example.com) without net type = %q, want %q", got, router.ActionDirect)
		}
	})

	// Cellular: network-type rules override regular rules.
	t.Run("cellular_overrides_regular", func(t *testing.T) {
		r.SetNetworkType("cellular")
		got := r.Match("example.com", nil, "", "")
		if got != router.ActionProxy {
			t.Errorf("Match(example.com) on cellular = %q, want %q (should override direct)", got, router.ActionProxy)
		}
		got = r.Match("secure.example.com", nil, "", "")
		if got != router.ActionReject {
			t.Errorf("Match(secure.example.com) on cellular = %q, want %q (should override direct)", got, router.ActionReject)
		}
	})

	// WiFi: no wifi-specific rule, so regular rules apply.
	t.Run("wifi_falls_through_to_regular", func(t *testing.T) {
		r.SetNetworkType("wifi")
		got := r.Match("example.com", nil, "", "")
		if got != router.ActionDirect {
			t.Errorf("Match(example.com) on wifi = %q, want %q (regular rule)", got, router.ActionDirect)
		}
		got = r.Match("secure.example.com", nil, "", "")
		if got != router.ActionDirect {
			t.Errorf("Match(secure.example.com) on wifi = %q, want %q (regular rule)", got, router.ActionDirect)
		}
	})

	// Ethernet: also no ethernet-specific rule, regular rules apply.
	t.Run("ethernet_falls_through_to_regular", func(t *testing.T) {
		r.SetNetworkType("ethernet")
		got := r.Match("example.com", nil, "", "")
		if got != router.ActionDirect {
			t.Errorf("Match(example.com) on ethernet = %q, want %q (regular rule)", got, router.ActionDirect)
		}
	})

	// Network-type rules also override for IP-based rules.
	t.Run("ip_cidr_network_type_priority", func(t *testing.T) {
		rIP := router.NewRouter(&router.RouterConfig{
			Rules: []router.Rule{
				{Type: "ip-cidr", Values: []string{"10.0.0.0/8"}, Action: router.ActionDirect},
				{Type: "ip-cidr", Values: []string{"10.0.0.0/8"}, Action: router.ActionReject, NetworkType: "cellular"},
			},
			DefaultAction: router.ActionProxy,
		}, nil, nil, nil)

		// No network type: regular rule.
		got := rIP.Match("", net.ParseIP("10.1.2.3"), "", "")
		if got != router.ActionDirect {
			t.Errorf("Match IP without net type = %q, want %q", got, router.ActionDirect)
		}

		// Cellular: network-type rule overrides.
		rIP.SetNetworkType("cellular")
		got = rIP.Match("", net.ParseIP("10.1.2.3"), "", "")
		if got != router.ActionReject {
			t.Errorf("Match IP on cellular = %q, want %q", got, router.ActionReject)
		}
	})

	// Network-type rules override for process rules.
	t.Run("process_network_type_priority", func(t *testing.T) {
		rProc := router.NewRouter(&router.RouterConfig{
			Rules: []router.Rule{
				{Type: "process", Values: []string{"chrome"}, Action: router.ActionDirect},
				{Type: "process", Values: []string{"chrome"}, Action: router.ActionReject, NetworkType: "cellular"},
			},
			DefaultAction: router.ActionProxy,
		}, nil, nil, nil)

		rProc.SetNetworkType("cellular")
		got := rProc.Match("", nil, "chrome", "")
		if got != router.ActionReject {
			t.Errorf("Match process on cellular = %q, want %q", got, router.ActionReject)
		}

		rProc.SetNetworkType("wifi")
		got = rProc.Match("", nil, "chrome", "")
		if got != router.ActionDirect {
			t.Errorf("Match process on wifi = %q, want %q (regular rule)", got, router.ActionDirect)
		}
	})
}

// ---------------------------------------------------------------------------
// TestClassifyInterfaceComprehensive
//
// Extensive interface name classification tests covering all patterns:
// WiFi, Cellular, Ethernet, and edge cases.
// ---------------------------------------------------------------------------

func TestClassifyInterfaceComprehensive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want netmon.NetworkType
	}{
		// --- WiFi patterns ---
		{"wlan0", netmon.NetworkWiFi},
		{"wlan1", netmon.NetworkWiFi},
		{"wlan10", netmon.NetworkWiFi},
		{"wlp2s0", netmon.NetworkWiFi},
		{"wlp3s0", netmon.NetworkWiFi},
		{"wlp0s20f3", netmon.NetworkWiFi},
		{"en0", netmon.NetworkWiFi},           // macOS WiFi
		{"en1", netmon.NetworkWiFi},           // macOS WiFi
		{"Wi-Fi", netmon.NetworkWiFi},         // Windows
		{"wi-fi", netmon.NetworkWiFi},         // case insensitive
		{"WI-FI", netmon.NetworkWiFi},         // all caps
		{"WLAN0", netmon.NetworkWiFi},         // case insensitive

		// --- Cellular patterns ---
		{"rmnet0", netmon.NetworkCellular},
		{"rmnet1", netmon.NetworkCellular},
		{"rmnet_data0", netmon.NetworkCellular},
		{"rmnet_ipa0", netmon.NetworkCellular},
		{"wwan0", netmon.NetworkCellular},
		{"wwan1", netmon.NetworkCellular},
		{"pdp_ip0", netmon.NetworkCellular},
		{"pdp_ip1", netmon.NetworkCellular},
		{"ccmni0", netmon.NetworkCellular},
		{"ccmni1", netmon.NetworkCellular},
		{"rev_rmnet0", netmon.NetworkCellular},
		{"RMNET0", netmon.NetworkCellular},     // case insensitive

		// --- Ethernet patterns ---
		{"eth0", netmon.NetworkEthernet},
		{"eth1", netmon.NetworkEthernet},
		{"eth10", netmon.NetworkEthernet},
		{"enp3s0", netmon.NetworkEthernet},
		{"enp0s25", netmon.NetworkEthernet},
		{"enp0s31f6", netmon.NetworkEthernet},
		{"eno1", netmon.NetworkEthernet},
		{"eno2", netmon.NetworkEthernet},
		{"en2", netmon.NetworkEthernet},        // macOS Ethernet (Thunderbolt)
		{"en3", netmon.NetworkEthernet},        // macOS Ethernet
		{"en10", netmon.NetworkEthernet},       // macOS Ethernet
		{"en99", netmon.NetworkEthernet},       // macOS high index
		{"ETH0", netmon.NetworkEthernet},       // case insensitive

		// --- Edge cases: Unknown ---
		{"lo", netmon.NetworkUnknown},          // loopback
		{"lo0", netmon.NetworkUnknown},         // macOS loopback
		{"docker0", netmon.NetworkUnknown},     // Docker bridge
		{"br-1a2b3c", netmon.NetworkUnknown},   // Docker bridge
		{"veth12345", netmon.NetworkUnknown},   // Docker veth
		{"vethb8f9c10", netmon.NetworkUnknown}, // Docker veth
		{"tun0", netmon.NetworkUnknown},        // VPN tunnel
		{"tun1", netmon.NetworkUnknown},        // VPN tunnel
		{"tap0", netmon.NetworkUnknown},        // TAP device
		{"virbr0", netmon.NetworkUnknown},      // libvirt bridge
		{"bridge0", netmon.NetworkUnknown},     // bridge
		{"bond0", netmon.NetworkUnknown},       // bonding
		{"dummy0", netmon.NetworkUnknown},      // dummy
		{"sit0", netmon.NetworkUnknown},        // SIT tunnel
		{"ip6tnl0", netmon.NetworkUnknown},     // IPv6 tunnel
		{"utun0", netmon.NetworkUnknown},       // macOS utun (VPN)
		{"utun3", netmon.NetworkUnknown},       // macOS utun

		// --- Edge cases: empty / unusual ---
		{"en", netmon.NetworkUnknown},          // "en" with no suffix -> no digits after "en"
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := netmon.ClassifyInterface(tt.name)
			if got != tt.want {
				t.Errorf("ClassifyInterface(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestClassifyInterfaceAndRouterIntegration
//
// Verifies that ClassifyInterface results can drive SetNetworkType and
// produce correct routing decisions in an end-to-end flow.
// ---------------------------------------------------------------------------

func TestClassifyInterfaceAndRouterIntegration(t *testing.T) {
	t.Parallel()

	r := router.NewRouter(&router.RouterConfig{
		Rules: []router.Rule{
			{Type: "domain", Values: []string{"video.example.com"}, Action: router.ActionDirect, NetworkType: "wifi"},
			{Type: "domain", Values: []string{"video.example.com"}, Action: router.ActionProxy, NetworkType: "cellular"},
			{Type: "domain", Values: []string{"video.example.com"}, Action: router.ActionDirect, NetworkType: "ethernet"},
		},
		DefaultAction: router.ActionProxy,
	}, nil, nil, nil)

	// Simulate detecting interface type and feeding it to the router.
	interfaceTests := []struct {
		ifaceName  string
		wantAction router.Action
	}{
		{"wlan0", router.ActionDirect},     // WiFi -> direct
		{"en0", router.ActionDirect},       // macOS WiFi -> direct
		{"rmnet0", router.ActionProxy},     // Cellular -> proxy
		{"wwan0", router.ActionProxy},      // Cellular -> proxy
		{"eth0", router.ActionDirect},      // Ethernet -> direct
		{"enp3s0", router.ActionDirect},    // Ethernet -> direct
		{"tun0", router.ActionProxy},       // Unknown -> no network-type match, falls to default
		{"docker0", router.ActionProxy},    // Unknown -> default
	}

	for _, tt := range interfaceTests {
		t.Run(tt.ifaceName, func(t *testing.T) {
			nt := netmon.ClassifyInterface(tt.ifaceName)
			r.SetNetworkType(nt.String())
			got := r.Match("video.example.com", nil, "", "")
			if got != tt.wantAction {
				t.Errorf("interface %q (type=%v): Match(video.example.com) = %q, want %q",
					tt.ifaceName, nt, got, tt.wantAction)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestNetworkTypeMultipleRuleTypes
//
// Verifies that network-type rules work for all rule types simultaneously:
// domain, ip-cidr, process, protocol.
// ---------------------------------------------------------------------------

func TestNetworkTypeMultipleRuleTypes(t *testing.T) {
	t.Parallel()

	r := router.NewRouter(&router.RouterConfig{
		Rules: []router.Rule{
			{Type: "domain", Values: []string{"example.com"}, Action: router.ActionDirect, NetworkType: "wifi"},
			{Type: "ip-cidr", Values: []string{"192.168.0.0/16"}, Action: router.ActionDirect, NetworkType: "wifi"},
			{Type: "process", Values: []string{"firefox"}, Action: router.ActionDirect, NetworkType: "wifi"},
			{Type: "protocol", Values: []string{"bittorrent"}, Action: router.ActionReject, NetworkType: "cellular"},
		},
		DefaultAction: router.ActionProxy,
	}, nil, nil, nil)

	// WiFi: domain, IP, and process rules match.
	r.SetNetworkType("wifi")

	if got := r.Match("example.com", nil, "", ""); got != router.ActionDirect {
		t.Errorf("WiFi domain match = %q, want direct", got)
	}
	if got := r.Match("", net.ParseIP("192.168.1.1"), "", ""); got != router.ActionDirect {
		t.Errorf("WiFi IP match = %q, want direct", got)
	}
	if got := r.Match("", nil, "firefox", ""); got != router.ActionDirect {
		t.Errorf("WiFi process match = %q, want direct", got)
	}
	// Cellular-only protocol rule should NOT match on WiFi.
	if got := r.Match("", nil, "", "bittorrent"); got != router.ActionProxy {
		t.Errorf("WiFi bittorrent (cellular rule) = %q, want proxy (default)", got)
	}

	// Cellular: only protocol rule matches.
	r.SetNetworkType("cellular")

	if got := r.Match("", nil, "", "bittorrent"); got != router.ActionReject {
		t.Errorf("Cellular bittorrent = %q, want reject", got)
	}
	// WiFi-only domain rule should NOT match on cellular.
	if got := r.Match("example.com", nil, "", ""); got != router.ActionProxy {
		t.Errorf("Cellular domain (wifi rule) = %q, want proxy (default)", got)
	}
}

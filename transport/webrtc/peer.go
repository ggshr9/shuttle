package webrtc

import (
	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
)

// ICEConfig holds the common ICE-related parameters shared between client and
// server when creating a PeerConnection.
type ICEConfig struct {
	STUNServers  []string
	TURNServers  []string
	TURNUser     string
	TURNPass     string
	ICEPolicy    string // "all", "relay", "public" (default "all")
	LoopbackOnly bool   // restrict ICE to 127.0.0.1, disable mDNS (for testing)
}

// buildICEServers converts STUN/TURN configuration into a slice of
// webrtc.ICEServer entries suitable for webrtc.Configuration.
func buildICEServers(cfg *ICEConfig) []webrtc.ICEServer {
	var servers []webrtc.ICEServer
	if len(cfg.STUNServers) > 0 {
		servers = append(servers, webrtc.ICEServer{
			URLs: cfg.STUNServers,
		})
	}
	if len(cfg.TURNServers) > 0 {
		servers = append(servers, webrtc.ICEServer{
			URLs:           cfg.TURNServers,
			Username:       cfg.TURNUser,
			Credential:     cfg.TURNPass,
			CredentialType: webrtc.ICECredentialTypePassword,
		})
	}
	return servers
}

// newPeerConnectionFromConfig creates a PeerConnection with detached
// DataChannels using the shared ICE configuration. Both client and server
// use this instead of duplicating the setup logic.
func newPeerConnectionFromConfig(cfg *ICEConfig) (*webrtc.PeerConnection, error) {
	se := webrtc.SettingEngine{}
	se.DetachDataChannels()
	if cfg.LoopbackOnly {
		se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
		_ = se.SetICEAddressRewriteRules(webrtc.ICEAddressRewriteRule{
			External:        []string{"127.0.0.1"},
			AsCandidateType: webrtc.ICECandidateTypeHost,
		})
		se.SetIncludeLoopbackCandidate(true)
	}

	api := webrtc.NewAPI(webrtc.WithSettingEngine(se))
	return api.NewPeerConnection(webrtc.Configuration{
		ICEServers:         buildICEServers(cfg),
		ICETransportPolicy: mapICEPolicy(cfg.ICEPolicy),
	})
}

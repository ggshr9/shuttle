package adapter

import "github.com/shuttleX/shuttle/config"

// BaseFactory provides default nil implementations of NewClient and NewServer.
// Per-request protocol factories (vless, vmess, trojan, shadowsocks) that do not
// use multiplexed transports can embed this to satisfy TransportFactory without
// writing identical stub methods.
type BaseFactory struct{}

func (BaseFactory) NewClient(_ *config.ClientConfig, _ FactoryOptions) (ClientTransport, error) {
	return nil, nil
}

func (BaseFactory) NewServer(_ *config.ServerConfig, _ FactoryOptions) (ServerTransport, error) {
	return nil, nil
}

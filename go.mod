module github.com/shuttle-proxy/shuttle

go 1.24

toolchain go1.24.12

require (
	github.com/flynn/noise v1.1.0
	github.com/hashicorp/yamux v0.1.2
	github.com/quic-go/quic-go v0.59.0
	golang.org/x/crypto v0.41.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/kr/text v0.2.0 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	nhooyr.io/websocket v1.8.17 // indirect
)

replace github.com/quic-go/quic-go v0.59.0 => ./quicfork

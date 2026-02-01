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
	github.com/getlantern/context v0.0.0-20190109183933-c447772a6520 // indirect
	github.com/getlantern/errors v0.0.0-20190325191628-abdb3e3e36f7 // indirect
	github.com/getlantern/golog v0.0.0-20190830074920-4ef2e798c2d7 // indirect
	github.com/getlantern/hex v0.0.0-20190417191902-c6586a6fe0b7 // indirect
	github.com/getlantern/hidden v0.0.0-20190325191715-f02dbb02be55 // indirect
	github.com/getlantern/ops v0.0.0-20190325191751-d70cb0d6f85f // indirect
	github.com/getlantern/systray v1.2.2 // indirect
	github.com/go-stack/stack v1.8.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/oxtoacart/bpool v0.0.0-20190530202638-03653db5a59c // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	nhooyr.io/websocket v1.8.17 // indirect
)

replace github.com/quic-go/quic-go v0.59.0 => ./quicfork

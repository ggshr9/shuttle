package engine

// Blank imports trigger init() registration for each transport factory.
import (
	_ "github.com/shuttleX/shuttle/transport/cdn"
	_ "github.com/shuttleX/shuttle/transport/h3"
	_ "github.com/shuttleX/shuttle/transport/hysteria2"
	_ "github.com/shuttleX/shuttle/transport/reality"
	_ "github.com/shuttleX/shuttle/transport/shadowsocks"
	_ "github.com/shuttleX/shuttle/transport/trojan"
	_ "github.com/shuttleX/shuttle/transport/vless"
	_ "github.com/shuttleX/shuttle/transport/vmess"
	_ "github.com/shuttleX/shuttle/transport/webrtc"
	_ "github.com/shuttleX/shuttle/transport/wireguard"
)

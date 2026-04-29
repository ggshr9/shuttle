package engine

// Blank imports trigger init() registration for each transport factory.
import (
	_ "github.com/ggshr9/shuttle/transport/cdn"
	_ "github.com/ggshr9/shuttle/transport/h3"
	_ "github.com/ggshr9/shuttle/transport/hysteria2"
	_ "github.com/ggshr9/shuttle/transport/reality"
	_ "github.com/ggshr9/shuttle/transport/shadowsocks"
	_ "github.com/ggshr9/shuttle/transport/trojan"
	_ "github.com/ggshr9/shuttle/transport/vless"
	_ "github.com/ggshr9/shuttle/transport/vmess"
	_ "github.com/ggshr9/shuttle/transport/webrtc"
	_ "github.com/ggshr9/shuttle/transport/wireguard"
)

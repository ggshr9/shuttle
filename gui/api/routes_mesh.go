package api

import (
	"net/http"
	"strings"

	"github.com/ggshr9/shuttle/engine"
)

func registerMeshRoutes(mux *http.ServeMux, eng *engine.Engine) {
	// GET /api/mesh/status — high-level mesh status (enabled, VIP, CIDR, peer count).
	mux.HandleFunc("GET /api/mesh/status", func(w http.ResponseWriter, r *http.Request) {
		ms := eng.Status().Mesh
		if ms == nil {
			writeJSON(w, map[string]any{
				"enabled":    false,
				"virtual_ip": "",
				"cidr":       "",
				"peer_count": 0,
			})
			return
		}
		writeJSON(w, map[string]any{
			"enabled":    ms.Enabled,
			"virtual_ip": ms.VirtualIP,
			"cidr":       ms.CIDR,
			"peer_count": len(ms.Peers),
		})
	})

	// GET /api/mesh/peers — list all mesh peers with connection quality.
	mux.HandleFunc("GET /api/mesh/peers", func(w http.ResponseWriter, r *http.Request) {
		ms := eng.Status().Mesh
		if ms == nil || len(ms.Peers) == 0 {
			writeJSON(w, []engine.MeshPeer{})
			return
		}
		writeJSON(w, ms.Peers)
	})

	// POST /api/mesh/peers/{vip}/connect — trigger P2P connection to a peer.
	mux.HandleFunc("POST /api/mesh/peers/", func(w http.ResponseWriter, r *http.Request) {
		// Parse VIP from path: /api/mesh/peers/{vip}/connect
		path := strings.TrimPrefix(r.URL.Path, "/api/mesh/peers/")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) != 2 || parts[1] != "connect" || parts[0] == "" {
			writeError(w, http.StatusNotFound, "expected /api/mesh/peers/{vip}/connect")
			return
		}
		vip := parts[0]

		if err := eng.ConnectMeshPeer(r.Context(), vip); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true, "vip": vip})
	})
}

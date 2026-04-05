# Sub-4: Mesh Deep Integration Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Expose mesh peer management via API, create MeshOutbound for routing traffic to mesh peers, and add mesh peer page to GUI.

**Existing infrastructure:**
- `mesh/client.go` — MeshClient with ListPeers(), ConnectPeer(), GetPeerState(), RouteMesh()
- `engine/mesh_manager.go` — MeshManager with Start/Close/Client()
- `engine/events.go` — MeshStatus/MeshPeer structs in Status()
- `gui/web/src/lib/MeshTopologyChart.svelte` — peer visualization component exists
- Split routes already work at TUN packet level

**What's missing:**
1. No REST API endpoints for mesh peer management
2. No MeshOutbound (can't route via routing rules to specific mesh peers)
3. No dedicated mesh peers page in GUI (topology chart exists but no management)

---

## Task 1: Mesh API Endpoints

**Files:**
- Create: `gui/api/routes_mesh.go` — mesh REST endpoints
- Modify: `gui/api/api.go` — register routes
- Test: `gui/api/routes_mesh_test.go`

Endpoints:
- `GET /api/mesh/status` — mesh enabled, VIP, CIDR, peer count
- `GET /api/mesh/peers` — list peers with quality metrics
- `POST /api/mesh/peers/{vip}/connect` — initiate P2P connection to peer

Implementation: read `engine.Status().Mesh` for status, call `meshManager.Client().ListPeers()` for peer list, call `meshManager.Client().ConnectPeer()` for connect.

---

## Task 2: MeshOutbound

**Files:**
- Create: `engine/outbound_mesh.go` — MeshOutbound implementing adapter.Outbound
- Create: `engine/outbound_mesh_test.go`
- Modify: `engine/engine_inbound.go` — build mesh outbound when mesh is enabled

MeshOutbound routes connections through the mesh tunnel to a specific peer or the mesh relay:

```go
type MeshOutbound struct {
    tag         string
    meshManager *MeshManager
}

func (m *MeshOutbound) DialContext(ctx, network, addr string) (net.Conn, error) {
    mc := m.meshManager.Client()
    if mc == nil {
        return nil, fmt.Errorf("mesh not connected")
    }
    // Use mesh tunnel — the MeshClient.Send/Receive handles packet routing
    // For TCP connections through mesh, we need a stream-based approach
    // Check if MeshClient supports stream-level dialing
}
```

**Important**: MeshClient currently operates at packet level (Send/Receive raw IP packets via TUN). TCP connection-level routing through mesh requires either:
- A) A TCP-over-mesh tunnel (MeshClient opens a stream to the peer, creates a net.Conn)
- B) Leveraging the existing TUN packet-level routing (simpler, already works)

**Recommended approach (B)**: MeshOutbound doesn't dial directly. Instead, it returns a connection that routes through TUN, which then uses MeshClient's packet routing. This means MeshOutbound is effectively a "direct" dial to the mesh VIP, and TUN intercepts the packets.

```go
func (m *MeshOutbound) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
    // Dial the address directly — TUN will intercept if it's a mesh destination
    return (&net.Dialer{}).DialContext(ctx, network, addr)
}
```

This works because routing rules already direct mesh-destined traffic to this outbound, and TUN's packet handler checks `IsMeshDestination()`.

---

## Task 3: Mesh GUI Page

**Files:**
- Create: `gui/web/src/pages/Mesh.svelte` — mesh peers management page
- Modify: `gui/web/src/App.svelte` — add Mesh tab
- Modify: `gui/web/src/lib/api.ts` — add mesh API functions

Page features:
- Mesh status card (enabled, VIP, CIDR)
- Peer list table (VIP, state, method, RTT, packet loss, score)
- Connect button per peer
- Existing MeshTopologyChart component embedded

---

## Dependency: Tasks 1-3 are sequential (API → Outbound → GUI).

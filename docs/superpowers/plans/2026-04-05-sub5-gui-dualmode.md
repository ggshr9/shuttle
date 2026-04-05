# Sub-5: GUI Dual-Mode Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a simplified "Simple Mode" to the existing GUI for one-click connect/disconnect with minimal UI, plus a toggle to switch to the existing "Advanced Mode" (current full UI).

**Existing GUI**: Svelte 5 + Vite + TypeScript, 6 pages (Dashboard, Servers, Subscriptions, Routing, Logs, Settings). All advanced features already exist.

**What's needed**: A simplified landing view that shows only essential information, hiding complexity until the user opts in.

---

## Task 1: Simple Mode Component

**Files:**
- Create: `gui/web/src/pages/SimpleMode.svelte`

Features:
- Large connect/disconnect button (center, prominent)
- Current server name + latency
- Real-time speed display (upload/download)
- Today's traffic summary (bytes sent/received)
- "Switch Server" dropdown (select from available servers)
- "Advanced Mode" button at bottom
- Clean, minimal design — no tabs, no sidebar

Data sources (all existing APIs):
- `/api/status` — connection state, active transport, speed
- `/api/config/servers` — server list for switcher
- WebSocket `/api/speed` — real-time speed updates

---

## Task 2: Mode Toggle + Persistence

**Files:**
- Modify: `gui/web/src/App.svelte` — add mode state, conditional rendering
- Modify: `gui/web/src/lib/api.ts` — add mode preference storage (localStorage)

Logic:
- `localStorage.getItem('shuttle_ui_mode')` — "simple" or "advanced" (default "simple")
- Simple mode: render `<SimpleMode />` full-screen, no tabs
- Advanced mode: render existing tab layout
- Toggle: button in SimpleMode → switch to advanced; button in Settings → switch to simple

---

## Task 3: Auto-Select Best Server

**Files:**
- Modify: `gui/web/src/pages/SimpleMode.svelte` — auto-select logic

When user clicks connect without explicitly selecting a server:
1. Call speed test on available servers
2. Select lowest latency
3. Connect

This uses existing `/api/speedtest` endpoint.

---

## Dependency: Tasks 1→2→3 sequential.

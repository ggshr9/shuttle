package admin

import "net/http"

// dashboardHTML is an embedded single-page admin dashboard.
const dashboardHTML = `<!DOCTYPE html>
<html data-theme="dark">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Shuttle Admin</title>
<style>
:root {
  --bg: #0f1117; --bg2: #161b22; --bg3: #21262d; --fg: #e1e4e8;
  --fg2: #8b949e; --border: #2d333b; --accent: #58a6ff;
  --green: #3fb950; --red: #f85149; --purple: #a371f7;
}
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: var(--bg); color: var(--fg); }
.app { max-width: 800px; margin: 0 auto; padding: 20px; }
h1 { font-size: 20px; margin-bottom: 20px; }
h2 { font-size: 16px; color: var(--fg2); margin: 24px 0 12px; }

.login { display: flex; flex-direction: column; align-items: center; justify-content: center; height: 80vh; gap: 12px; }
.login input { width: 300px; padding: 10px; background: var(--bg2); border: 1px solid var(--border); border-radius: 6px; color: var(--fg); font-size: 14px; }
.login button { width: 300px; padding: 10px; background: var(--accent); color: #fff; border: none; border-radius: 6px; cursor: pointer; font-size: 14px; }

.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 12px; }
.card { background: var(--bg2); border: 1px solid var(--border); border-radius: 8px; padding: 14px; }
.card .label { font-size: 11px; color: var(--fg2); text-transform: uppercase; letter-spacing: 0.5px; }
.card .value { font-size: 22px; font-weight: 600; margin-top: 4px; }

table { width: 100%; border-collapse: collapse; margin-top: 8px; }
th, td { text-align: left; padding: 8px 12px; border-bottom: 1px solid var(--border); font-size: 13px; }
th { color: var(--fg2); font-weight: 500; font-size: 11px; text-transform: uppercase; }
.mono { font-family: 'Cascadia Code', 'Fira Code', monospace; font-size: 12px; }
.badge { display: inline-block; padding: 2px 8px; border-radius: 10px; font-size: 11px; }
.badge.on { background: rgba(63,185,80,.15); color: var(--green); }
.badge.off { background: rgba(248,81,73,.15); color: var(--red); }

.actions { display: flex; gap: 8px; margin-top: 16px; }
.btn { padding: 8px 16px; border-radius: 6px; border: 1px solid var(--border); background: var(--bg3); color: var(--fg); cursor: pointer; font-size: 13px; }
.btn:hover { border-color: var(--accent); }
.btn.danger { color: var(--red); }
.btn.primary { background: var(--accent); color: #fff; border-color: var(--accent); }

.add-form { display: flex; gap: 8px; margin-top: 12px; align-items: center; }
.add-form input { padding: 8px 10px; background: var(--bg2); border: 1px solid var(--border); border-radius: 6px; color: var(--fg); font-size: 13px; }
.add-form input[name="name"] { flex: 1; }
.add-form input[name="quota"] { width: 120px; }

.error { color: var(--red); font-size: 13px; margin-top: 8px; }
.hidden { display: none; }
</style>
</head>
<body>
<div class="app" id="app">
  <div id="login-view" class="login">
    <h1>Shuttle Admin</h1>
    <input type="password" id="token-input" placeholder="Admin Token" autofocus>
    <button onclick="login()">Login</button>
    <div id="login-error" class="error hidden"></div>
  </div>

  <div id="main-view" class="hidden">
    <h1>Shuttle Admin Dashboard</h1>

    <div class="cards" id="status-cards"></div>

    <h2>Users</h2>
    <table>
      <thead><tr><th>Name</th><th>Token</th><th>Sent</th><th>Recv</th><th>Conns</th><th>Status</th><th></th></tr></thead>
      <tbody id="users-body"></tbody>
    </table>
    <div class="add-form">
      <input name="name" id="new-user-name" placeholder="Username">
      <input name="quota" id="new-user-quota" placeholder="Quota (GB, 0=unlimited)" type="number">
      <button class="btn primary" onclick="addUser()">Add User</button>
    </div>
    <div id="user-error" class="error hidden"></div>

    <div class="actions">
      <button class="btn" onclick="reloadConfig()">Reload Config</button>
      <button class="btn" onclick="refresh()">Refresh</button>
    </div>
  </div>
</div>
<script>
let TOKEN = '';
const BASE = location.origin;

function headers() {
  return { 'Authorization': 'Bearer ' + TOKEN, 'Content-Type': 'application/json' };
}

async function apiFetch(path, opts = {}) {
  const res = await fetch(BASE + path, { ...opts, headers: headers() });
  if (!res.ok) {
    const e = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(e.error || res.statusText);
  }
  return res.json();
}

function formatBytes(b) {
  if (b < 1024) return b + ' B';
  if (b < 1048576) return (b / 1024).toFixed(1) + ' KB';
  if (b < 1073741824) return (b / 1048576).toFixed(1) + ' MB';
  return (b / 1073741824).toFixed(2) + ' GB';
}

async function login() {
  TOKEN = document.getElementById('token-input').value;
  try {
    await apiFetch('/api/status');
    document.getElementById('login-view').classList.add('hidden');
    document.getElementById('main-view').classList.remove('hidden');
    refresh();
  } catch (e) {
    document.getElementById('login-error').textContent = e.message;
    document.getElementById('login-error').classList.remove('hidden');
  }
}

document.getElementById('token-input').addEventListener('keydown', e => {
  if (e.key === 'Enter') login();
});

async function refresh() {
  try {
    const [status, users] = await Promise.all([
      apiFetch('/api/status'),
      apiFetch('/api/users'),
    ]);
    renderStatus(status);
    renderUsers(users);
  } catch (e) {
    console.error(e);
  }
}

function renderStatus(s) {
  const cards = [
    { label: 'Uptime', value: s.uptime },
    { label: 'Active Conns', value: s.active_conns },
    { label: 'Total Conns', value: s.total_conns },
    { label: 'Sent', value: formatBytes(s.bytes_sent) },
    { label: 'Received', value: formatBytes(s.bytes_recv) },
    { label: 'Version', value: s.version },
  ];
  document.getElementById('status-cards').innerHTML = cards.map(c =>
    '<div class="card"><div class="label">' + esc(String(c.label)) + '</div><div class="value">' + esc(String(c.value)) + '</div></div>'
  ).join('');
}

function renderUsers(users) {
  const tbody = document.getElementById('users-body');
  if (!users || users.length === 0) {
    tbody.innerHTML = '<tr><td colspan="7" style="color:var(--fg2)">No users configured</td></tr>';
    return;
  }
  tbody.innerHTML = users.map(u => {
    const safeToken = esc(u.token);
    const tokenJSON = JSON.stringify(u.token);
    return '<tr>' +
    '<td>' + esc(u.name) + '</td>' +
    '<td class="mono">' + esc(u.token.substring(0, 8)) + '...</td>' +
    '<td>' + esc(formatBytes(u.bytes_sent)) + '</td>' +
    '<td>' + esc(formatBytes(u.bytes_recv)) + '</td>' +
    '<td>' + esc(String(u.active_conns)) + '</td>' +
    '<td><span class="badge ' + (u.enabled ? 'on' : 'off') + '">' + (u.enabled ? 'Active' : 'Disabled') + '</span></td>' +
    '<td>' +
      '<button class="btn" onclick="toggleUser(' + tokenJSON + ',' + !u.enabled + ')">' + (u.enabled ? 'Disable' : 'Enable') + '</button> ' +
      '<button class="btn danger" onclick="deleteUser(' + tokenJSON + ')">Delete</button>' +
    '</td>' +
  '</tr>'}).join('');
}

function esc(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

async function addUser() {
  const name = document.getElementById('new-user-name').value.trim();
  const quotaGB = parseFloat(document.getElementById('new-user-quota').value) || 0;
  if (!name) return;
  try {
    await apiFetch('/api/users', {
      method: 'POST',
      body: JSON.stringify({ name, max_bytes: Math.round(quotaGB * 1073741824) }),
    });
    document.getElementById('new-user-name').value = '';
    document.getElementById('new-user-quota').value = '';
    document.getElementById('user-error').classList.add('hidden');
    refresh();
  } catch (e) {
    document.getElementById('user-error').textContent = e.message;
    document.getElementById('user-error').classList.remove('hidden');
  }
}

async function deleteUser(token) {
  if (!confirm('Delete this user?')) return;
  try { await apiFetch('/api/users/' + token, { method: 'DELETE' }); refresh(); }
  catch (e) { alert(e.message); }
}

async function toggleUser(token, enabled) {
  try { await apiFetch('/api/users/' + token, { method: 'PUT', body: JSON.stringify({ enabled }) }); refresh(); }
  catch (e) { alert(e.message); }
}

async function reloadConfig() {
  try { await apiFetch('/api/reload', { method: 'POST' }); alert('Config reloaded'); refresh(); }
  catch (e) { alert(e.message); }
}
</script>
</body>
</html>`

func handleDashboard(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(dashboardHTML))
}

const API = '/api';
const form = document.getElementById('probe-form');
const probesBody = document.getElementById('probes-body');
const historyEl = document.getElementById('history');
const fields = ['probe-id','name','type','host','port','user','dbname','password','enabled'];

let statusCache = {};

async function request(url, options = {}) {
  const res = await fetch(`${API}${url}`, {
    headers: {'Content-Type': 'application/json'},
    ...options,
  });
  if (!res.ok) throw new Error(await res.text());
  if (res.status === 204) return null;
  return res.json();
}

function resetForm() {
  fields.forEach(id => {
    const el = document.getElementById(id);
    if (el.type === 'checkbox') el.checked = true;
    else el.value = '';
  });
  document.getElementById('type').value = 'postgres';
}

function render(probes) {
  probesBody.innerHTML = probes.map(p => {
    const st = statusCache[p.id];
    const cls = st?.status ? `status-${st.status}` : '';
    return `<tr>
      <td>${p.name}${p.enabled ? '' : ' (停用)'}</td>
      <td>${p.type}</td>
      <td>${p.host}:${p.port}</td>
      <td class="${cls}">${st?.status || 'unknown'}</td>
      <td>${st?.checkedAt ? new Date(st.checkedAt).toLocaleString() : '-'}</td>
      <td>
        <button onclick="editProbe('${p.id}')">编辑</button>
        <button onclick="checkProbe('${p.id}')">检查</button>
        <button onclick="showHistory('${p.id}')">历史</button>
        <button onclick="deleteProbe('${p.id}')">删除</button>
      </td>
    </tr>`;
  }).join('');
}

async function loadProbes() {
  const probes = await request('/probes');
  for (const p of probes) {
    const h = await request(`/probes/${p.id}/history`);
    if (h.length) statusCache[p.id] = h[0];
  }
  render(probes);
}

window.editProbe = async (id) => {
  const probes = await request('/probes');
  const p = probes.find(v => v.id === id);
  if (!p) return;
  document.getElementById('probe-id').value = p.id;
  document.getElementById('name').value = p.name;
  document.getElementById('type').value = p.type;
  document.getElementById('host').value = p.host;
  document.getElementById('port').value = p.port;
  document.getElementById('user').value = p.user || '';
  document.getElementById('dbname').value = p.dbname || '';
  document.getElementById('enabled').checked = p.enabled;
};

window.checkProbe = async (id) => {
  statusCache[id] = await request(`/probes/${id}/check`, {method: 'POST'});
  await loadProbes();
};

window.showHistory = async (id) => {
  const list = await request(`/probes/${id}/history`);
  historyEl.innerHTML = list.map(h => `<li><b class="status-${h.status}">${h.status}</b> ${new Date(h.checkedAt).toLocaleString()} - ${h.message}</li>`).join('');
};

window.deleteProbe = async (id) => {
  if (!confirm('确认删除该探针吗？')) return;
  await request(`/probes/${id}`, {method: 'DELETE'});
  delete statusCache[id];
  await loadProbes();
};

form.addEventListener('submit', async (e) => {
  e.preventDefault();
  const probe = {
    id: document.getElementById('probe-id').value,
    name: document.getElementById('name').value,
    type: document.getElementById('type').value,
    host: document.getElementById('host').value,
    port: Number(document.getElementById('port').value),
    user: document.getElementById('user').value,
    dbname: document.getElementById('dbname').value,
    password: document.getElementById('password').value,
    enabled: document.getElementById('enabled').checked,
  };
  const method = probe.id ? 'PUT' : 'POST';
  const path = probe.id ? `/probes/${probe.id}` : '/probes';
  await request(path, {method, body: JSON.stringify(probe)});
  resetForm();
  await loadProbes();
});

document.getElementById('reset-btn').addEventListener('click', resetForm);
document.getElementById('check-all-btn').addEventListener('click', async () => {
  const result = await request('/probes/check-all', {method: 'POST'});
  Object.assign(statusCache, result);
  await loadProbes();
});

loadProbes().catch(err => alert(err.message));

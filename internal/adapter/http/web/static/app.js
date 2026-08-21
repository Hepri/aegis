const API = '/api';

async function getClients() {
  const res = await fetch(`${API}/clients`);
  return res.json();
}

async function getClient(id) {
  const res = await fetch(`${API}/clients/${id}`);
  if (!res.ok) throw new Error('Not found');
  return res.json();
}

async function getClientPreview(id) {
  const res = await fetch(`${API}/clients/${id}/preview`);
  if (!res.ok) return null;
  return res.json();
}

async function createClient(name) {
  const res = await fetch(`${API}/clients`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name })
  });
  return res.json();
}

async function addUser(clientId, user) {
  const res = await fetch(`${API}/clients/${clientId}/users`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(user)
  });
  return res.json();
}

async function updateSchedule(clientId, userId, schedule) {
  await fetch(`${API}/clients/${clientId}/users/${userId}/schedule`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ schedule })
  });
}

async function deleteUser(clientId, userId) {
  await fetch(`${API}/clients/${clientId}/users/${userId}`, { method: 'DELETE' });
}

async function deleteClient(clientId) {
  await fetch(`${API}/clients/${clientId}`, { method: 'DELETE' });
}

async function grantTemporaryAccess(clientId, userId, duration) {
  await fetch(`${API}/clients/${clientId}/temporary-access`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user_id: userId, duration })
  });
}

async function blockComputer(clientId, duration) {
  await fetch(`${API}/clients/${clientId}/block`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ duration })
  });
}

async function deleteBlock(clientId, requestId) {
  await fetch(`${API}/clients/${clientId}/block/${requestId}`, { method: 'DELETE' });
}

async function deleteTemporaryAccess(clientId, requestId) {
  await fetch(`${API}/clients/${clientId}/temporary-access/${requestId}`, { method: 'DELETE' });
}

const days = ['monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday', 'sunday'];
const dayLabels = { monday: 'Пн', tuesday: 'Вт', wednesday: 'Ср', thursday: 'Чт', friday: 'Пт', saturday: 'Сб', sunday: 'Вс' };

let currentClientId = null;
let currentClient = null;

async function getActivity(clientId, date) {
  const q = date ? `?date=${encodeURIComponent(date)}` : '';
  const res = await fetch(`${API}/clients/${clientId}/activity${q}`);
  if (!res.ok) throw new Error('activity fetch failed');
  return res.json();
}

function todayISODate() {
  const d = new Date();
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${y}-${m}-${day}`;
}

function formatDurationMs(ms) {
  if (!ms || ms < 0) return '0м';
  const totalMin = Math.round(ms / 60000);
  if (totalMin < 60) return `${totalMin}м`;
  const h = Math.floor(totalMin / 60);
  const m = totalMin % 60;
  return m ? `${h}ч ${m}м` : `${h}ч`;
}

function formatDateTime(isoStr) {
  if (!isoStr) return '—';
  const d = new Date(isoStr);
  return d.toLocaleString('ru-RU', { hour: '2-digit', minute: '2-digit', day: 'numeric', month: 'short' });
}

function updateOnlineStatus() {
  const el = document.getElementById('onlineStatus');
  if (!el || !currentClient) return;
  if (currentClient.online) {
    el.textContent = 'онлайн';
    el.className = 'onlineStatus online';
  } else {
    el.textContent = currentClient.last_seen
      ? `офлайн (был ${formatDateTime(currentClient.last_seen)})`
      : 'офлайн';
    el.className = 'onlineStatus offline';
  }
}

async function renderActivity() {
  const sessionsEl = document.getElementById('activitySessions');
  const appsEl = document.getElementById('activityApps');
  const timelineEl = document.getElementById('activityTimeline');
  if (!currentClientId) {
    sessionsEl.innerHTML = '';
    appsEl.innerHTML = '';
    timelineEl.innerHTML = '';
    return;
  }
  const dateInput = document.getElementById('activityDate');
  if (!dateInput.value) dateInput.value = todayISODate();
  try {
    const data = await getActivity(currentClientId, dateInput.value);
    const sessions = data.sessions || [];
    if (sessions.length === 0) {
      sessionsEl.innerHTML = '<p class="emptyHint">Нет сессий за этот день</p>';
    } else {
      sessionsEl.innerHTML = `<table class="activityTable"><thead><tr>
        <th>Пользователь</th><th>Вход</th><th>Выход</th><th>Длительность</th><th>Блокировка экрана</th>
      </tr></thead><tbody>${sessions.map(s => `<tr>
        <td>${s.username || '—'}</td>
        <td>${formatDateTime(s.login)}</td>
        <td>${s.logout ? formatDateTime(s.logout) : 'ещё открыта'}</td>
        <td>${formatDurationMs(s.duration_ms)}</td>
        <td>${s.locked_ms ? formatDurationMs(s.locked_ms) : '—'}</td>
      </tr>`).join('')}</tbody></table>`;
    }

    const apps = data.apps || [];
    if (apps.length === 0) {
      appsEl.innerHTML = '<p class="emptyHint">Нет данных по приложениям</p>';
    } else {
      appsEl.innerHTML = `<table class="activityTable"><thead><tr>
        <th>Приложение</th><th>Открыто</th><th>В фокусе</th>
      </tr></thead><tbody>${apps.map(a => `<tr>
        <td title="${a.exe_path || ''}">${a.app_name || a.exe_path || '—'}</td>
        <td>${formatDurationMs(a.open_ms)}</td>
        <td>${formatDurationMs(a.focus_ms)}</td>
      </tr>`).join('')}</tbody></table>`;
    }

    const timeline = data.focus_timeline || [];
    if (timeline.length === 0) {
      timelineEl.innerHTML = '<p class="emptyHint">Нет таймлайна фокуса</p>';
    } else {
      timelineEl.innerHTML = timeline.map(t => `<div class="focusSpan">
        <span class="focusTime">${formatTime(t.start)}–${formatTime(t.end)}</span>
        <span class="focusApp">${t.app_name || t.exe_path || '—'}</span>
      </div>`).join('');
    }
  } catch (e) {
    sessionsEl.innerHTML = '<p class="emptyHint">Не удалось загрузить активность</p>';
    appsEl.innerHTML = '';
    timelineEl.innerHTML = '';
  }
}

async function selectClient() {
  const sel = document.getElementById('clientSelect');
  currentClientId = sel.value;
  if (!currentClientId) {
    document.getElementById('clientSection').style.display = 'none';
    return;
  }
  currentClient = await getClient(currentClientId);
  document.getElementById('clientSection').style.display = 'block';
  document.getElementById('clientIdDisplay').textContent = currentClientId;
  updateOnlineStatus();
  renderUsers();
  renderConfigPreview();
  const dateInput = document.getElementById('activityDate');
  if (!dateInput.value) dateInput.value = todayISODate();
  renderActivity();
}

function formatTime(isoStr) {
  const d = new Date(isoStr);
  return d.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' });
}

function formatDateLabel(isoStr) {
  const d = new Date(isoStr);
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const tomorrow = new Date(today);
  tomorrow.setDate(tomorrow.getDate() + 1);
  const dayStart = new Date(d.getFullYear(), d.getMonth(), d.getDate());
  const dateStr = d.toLocaleDateString('ru-RU', { day: 'numeric', month: 'long' });
  if (dayStart.getTime() === today.getTime()) return 'Сегодня, ' + dateStr;
  if (dayStart.getTime() === tomorrow.getTime()) return 'Завтра, ' + dateStr;
  return dateStr;
}

async function renderConfigPreview() {
  const div = document.getElementById('configPreviewContent');
  if (!currentClientId) { div.innerHTML = ''; return; }
  const config = await getClientPreview(currentClientId);
  if (!config || !config.users || config.users.length === 0) {
    div.innerHTML = '<p class="dayLabel">Нет пользователей или интервалов</p>';
    return;
  }
  const userById = Object.fromEntries((currentClient.users || []).map(u => [u.username, u]));
  let html = '';
  for (const uc of config.users) {
    const name = (userById[uc.username] || {}).name || uc.username;
    const byDay = {};
    for (const iv of uc.allowed_intervals || []) {
      const dayKey = iv.start.slice(0, 10);
      if (!byDay[dayKey]) byDay[dayKey] = { intervals: [], firstStart: iv.start };
      byDay[dayKey].intervals.push(`${formatTime(iv.start)}–${formatTime(iv.end)}`);
    }
    const dayKeys = Object.keys(byDay).sort();
    let dayHtml = '';
    for (const k of dayKeys) {
      const label = formatDateLabel(byDay[k].firstStart);
      dayHtml += `<div class="dayBlock"><span class="dayLabel">${label}</span><div class="intervalsList">${byDay[k].intervals.join(', ')}</div></div>`;
    }
    html += `<div class="userIntervals"><span class="userName">${name}</span>${dayHtml}</div>`;
  }
  div.innerHTML = html || '<p class="dayLabel">Нет интервалов доступа</p>';
}

function renderUsers() {
  const ul = document.getElementById('userList');
  ul.innerHTML = (currentClient.users || []).map(u => {
    // Get temp access and blocks for this user
    const userTempAccess = (currentClient.temporary_access_requests || []).filter(t => t.user_id === u.id);
    const userBlocks = (currentClient.block_requests || []).filter(b => b.user_id === u.id);
    const now = new Date();
    const activeTempAccess = userTempAccess.filter(t => new Date(t.until) > now);
    const activeBlocks = userBlocks.filter(b => new Date(b.until) > now);
    
    return `
    <li data-user-id="${u.id}" class="userCard">
      <div class="userHeader">
        <div>
          <span class="userName">${u.name}</span>
          <code>${u.username}</code>
        </div>
        <button onclick="deleteUserConfirm('${u.id}')" class="deleteBtn">×</button>
      </div>
      
      ${activeTempAccess.length > 0 ? `
        <div class="userTempAccess">
          <span class="badge">Временный доступ</span>
          ${activeTempAccess.map(t => `
            <span class="tempAccessTime">${formatTime(t.start)} — ${formatTime(t.until)}</span>
            <button onclick="deleteTempAccessConfirm('${t.id}')" class="deleteBtn smallBtn">×</button>
          `).join('')}
        </div>
      ` : ''}
      
      ${activeBlocks.length > 0 ? `
        <div class="userBlock">
          <span class="badge badgeRed">Заблокирован</span>
          ${activeBlocks.map(b => `
            <span class="tempAccessTime">${formatTime(b.start)} — ${formatTime(b.until)}</span>
            <button onclick="deleteBlockConfirm('${b.id}')" class="deleteBtn smallBtn">×</button>
          `).join('')}
        </div>
      ` : ''}
      
      <div class="userActions">
        <button onclick="editSchedule('${u.id}')">📅 Расписание</button>
        <div class="grantAccessControl">
          <select id="duration_${u.id}" class="smallSelect">
            <option value="15">15 мин</option>
            <option value="30">30 мин</option>
            <option value="45">45 мин</option>
            <option value="60">1 час</option>
            <option value="120">2 часа</option>
            <option value="180">3 часа</option>
            <option value="240">4 часа</option>
            <option value="other">Другое</option>
          </select>
          <span id="customDuration_${u.id}" style="display:none" class="customDuration">
            <input type="number" id="hours_${u.id}" min="0" max="72" value="1" class="smallInput"> ч
            <input type="number" id="minutes_${u.id}" min="0" max="59" value="0" class="smallInput"> мин
          </span>
          <button onclick="grantAccessToUser('${u.id}')" class="primaryBtn">⏱️ Добавить время</button>
          <button onclick="blockUser('${u.id}')" class="dangerBtn">🚫 Заблокировать</button>
        </div>
      </div>
    </li>
  `;
  }).join('');
  
  // Setup duration change listeners
  (currentClient.users || []).forEach(u => {
    const sel = document.getElementById(`duration_${u.id}`);
    if (sel) {
      sel.addEventListener('change', () => {
        document.getElementById(`customDuration_${u.id}`).style.display = 
          sel.value === 'other' ? 'inline' : 'none';
      });
    }
  });
}

async function deleteBlockConfirm(requestId) {
  if (!confirm('Удалить блокировку?')) return;
  await deleteBlock(currentClientId, requestId);
  currentClient = await getClient(currentClientId);
  renderUsers();
  renderConfigPreview();
}

async function deleteTempAccessConfirm(requestId) {
  if (!confirm('Удалить временный доступ?')) return;
  await deleteTemporaryAccess(currentClientId, requestId);
  currentClient = await getClient(currentClientId);
  renderUsers();
  renderConfigPreview();
}


function getDurationMinutes(userId) {
  const sel = document.getElementById(`duration_${userId}`);
  if (!sel) return 60;
  if (sel.value !== 'other') return parseInt(sel.value, 10);
  const h = parseInt(document.getElementById(`hours_${userId}`).value || 0, 10);
  const m = parseInt(document.getElementById(`minutes_${userId}`).value || 0, 10);
  return h * 60 + m;
}

async function grantAccessToUser(userId) {
  const duration = getDurationMinutes(userId);
  if (duration <= 0) {
    alert('Укажите длительность');
    return;
  }
  await grantTemporaryAccess(currentClientId, userId, duration);
  currentClient = await getClient(currentClientId);
  renderUsers();
  renderConfigPreview();
}

async function blockUser(userId) {
  const duration = getDurationMinutes(userId);
  if (duration <= 0) {
    alert('Укажите длительность');
    return;
  }
  await fetch(`${API}/clients/${currentClientId}/block`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user_id: userId, duration })
  });
  currentClient = await getClient(currentClientId);
  renderUsers();
  renderConfigPreview();
}

function renderScheduleEditor(userId) {
  const user = currentClient.users.find(u => u.id === userId);
  if (!user) return;
  const schedule = user.schedule || {};
  const div = document.getElementById('scheduleEditor');
  div.innerHTML = days.map(day => {
    const intervals = schedule[day] || [];
    return `
      <div class="schedule-day" data-day="${day}">
        <label>${dayLabels[day] || day}</label>
        ${intervals.map((iv, i) => `
          <div class="interval" data-day="${day}">
            <input type="time" value="${iv.start}" data-field="start">
            <span>—</span>
            <input type="time" value="${iv.end}" data-field="end">
          </div>
        `).join('')}
        <button type="button" onclick="addInterval('${userId}', '${day}')">+</button>
      </div>
    `;
  }).join('');
  div.querySelectorAll('input').forEach(input => {
    input.addEventListener('change', () => saveScheduleFromEditor(userId));
  });
}

function addInterval(userId, day) {
  const user = currentClient.users.find(u => u.id === userId);
  if (!user.schedule) user.schedule = {};
  if (!user.schedule[day]) user.schedule[day] = [];
  user.schedule[day].push({ start: '09:00', end: '17:00' });
  renderScheduleEditor(userId);
}

async function saveScheduleFromEditor(userId) {
  const div = document.getElementById('scheduleEditor');
  const schedule = {};
  days.forEach(day => {
    const dayEl = div.querySelector(`.schedule-day[data-day="${day}"]`);
    if (!dayEl) return;
    const intervals = [];
    dayEl.querySelectorAll('.interval').forEach(intervalEl => {
      const start = intervalEl.querySelector('input[data-field="start"]');
      const end = intervalEl.querySelector('input[data-field="end"]');
      if (start && end && start.value && end.value) {
        intervals.push({ start: start.value, end: end.value });
      }
    });
    if (intervals.length) schedule[day] = intervals;
  });
  await updateSchedule(currentClientId, userId, schedule);
  currentClient = await getClient(currentClientId);
  renderConfigPreview();
}

function editSchedule(userId) {
  renderScheduleEditor(userId);
}

async function deleteUserConfirm(userId) {
  if (!confirm('Удалить пользователя?')) return;
  await deleteUser(currentClientId, userId);
  currentClient = await getClient(currentClientId);
  renderUsers();
  renderConfigPreview();
}

document.getElementById('clientSelect').addEventListener('change', selectClient);
document.getElementById('addClient').addEventListener('click', async () => {
  const name = prompt('Имя компьютера:', 'Home PC');
  if (!name) return;
  const { id } = await createClient(name);
  await loadClients();
  document.getElementById('clientSelect').value = id;
  selectClient();
  alert(`Компьютер добавлен. Client ID: ${id}\n\nСкопируйте его для установки клиента:\naegis-client.exe install --server-url=http://server:8080 --client-id=${id}`);
});

document.getElementById('copyClientId').addEventListener('click', () => {
  const id = document.getElementById('clientIdDisplay').textContent;
  navigator.clipboard.writeText(id).then(() => alert('Client ID скопирован')).catch(() => alert('Не удалось скопировать'));
});

document.getElementById('deleteClient').addEventListener('click', async () => {
  if (!currentClientId) return;
  if (!confirm(`Удалить компьютер «${currentClient.name || currentClientId}»? Все пользователи и расписание будут удалены.`)) return;
  await deleteClient(currentClientId);
  currentClientId = null;
  currentClient = null;
  document.getElementById('clientSection').style.display = 'none';
  document.getElementById('clientSelect').value = '';
  await loadClients();
});

document.getElementById('addUser').addEventListener('click', async () => {
  const name = prompt('Имя (например, Александр):');
  const username = prompt('Имя учётной записи Windows:');
  if (!name || !username) return;
  await addUser(currentClientId, { name, username, schedule: {} });
  currentClient = await getClient(currentClientId);
  renderUsers();
  renderConfigPreview();
});

async function loadClients() {
  const clients = await getClients();
  const sel = document.getElementById('clientSelect');
  const prev = sel.value;
  sel.innerHTML = '<option value="">— Выберите компьютер —</option>' +
    clients.map(c => {
      const mark = c.online ? ' ●' : '';
      return `<option value="${c.id}">${c.name || c.id}${mark}</option>`;
    }).join('');
  if (prev) {
    sel.value = prev;
  } else if (clients.length > 0) {
    sel.value = clients[0].id;
    await selectClient();
  }
}

document.getElementById('refreshActivity').addEventListener('click', renderActivity);
document.getElementById('activityDate').addEventListener('change', renderActivity);

loadClients();
setInterval(() => {
  if (currentClientId) {
    getClient(currentClientId).then(c => {
      currentClient = c;
      updateOnlineStatus();
    }).catch(() => {});
  }
}, 60000);

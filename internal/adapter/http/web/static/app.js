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

async function updateEarnSettings(clientId, settings) {
  await fetch(`${API}/clients/${clientId}/earn-settings`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(settings)
  });
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
  const ver = currentClient.client_version ? ` · v${currentClient.client_version}` : '';
  if (currentClient.online) {
    el.textContent = 'онлайн' + ver;
    el.className = 'onlineStatus online';
  } else {
    el.textContent = currentClient.last_seen
      ? `офлайн (был ${formatDateTime(currentClient.last_seen)})${ver}`
      : 'офлайн' + ver;
    el.className = 'onlineStatus offline';
  }
}

function formatTime(isoStr) {
  if (!isoStr) return '—';
  const d = new Date(isoStr);
  return d.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' });
}

function isHiddenActivityUser(username) {
  return String(username || '').toLowerCase() === 'admin';
}

function formatSessionRange(s) {
  const start = formatTime(s.login);
  if (s.locked_now) return `${start} — экран`;
  if (!s.logout) return `${start} — сейчас`;
  return `${start} — ${formatTime(s.logout)}`;
}

function isSessionActive(s) {
  return !s.logout && !s.locked_now;
}

function sessionSortKey(s) {
  if (isSessionActive(s)) return 0;
  if (!s.logout && s.locked_now) return 1;
  return 2;
}

function renderSessionCard(s, { open = false } = {}) {
  const apps = s.apps || [];
  const active = isSessionActive(s);
  const locked = s.locked_now;
  const appsHtml = apps.length === 0
    ? '<p class="emptyHint">Нет приложений</p>'
    : `<table class="activityTable"><thead><tr>
        <th>Приложение</th><th>Открыто</th><th>В фокусе</th>
      </tr></thead><tbody>${apps.map(a => `<tr>
        <td title="${escapeAttr(a.exe_path || '')}">${escapeHtml(a.app_name || a.exe_path || '—')}</td>
        <td>${formatDurationMs(a.open_ms)}</td>
        <td>${formatDurationMs(a.focus_ms)}</td>
      </tr>`).join('')}</tbody></table>`;
  const lockedHint = (!locked && s.locked_ms)
    ? `<span class="sessionLocked">экран ${formatDurationMs(s.locked_ms)}</span>`
    : '';
  const status = active ? '<span class="sessionStatus active">сейчас</span>'
    : (locked ? '<span class="sessionStatus locked">экран</span>' : '');
  const cls = active ? ' isActive' : (locked ? ' isLocked' : '');
  return `<details class="sessionCard${cls}" ${open ? 'open' : ''}>
    <summary class="sessionSummary">
      <span class="sessionChevron" aria-hidden="true"></span>
      <span class="sessionRange">${formatSessionRange(s)}</span>
      ${status}
      <span class="sessionDur">${formatDurationMs(s.duration_ms)}</span>
      ${lockedHint}
    </summary>
    <div class="sessionBody">${appsHtml}</div>
  </details>`;
}

function groupSessionsByUser(sessions) {
  const byUser = new Map();
  for (const s of sessions) {
    const user = s.username || '—';
    if (isHiddenActivityUser(user)) continue;
    if (!byUser.has(user)) byUser.set(user, []);
    byUser.get(user).push(s);
  }
  const groups = [];
  for (const [username, list] of byUser) {
    list.sort((a, b) => {
      const ka = sessionSortKey(a);
      const kb = sessionSortKey(b);
      if (ka !== kb) return ka - kb;
      return new Date(b.login) - new Date(a.login);
    });
    groups.push({ username, sessions: list });
  }
  groups.sort((a, b) => {
    const aActive = a.sessions.some(isSessionActive) ? 0 : 1;
    const bActive = b.sessions.some(isSessionActive) ? 0 : 1;
    if (aActive !== bActive) return aActive - bActive;
    return a.username.localeCompare(b.username, 'ru');
  });
  return groups;
}

function buildDayFocusSummaryByUser(sessions) {
  const byUser = new Map();
  for (const s of sessions) {
    const user = s.username || '—';
    if (isHiddenActivityUser(user)) continue;
    if (!byUser.has(user)) byUser.set(user, new Map());
    const byKey = byUser.get(user);
    for (const a of (s.apps || [])) {
      const focus = Number(a.focus_ms) || 0;
      if (focus <= 0) continue;
      const key = a.exe_path || a.app_name || '—';
      const cur = byKey.get(key) || { app_name: a.app_name || a.exe_path || '—', exe_path: a.exe_path || '', focus_ms: 0 };
      if (a.app_name) cur.app_name = a.app_name;
      cur.focus_ms += focus;
      byKey.set(key, cur);
    }
  }
  const users = [];
  for (const [username, byKey] of byUser) {
    const apps = Array.from(byKey.values()).sort((a, b) => b.focus_ms - a.focus_ms);
    const totalFocusMs = apps.reduce((sum, a) => sum + a.focus_ms, 0);
    users.push({ username, totalFocusMs, apps });
  }
  users.sort((a, b) => b.totalFocusMs - a.totalFocusMs || a.username.localeCompare(b.username, 'ru'));
  return users;
}

function renderDaySummary(sessions) {
  const users = buildDayFocusSummaryByUser(sessions);
  if (users.length === 0 || users.every(u => u.apps.length === 0)) {
    return `<div class="daySummary">
      <h4 class="daySummaryTitle">Сводка за день</h4>
      <p class="emptyHint">Нет данных по приложениям</p>
    </div>`;
  }
  return `<div class="daySummary">
    <h4 class="daySummaryTitle">Сводка за день</h4>
    <div class="daySummaryUsers">${users.map(u => `
      <div class="daySummaryUser">
        <div class="daySummaryTotal">
          <span class="daySummaryUserName">${escapeHtml(u.username)}</span>
          <span>в фокусе: <strong>${formatDurationMs(u.totalFocusMs)}</strong></span>
        </div>
        ${u.apps.length === 0
          ? '<p class="emptyHint">Нет фокуса</p>'
          : `<table class="activityTable">
              <thead><tr><th>Приложение</th><th>В фокусе</th></tr></thead>
              <tbody>${u.apps.map(a => `<tr>
                <td title="${escapeAttr(a.exe_path || '')}">${escapeHtml(a.app_name || '—')}</td>
                <td>${formatDurationMs(a.focus_ms)}</td>
              </tr>`).join('')}</tbody>
            </table>`}
      </div>`).join('')}
    </div>
  </div>`;
}

async function renderActivity() {
  const sessionsEl = document.getElementById('activitySessions');
  const summaryEl = document.getElementById('activityDaySummary');
  if (!currentClientId) {
    sessionsEl.innerHTML = '';
    if (summaryEl) summaryEl.innerHTML = '';
    return;
  }
  const dateInput = document.getElementById('activityDate');
  if (!dateInput.value) dateInput.value = todayISODate();
  try {
    const data = await getActivity(currentClientId, dateInput.value);
    const sessions = data.sessions || [];
    if (sessions.length === 0) {
      if (summaryEl) summaryEl.innerHTML = '';
      sessionsEl.innerHTML = '<p class="emptyHint">Нет сессий за этот день</p>';
      return;
    }
    if (summaryEl) summaryEl.innerHTML = renderDaySummary(sessions);
    const groups = groupSessionsByUser(sessions);
    if (groups.length === 0) {
      sessionsEl.innerHTML = '<p class="emptyHint">Нет сессий за этот день</p>';
      return;
    }
    sessionsEl.innerHTML = groups.map(g => {
      const hasActive = g.sessions.some(isSessionActive);
      return `<section class="sessionGroup">
        <h4 class="sessionGroupTitle">${escapeHtml(g.username)}</h4>
        <div class="sessionGroupList">
          ${g.sessions.map((s, i) => renderSessionCard(s, { open: hasActive ? isSessionActive(s) : i === 0 })).join('')}
        </div>
      </section>`;
    }).join('');
  } catch (e) {
    if (summaryEl) summaryEl.innerHTML = '';
    sessionsEl.innerHTML = '<p class="emptyHint">Не удалось загрузить активность</p>';
  }
}

function escapeHtml(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}
function escapeAttr(s) { return escapeHtml(s); }

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
  renderEarnSettings();
  const dateInput = document.getElementById('activityDate');
  if (!dateInput.value) dateInput.value = todayISODate();
  renderActivity();
}

function renderEarnSettings() {
  const box = document.getElementById('earnSettings');
  if (!box || !currentClient) return;
  const s = currentClient.earn_settings || {};
  const taskCount = currentClient.earn_task_count || 0;
  const earnURL = `${location.origin}/earn?client_id=${encodeURIComponent(currentClientId)}`;
  box.innerHTML = `
    <p class="configPreviewHint">Банк задачек: <strong>${taskCount}</strong> · генератор математики включён по умолчанию</p>
    <p class="configPreviewHint">Ребёнок / отладка из LAN: <a href="${earnURL}" target="_blank" rel="noopener">${earnURL}</a></p>
    <p class="configPreviewHint">Без client_id тоже работает: <a href="${location.origin}/earn" target="_blank" rel="noopener">${location.origin}/earn</a> (выбор ПК)</p>
    <label>Минут за задачу
      <input type="number" id="earnDefaultReward" min="1" max="120" value="${s.default_reward_minutes || 1}" class="smallInput">
    </label>
    <label>Лимит заработка в день (мин)
      <input type="number" id="earnMaxPerDay" min="1" max="600" value="${s.max_earn_per_day || 120}" class="smallInput">
    </label>
    <label>Блокировка после ошибки (сек)
      <input type="number" id="earnWrongLock" min="1" max="600" value="${s.wrong_lock_seconds || 15}" class="smallInput">
    </label>
    <label>Ошибок подряд → штраф и новая задача
      <input type="number" id="earnWrongStreak" min="1" max="20" value="${s.wrong_streak_limit || 3}" class="smallInput">
    </label>
    <label>Штраф (мин с баланса)
      <input type="number" id="earnWrongPenalty" min="0" max="60" value="${s.wrong_streak_penalty_minutes ?? 1}" class="smallInput">
    </label>
    <label class="checkboxLabel"><input type="checkbox" id="earnMathGen" ${s.math_generator_enabled !== false ? 'checked' : ''}> Генератор математики (3 класс)</label>
    <div class="earnActions">
      <button type="button" id="saveEarnSettings" class="primaryBtn">Сохранить награды</button>
      <button type="button" id="clearEarnBalances" class="dangerBtn">Стереть накопленное время</button>
    </div>
  `;
  document.getElementById('saveEarnSettings').onclick = async () => {
    await updateEarnSettings(currentClientId, {
      default_reward_minutes: Number(document.getElementById('earnDefaultReward').value) || 1,
      max_earn_per_day: Number(document.getElementById('earnMaxPerDay').value) || 120,
      wrong_lock_seconds: Number(document.getElementById('earnWrongLock').value) || 15,
      wrong_streak_limit: Number(document.getElementById('earnWrongStreak').value) || 3,
      wrong_streak_penalty_minutes: Number(document.getElementById('earnWrongPenalty').value) || 0,
      math_generator_enabled: document.getElementById('earnMathGen').checked
    });
    currentClient = await getClient(currentClientId);
    renderEarnSettings();
    renderUsers();
  };
  document.getElementById('clearEarnBalances').onclick = async () => {
    const users = currentClient.users || [];
    const total = users.reduce((sum, u) => sum + (u.earn_balance_minutes || 0), 0);
    const names = users.map((u) => `${u.name}: ${u.earn_balance_minutes || 0} мин`).join('\n');
    const msg = total === 0
      ? 'Балансы уже нулевые. Всё равно сбросить?'
      : `Стереть всё накопленное время (${total} мин)?\n\n${names}\n\nЭто нельзя отменить.`;
    if (!confirm(msg)) return;
    const res = await fetch(`${API}/clients/${currentClientId}/earn-balances/clear`, { method: 'POST' });
    if (!res.ok) {
      alert(await res.text() || 'Не удалось сбросить');
      return;
    }
    currentClient = await getClient(currentClientId);
    renderEarnSettings();
    renderUsers();
  };
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
          <span class="earnBalance">баланс: ${u.earn_balance_minutes || 0} мин</span>
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
      const ver = c.client_version ? ` [${c.client_version}]` : '';
      return `<option value="${c.id}">${c.name || c.id}${mark}${ver}</option>`;
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

const PASS_KEY = 'aegis_earn_admin_password';
const params = new URLSearchParams(location.search);
const clientId = params.get('client_id') || '';

const $ = (id) => document.getElementById(id);

let cache = null;
let subject = '';
let onlyCustom = false;

function headers(json) {
  const pw = sessionStorage.getItem(PASS_KEY) || '';
  const h = {};
  if (pw) h['X-Earn-Admin-Password'] = pw;
  if (json) h['Content-Type'] = 'application/json';
  return h;
}

function showEditorMsg(ok, msg) {
  const el = $('editorMsg');
  el.hidden = !msg;
  el.className = 'feedback ' + (ok ? 'ok' : 'bad');
  el.textContent = msg || '';
}

async function unlock() {
  $('lockError').hidden = true;
  const password = $('password').value;
  const res = await fetch('/api/earn-admin/unlock', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password })
  });
  if (!res.ok) {
    $('lockError').hidden = false;
    return;
  }
  sessionStorage.setItem(PASS_KEY, password);
  $('password').value = '';
  showBrowser();
  await loadBank();
}

function showBrowser() {
  $('lockGate').hidden = true;
  $('browser').hidden = false;
  const canEdit = Boolean(clientId);
  $('editor').hidden = !canEdit;
  $('noClientHint').hidden = canEdit;
}

async function loadBank() {
  $('stats').textContent = 'Загрузка…';
  $('list').innerHTML = '';
  const q = clientId ? `?client_id=${encodeURIComponent(clientId)}` : '';
  const res = await fetch(`/api/earn-admin/bank${q}`, { headers: headers() });
  if (res.status === 401) {
    sessionStorage.removeItem(PASS_KEY);
    $('lockGate').hidden = false;
    $('browser').hidden = true;
    return;
  }
  if (!res.ok) {
    $('stats').textContent = await res.text();
    return;
  }
  cache = await res.json();
  fillSubjectSelect();
  const subjects = cache.subjects || Object.keys(cache.by_subject || {});
  $('subjectTabs').innerHTML = [
    `<button type="button" class="tab ${subject === '' ? 'active' : ''}" data-subj="">все</button>`,
    ...subjects.map((s) =>
      `<button type="button" class="tab ${subject === s ? 'active' : ''}" data-subj="${escapeAttr(s)}">${escapeHtml(s)}</button>`
    )
  ].join('');
  $('subjectTabs').onclick = (e) => {
    const btn = e.target.closest('[data-subj]');
    if (!btn) return;
    subject = btn.dataset.subj;
    [...$('subjectTabs').querySelectorAll('.tab')].forEach((t) => {
      t.classList.toggle('active', t.dataset.subj === subject);
    });
    render();
  };
  render();
}

function fillSubjectSelect() {
  const sel = $('subjectSelect');
  if (!sel) return;
  const builtins = cache.builtin_subjects || cache.subjects || [];
  const extras = (cache.subjects || []).filter((s) => !builtins.includes(s));
  sel.innerHTML = [
    ...builtins.map((s) => `<option value="${escapeAttr(s)}">${escapeHtml(s)}</option>`),
    ...extras.map((s) => `<option value="${escapeAttr(s)}">${escapeHtml(s)}</option>`),
    '<option value="__new__">+ новый раздел…</option>'
  ].join('');
  sel.onchange = () => {
    $('subjectCustomWrap').hidden = sel.value !== '__new__';
  };
  sel.onchange();
}

function resetEditor() {
  $('taskId').value = '';
  $('editorTitle').textContent = 'Новый вопрос';
  $('prompt').value = '';
  $('answer').value = '';
  $('choices').value = '';
  $('reward').value = '1';
  if ($('subjectSelect').options.length) {
    $('subjectSelect').selectedIndex = 0;
    $('subjectSelect').onchange();
  }
  $('subjectCustom').value = '';
  showEditorMsg(true, '');
}

function editTask(t) {
  if (!t || !t.editable) return;
  $('taskId').value = t.id || '';
  $('editorTitle').textContent = 'Редактировать вопрос';
  $('prompt').value = t.prompt || '';
  $('answer').value = t.answer || '';
  $('choices').value = (t.choices || []).join('\n');
  $('reward').value = String(t.reward_minutes || 1);
  const sel = $('subjectSelect');
  const want = t.subject || '';
  let found = false;
  for (const opt of sel.options) {
    if (opt.value === want) {
      sel.value = want;
      found = true;
      break;
    }
  }
  if (!found && want) {
    sel.value = '__new__';
    $('subjectCustom').value = want;
  }
  sel.onchange();
  showEditorMsg(true, '');
  $('editor').scrollIntoView({ behavior: 'smooth', block: 'start' });
}

function currentSubject() {
  const sel = $('subjectSelect');
  if (sel.value === '__new__') {
    return ($('subjectCustom').value || '').trim();
  }
  return sel.value;
}

async function saveTask() {
  if (!clientId) {
    showEditorMsg(false, 'Нужен client_id');
    return;
  }
  const subjectName = currentSubject();
  const choices = ($('choices').value || '')
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean);
  const body = {
    id: ($('taskId').value || '').trim() || undefined,
    subject: subjectName,
    prompt: ($('prompt').value || '').trim(),
    answer: ($('answer').value || '').trim(),
    choices,
    reward_minutes: Number($('reward').value) || 1
  };
  const res = await fetch(`/api/clients/${encodeURIComponent(clientId)}/earn-tasks`, {
    method: 'POST',
    headers: headers(true),
    body: JSON.stringify(body)
  });
  if (res.status === 401) {
    sessionStorage.removeItem(PASS_KEY);
    location.reload();
    return;
  }
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    showEditorMsg(false, data.message || await res.text() || 'Не сохранено');
    return;
  }
  showEditorMsg(true, 'Сохранено');
  resetEditor();
  await loadBank();
  if (data.subject) {
    subject = data.subject;
  }
}

async function deleteTask(id) {
  if (!clientId || !id) return;
  if (!confirm('Удалить этот вопрос?')) return;
  const res = await fetch(`/api/clients/${encodeURIComponent(clientId)}/earn-tasks/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: headers()
  });
  if (res.status === 401) {
    sessionStorage.removeItem(PASS_KEY);
    location.reload();
    return;
  }
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    alert(data.message || 'Не удалось удалить');
    return;
  }
  if ($('taskId').value === id) resetEditor();
  await loadBank();
}

function render() {
  if (!cache) return;
  const q = ($('search').value || '').trim().toLowerCase();
  const by = cache.by_subject || {};
  const parts = Object.keys(by).map((k) => `${k}: ${by[k]}`);
  let tasks = cache.tasks || [];
  if (onlyCustom) tasks = tasks.filter((t) => t.editable);
  if (subject) tasks = tasks.filter((t) => t.subject === subject);
  if (q) {
    tasks = tasks.filter((t) =>
      String(t.prompt || '').toLowerCase().includes(q) ||
      String(t.answer || '').toLowerCase().includes(q) ||
      String(t.id || '').toLowerCase().includes(q)
    );
  }
  const customN = cache.custom_count || 0;
  $('stats').textContent =
    `Показано ${tasks.length} из ${cache.task_count || 0}` +
    (customN ? ` (своих: ${customN})` : '') +
    `. ${parts.join(' · ')}. ` +
    (cache.math_note || '');
  if (!tasks.length) {
    $('list').innerHTML = '<p class="empty">Ничего не найдено</p>';
    return;
  }
  $('list').innerHTML = tasks.map((t) => {
    const choices = (t.choices || []).length
      ? `<div class="choices">Варианты (${t.choices.length}): ${escapeHtml((t.choices || []).join(' · '))}</div>`
      : '';
    const badge = t.editable
      ? '<span class="badge mine">мой</span>'
      : '<span class="badge builtin">встроенный</span>';
    const actions = t.editable
      ? `<div class="itemActions">
          <button type="button" class="smallBtn" data-edit="${escapeAttr(t.id)}">Изменить</button>
          <button type="button" class="smallBtn danger" data-del="${escapeAttr(t.id)}">Удалить</button>
        </div>`
      : '';
    return `<details class="item ${t.editable ? 'isMine' : ''}">
      <summary>${badge} <strong>${escapeHtml(t.subject || '')}</strong> — ${escapeHtml(t.prompt || '')}</summary>
      <div class="meta">${escapeHtml(t.id)} · ${t.reward_minutes || 1} мин · ${escapeHtml(t.kind || '')}</div>
      <div class="answer">Ответ: ${escapeHtml(t.answer || '')}</div>
      ${choices}
      ${actions}
    </details>`;
  }).join('');

  $('list').onclick = (e) => {
    const edit = e.target.closest('[data-edit]');
    if (edit) {
      const id = edit.getAttribute('data-edit');
      const task = (cache.tasks || []).find((x) => x.id === id);
      editTask(task);
      return;
    }
    const del = e.target.closest('[data-del]');
    if (del) {
      deleteTask(del.getAttribute('data-del'));
    }
  };
}

function escapeHtml(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function escapeAttr(s) {
  return escapeHtml(s).replace(/'/g, '&#39;');
}

$('unlockBtn').onclick = () => unlock().catch((e) => {
  $('lockError').hidden = false;
  $('lockError').textContent = e.message || 'Ошибка';
});
$('password').addEventListener('keydown', (e) => {
  if (e.key === 'Enter') {
    e.preventDefault();
    $('unlockBtn').click();
  }
});
$('search').addEventListener('input', () => render());
$('onlyCustom').addEventListener('change', (e) => {
  onlyCustom = e.target.checked;
  render();
});
$('saveTaskBtn').onclick = () => saveTask().catch((e) => showEditorMsg(false, e.message || 'Ошибка'));
$('resetTaskBtn').onclick = () => resetEditor();

if (sessionStorage.getItem(PASS_KEY)) {
  showBrowser();
  loadBank().catch(() => {
    sessionStorage.removeItem(PASS_KEY);
    $('lockGate').hidden = false;
    $('browser').hidden = true;
  });
}

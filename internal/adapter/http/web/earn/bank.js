const PASS_KEY = 'aegis_earn_admin_password';
const params = new URLSearchParams(location.search);
const clientId = params.get('client_id') || '';

const $ = (id) => document.getElementById(id);

let cache = null;
let subject = '';

function headers() {
  const pw = sessionStorage.getItem(PASS_KEY) || '';
  return pw ? { 'X-Earn-Admin-Password': pw } : {};
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
  const subjects = cache.subjects || Object.keys(cache.by_subject || {});
  $('subjectTabs').innerHTML = [
    `<button type="button" class="tab ${subject === '' ? 'active' : ''}" data-subj="">все</button>`,
    ...subjects.map((s) =>
      `<button type="button" class="tab ${subject === s ? 'active' : ''}" data-subj="${s}">${s}</button>`
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

function render() {
  if (!cache) return;
  const q = ($('search').value || '').trim().toLowerCase();
  const by = cache.by_subject || {};
  const parts = Object.keys(by).map((k) => `${k}: ${by[k]}`);
  let tasks = cache.tasks || [];
  if (subject) tasks = tasks.filter((t) => t.subject === subject);
  if (q) {
    tasks = tasks.filter((t) =>
      String(t.prompt || '').toLowerCase().includes(q) ||
      String(t.answer || '').toLowerCase().includes(q) ||
      String(t.id || '').toLowerCase().includes(q)
    );
  }
  $('stats').textContent =
    `Показано ${tasks.length} из ${cache.task_count || 0}. ${parts.join(' · ')}. ` +
    (cache.math_note || '');
  if (!tasks.length) {
    $('list').innerHTML = '<p class="empty">Ничего не найдено</p>';
    return;
  }
  $('list').innerHTML = tasks.map((t) => {
    const choices = (t.choices || []).length
      ? `<div class="choices">Варианты (${t.choices.length}): ${escapeHtml(shuffleArray(t.choices).join(' · '))}</div>`
      : '';
    return `<details class="item">
      <summary><strong>${escapeHtml(t.subject || '')}</strong> — ${escapeHtml(t.prompt || '')}</summary>
      <div class="meta">${escapeHtml(t.id)} · ${t.reward_minutes || 1} мин · ${escapeHtml(t.kind || '')}</div>
      <div class="answer">Ответ: ${escapeHtml(t.answer || '')}</div>
      ${choices}
    </details>`;
  }).join('');
}

function shuffleArray(arr) {
  const out = Array.from(arr || []);
  for (let i = out.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    const tmp = out[i];
    out[i] = out[j];
    out[j] = tmp;
  }
  return out;
}

function escapeHtml(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
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

if (sessionStorage.getItem(PASS_KEY)) {
  showBrowser();
  loadBank().catch(() => {
    sessionStorage.removeItem(PASS_KEY);
    $('lockGate').hidden = false;
    $('browser').hidden = true;
  });
}

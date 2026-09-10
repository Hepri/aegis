const params = new URLSearchParams(location.search);
let clientId = params.get('client_id') || '';
let userId = params.get('user_id') || '';
let balance = 0;
let currentTask = null;

const $ = (id) => document.getElementById(id);

async function api(path, opts) {
  const res = await fetch(path, opts);
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || res.statusText);
  }
  const ct = res.headers.get('content-type') || '';
  if (ct.includes('application/json')) return res.json();
  return null;
}

function setBalance(n) {
  balance = n || 0;
  $('balance').textContent = String(balance);
  document.querySelectorAll('[data-redeem]').forEach((btn) => {
    const m = Number(btn.dataset.redeem);
    btn.disabled = balance < m;
  });
}

function showFeedback(el, ok, msg) {
  el.hidden = false;
  el.className = 'feedback ' + (ok ? 'ok' : 'bad');
  el.textContent = msg;
}

async function loadState() {
  if (!clientId) {
    $('clientName').textContent = 'Укажите client_id в адресе';
    return;
  }
  const q = new URLSearchParams({ client_id: clientId });
  if (userId) q.set('user_id', userId);
  const state = await api(`/api/earn/state?${q}`);
  $('clientName').textContent = state.client_name || '';

  if (!userId) {
    if ((state.users || []).length === 1) {
      userId = state.users[0].id;
    } else {
      $('userPick').hidden = false;
      $('userButtons').innerHTML = (state.users || []).map((u) =>
        `<button type="button" class="userBtn" data-uid="${u.id}">${u.name}</button>`
      ).join('');
      $('userButtons').onclick = (e) => {
        const btn = e.target.closest('[data-uid]');
        if (!btn) return;
        userId = btn.dataset.uid;
        $('userPick').hidden = true;
        bootUser(state);
      };
      return;
    }
  }
  bootUser(state);
}

function bootUser(state) {
  $('mainPanel').hidden = false;
  const u = (state.users || []).find((x) => x.id === userId) || state.user;
  setBalance(u ? u.balance_minutes : 0);

  const opts = state.redeem_options || [5, 15, 30];
  $('redeemButtons').innerHTML = opts.map((m) =>
    `<button type="button" data-redeem="${m}">${m} мин</button>`
  ).join('');
  $('redeemButtons').onclick = async (e) => {
    const btn = e.target.closest('[data-redeem]');
    if (!btn || btn.disabled) return;
    const minutes = Number(btn.dataset.redeem);
    try {
      const res = await api('/api/earn/redeem', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ client_id: clientId, user_id: userId, minutes })
      });
      setBalance(res.balance_minutes);
      showFeedback($('redeemMsg'), true, res.message || `Куплено ${minutes} мин`);
    } catch (err) {
      showFeedback($('redeemMsg'), false, err.message || 'Не удалось купить');
    }
  };

  loadNextTask();
}

async function loadNextTask() {
  $('answerFeedback').hidden = true;
  const task = await api(`/api/earn/next?client_id=${encodeURIComponent(clientId)}&user_id=${encodeURIComponent(userId)}`);
  if (!task || task.empty) {
    currentTask = null;
    $('emptyTasks').hidden = false;
    $('taskContent').hidden = true;
    return;
  }
  currentTask = task;
  $('emptyTasks').hidden = true;
  $('taskContent').hidden = false;
  $('taskPrompt').textContent = task.prompt;
  $('taskReward').textContent = String(task.reward_minutes);
  $('answerInput').value = '';
  $('answerInput').focus();
}

$('answerForm').addEventListener('submit', async (e) => {
  e.preventDefault();
  if (!currentTask) return;
  const answer = $('answerInput').value;
  try {
    const res = await api('/api/earn/answer', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        client_id: clientId,
        user_id: userId,
        task_id: currentTask.id,
        answer
      })
    });
    setBalance(res.balance_minutes);
    if (res.correct) {
      showFeedback($('answerFeedback'), true, 'Верно!');
      setTimeout(loadNextTask, 600);
    } else {
      showFeedback($('answerFeedback'), false, 'Неверно, попробуй ещё');
    }
  } catch (err) {
    showFeedback($('answerFeedback'), false, err.message || 'Ошибка');
  }
});

loadState().catch((err) => {
  $('clientName').textContent = err.message || 'Ошибка загрузки';
});

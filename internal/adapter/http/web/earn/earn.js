const params = new URLSearchParams(location.search);
let clientId = params.get('client_id') || '';
let userId = params.get('user_id') || '';
let exitUrl = params.get('exit_url') || 'http://127.0.0.1:17855/exit';
let balance = 0;
let currentTask = null;
let earnSettings = {};
let lockTimer = null;

const $ = (id) => document.getElementById(id);

function setQuery(extra) {
  const q = new URLSearchParams(location.search);
  if (clientId) q.set('client_id', clientId); else q.delete('client_id');
  if (userId) q.set('user_id', userId); else q.delete('user_id');
  if (exitUrl) q.set('exit_url', exitUrl);
  Object.entries(extra || {}).forEach(([k, v]) => {
    if (v == null || v === '') q.delete(k);
    else q.set(k, v);
  });
  const next = `${location.pathname}?${q}`;
  history.replaceState(null, '', next);
}

async function doExit() {
  const buttons = [$('exitBtn'), $('exitBtnBottom')].filter(Boolean);
  buttons.forEach((btn) => {
    btn.disabled = true;
    btn.textContent = 'Выход…';
  });
  try {
    await fetch(exitUrl, { method: 'POST', mode: 'cors' });
  } catch (_) {
    try {
      await fetch('http://127.0.0.1:17855/exit', { method: 'POST', mode: 'cors' });
    } catch (__) {
      try { location.href = exitUrl; } catch (___) {}
      buttons.forEach((btn) => {
        btn.disabled = false;
        btn.textContent = btn.id === 'exitBtnBottom' ? 'Выйти из Задачек' : 'Выйти';
      });
      alert('Выход доступен только на ПК в аккаунте «Задачки».');
    }
  }
}

$('exitBtn').onclick = doExit;
if ($('exitBtnBottom')) $('exitBtnBottom').onclick = doExit;

async function api(path, opts) {
  const res = await fetch(path, opts);
  const ct = res.headers.get('content-type') || '';
  const isJSON = ct.includes('application/json');
  const body = isJSON ? await res.json() : await res.text();
  if (!res.ok) {
    if (isJSON && body && typeof body === 'object') {
      const err = new Error(body.message || res.statusText || 'error');
      err.status = res.status;
      err.payload = body;
      throw err;
    }
    const err = new Error(typeof body === 'string' ? body : res.statusText);
    err.status = res.status;
    throw err;
  }
  return isJSON ? body : null;
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

function streakMax() {
  return Number(earnSettings.wrong_streak_limit) || 3;
}

function updateWrongUI(count) {
  $('wrongCount').textContent = String(count || 0);
  $('wrongMax').textContent = String(streakMax());
}

function clearLockTimer() {
  if (lockTimer) {
    clearInterval(lockTimer);
    lockTimer = null;
  }
}

function setAnswerEnabled(on) {
  $('answerBtn').disabled = !on;
  $('answerInput').disabled = !on;
  const skip = $('skipBtn');
  if (skip) skip.disabled = !on;
  $('choiceButtons').querySelectorAll('button').forEach((b) => { b.disabled = !on; });
}

function startLock(seconds, hint) {
  const sec = Math.max(0, Math.ceil(Number(seconds) || 0));
  if (sec <= 0) {
    $('lockBanner').hidden = true;
    setAnswerEnabled(true);
    return;
  }
  clearLockTimer();
  $('lockBanner').hidden = false;
  $('lockHint').textContent = hint || 'Подожди';
  let left = sec;
  $('lockSeconds').textContent = String(left);
  setAnswerEnabled(false);
  lockTimer = setInterval(() => {
    left -= 1;
    if (left <= 0) {
      clearLockTimer();
      $('lockBanner').hidden = true;
      setAnswerEnabled(true);
      return;
    }
    $('lockSeconds').textContent = String(left);
  }, 1000);
}

function renderChoices(choices) {
  const box = $('choiceButtons');
  const input = $('answerInput');
  if (choices && choices.length) {
    const shuffled = shuffleArray(choices);
    box.hidden = false;
    input.hidden = true;
    input.required = false;
    box.innerHTML = shuffled.map((c) =>
      `<button type="button" class="choiceBtn" data-choice="${String(c).replace(/"/g, '&quot;')}">${c}</button>`
    ).join('');
    box.onclick = (e) => {
      const btn = e.target.closest('[data-choice]');
      if (!btn || btn.disabled) return;
      submitAnswer(btn.dataset.choice);
    };
  } else {
    box.hidden = true;
    box.innerHTML = '';
    input.hidden = false;
    input.required = true;
  }
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

async function pickClient() {
  $('clientPick').hidden = false;
  $('userPick').hidden = true;
  $('mainPanel').hidden = true;
  const clients = await api('/api/clients');
  const list = Array.isArray(clients) ? clients : [];
  if (list.length === 0) {
    $('clientButtons').innerHTML = '<p class="muted">Нет клиентов — сначала добавь компьютер в админке</p>';
    return;
  }
  if (list.length === 1) {
    clientId = list[0].id;
    setQuery();
    $('clientPick').hidden = true;
    await loadState();
    return;
  }
  $('clientButtons').innerHTML = list.map((c) =>
    `<button type="button" class="userBtn" data-cid="${c.id}">${c.name || c.id}</button>`
  ).join('');
  $('clientButtons').onclick = async (e) => {
    const btn = e.target.closest('[data-cid]');
    if (!btn) return;
    clientId = btn.dataset.cid;
    setQuery();
    $('clientPick').hidden = true;
    await loadState();
  };
}

async function loadState() {
  if (!clientId) {
    $('clientName').textContent = 'Локальная отладка';
    await pickClient();
    return;
  }
  const q = new URLSearchParams({ client_id: clientId });
  if (userId) q.set('user_id', userId);
  const state = await api(`/api/earn/state?${q}`);
  earnSettings = state.earn_settings || {};
  $('clientName').textContent = state.client_name || clientId;

  if (!userId) {
    if ((state.users || []).length === 1) {
      userId = state.users[0].id;
      setQuery();
    } else {
      $('userPick').hidden = false;
      $('userButtons').innerHTML = (state.users || []).map((u) =>
        `<button type="button" class="userBtn" data-uid="${u.id}">${u.name}</button>`
      ).join('');
      $('userButtons').onclick = (e) => {
        const btn = e.target.closest('[data-uid]');
        if (!btn) return;
        userId = btn.dataset.uid;
        setQuery();
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
  updateWrongUI(u ? u.wrong_count : 0);
  earnSettings = state.earn_settings || earnSettings || {};

  const redeemOn = state.redeem_enabled !== false && !(earnSettings && earnSettings.disable_redeem);
  const redeemBox = document.querySelector('.redeem');
  if (redeemBox) redeemBox.hidden = !redeemOn;

  if (redeemOn) {
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
  }

  if (u && u.lock_seconds > 0) {
    startLock(u.lock_seconds);
  }
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
  const subj = task.subject || '';
  $('taskSubject').textContent = subj;
  $('taskSubject').hidden = !subj;
  $('taskPrompt').textContent = task.prompt;
  $('taskReward').textContent = String(task.reward_minutes);
  updateWrongUI(task.wrong_count || 0);
  $('answerInput').value = '';
  renderChoices(task.choices);
  if (task.lock_seconds > 0) {
    startLock(task.lock_seconds);
  } else {
    setAnswerEnabled(true);
    if (!task.choices || !task.choices.length) $('answerInput').focus();
  }
}

async function submitAnswer(answer) {
  if (!currentTask) return;
  const text = String(answer ?? '').trim();
  if (!text) return;
  try {
    const res = await api('/api/earn/answer', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        client_id: clientId,
        user_id: userId,
        task_id: currentTask.id,
        answer: text
      })
    });
    setBalance(res.balance_minutes);
    updateWrongUI(res.wrong_count);
    if (res.correct) {
      showFeedback($('answerFeedback'), true, `Верно! +${res.reward_minutes || 0} мин`);
      setTimeout(loadNextTask, 500);
      return;
    }
    let msg = 'Неверно';
    if (res.replace_question) {
      msg = `Слишком много ошибок (−${res.penalty_minutes || 0} мин). Новая задачка.`;
    } else if (res.wrong_count && res.wrong_streak_max) {
      msg = `Неверно (${res.wrong_count}/${res.wrong_streak_max})`;
    }
    showFeedback($('answerFeedback'), false, msg);
    if (res.lock_seconds > 0) {
      startLock(res.lock_seconds, res.replace_question ? 'Новая задачка через' : 'Подожди');
    }
    if (res.replace_question) {
      setTimeout(loadNextTask, (res.lock_seconds || 0) * 1000 + 50);
    }
  } catch (err) {
    if (err.status === 423 && err.payload) {
      setBalance(err.payload.balance_minutes);
      updateWrongUI(err.payload.wrong_count);
      startLock(err.payload.lock_seconds || 0);
      showFeedback($('answerFeedback'), false, 'Ещё рано отвечать');
      return;
    }
    showFeedback($('answerFeedback'), false, err.message || 'Ошибка');
  }
}

$('answerForm').addEventListener('submit', async (e) => {
  e.preventDefault();
  if (!currentTask) return;
  await submitAnswer($('answerInput').value);
});

$('skipBtn').addEventListener('click', async () => {
  if (!currentTask) return;
  const penalty = Number(earnSettings.wrong_streak_penalty_minutes);
  const lockSec = Number(earnSettings.wrong_lock_seconds) || 15;
  const penaltyText = penalty > 0 ? `−${penalty} мин с баланса` : 'без штрафа минут';
  const ok = confirm(
    `Точно хочешь другую задачу?\n\nЭто как ${earnSettings.wrong_streak_limit || 3} ошибки подряд: ${penaltyText} и пауза ${lockSec} с.`
  );
  if (!ok) return;
  try {
    const res = await api('/api/earn/skip', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        client_id: clientId,
        user_id: userId,
        task_id: currentTask.id
      })
    });
    setBalance(res.balance_minutes);
    updateWrongUI(res.wrong_count);
    const pen = res.penalty_minutes || 0;
    showFeedback(
      $('answerFeedback'),
      false,
      pen > 0 ? `Другая задача (−${pen} мин), подожди` : 'Другая задача после паузы'
    );
    if (res.lock_seconds > 0) {
      startLock(res.lock_seconds, 'Новая задачка через');
    }
    setTimeout(loadNextTask, (res.lock_seconds || 0) * 1000 + 50);
  } catch (err) {
    if (err.status === 423 && err.payload) {
      setBalance(err.payload.balance_minutes);
      startLock(err.payload.lock_seconds || 0);
      showFeedback($('answerFeedback'), false, 'Ещё рано');
      return;
    }
    showFeedback($('answerFeedback'), false, err.message || 'Ошибка');
  }
});

loadState().catch((err) => {
  $('clientName').textContent = err.message || 'Ошибка загрузки';
});

'use strict';
/* SIP Santral web panel — core: helpers, API, layout, router. Pages live in
   pages-*.js and register themselves in App.pages. */
const App = window.App = { pages: {}, cleanups: [], status: null, footer: {} };

// ---------------------------------------------------------------------------
// DOM helpers
// ---------------------------------------------------------------------------

function h(tag, props, ...kids) {
  const el = document.createElement(tag);
  if (props) {
    for (const [k, v] of Object.entries(props)) {
      if (v === undefined || v === null || v === false) continue;
      if (k === 'class') el.className = v;
      else if (k === 'style' && typeof v === 'object') Object.assign(el.style, v);
      else if (k.startsWith('on') && typeof v === 'function') el.addEventListener(k.slice(2).toLowerCase(), v);
      else if (k === 'value') el.value = v;
      else if (k === 'checked') el.checked = !!v;
      else if (k === 'disabled') el.disabled = !!v;
      else if (k === 'html') el.innerHTML = v;
      else el.setAttribute(k, v === true ? '' : String(v));
    }
  }
  add(el, kids);
  return el;
}
function add(el, kids) {
  for (const k of kids) {
    if (k === null || k === undefined || k === false) continue;
    if (Array.isArray(k)) add(el, k);
    else el.append(k instanceof Node ? k : document.createTextNode(String(k)));
  }
}
App.h = h;
// set replaces an element's children, skipping null/false like h() does.
App.set = (el, ...kids) => { el.replaceChildren(); add(el, kids); return el; };

const ICONS = {
  grid: '<rect x="3" y="3" width="7" height="9" rx="1.5"/><rect x="14" y="3" width="7" height="5" rx="1.5"/><rect x="14" y="12" width="7" height="9" rx="1.5"/><rect x="3" y="16" width="7" height="5" rx="1.5"/>',
  phoneIn: '<polyline points="16 2 16 8 22 8"/><line x1="23" y1="1" x2="16" y2="8"/><path d="M22 16.92v3a2 2 0 0 1-2.18 2 19.79 19.79 0 0 1-8.63-3.07 19.5 19.5 0 0 1-6-6 19.79 19.79 0 0 1-3.07-8.67A2 2 0 0 1 4.11 2h3a2 2 0 0 1 2 1.72 12.84 12.84 0 0 0 .7 2.81 2 2 0 0 1-.45 2.11L8.09 9.91a16 16 0 0 0 6 6l1.27-1.27a2 2 0 0 1 2.11-.45 12.84 12.84 0 0 0 2.81.7A2 2 0 0 1 22 16.92z"/>',
  phoneMissed: '<line x1="23" y1="1" x2="17" y2="7"/><line x1="17" y1="1" x2="23" y2="7"/><path d="M22 16.92v3a2 2 0 0 1-2.18 2 19.79 19.79 0 0 1-8.63-3.07 19.5 19.5 0 0 1-6-6 19.79 19.79 0 0 1-3.07-8.67A2 2 0 0 1 4.11 2h3a2 2 0 0 1 2 1.72 12.84 12.84 0 0 0 .7 2.81 2 2 0 0 1-.45 2.11L8.09 9.91a16 16 0 0 0 6 6l1.27-1.27a2 2 0 0 1 2.11-.45 12.84 12.84 0 0 0 2.81.7A2 2 0 0 1 22 16.92z"/>',
  megaphone: '<path d="m3 11 18-5v12L3 14v-3z"/><path d="M11.6 16.8a3 3 0 1 1-5.8-1.6"/>',
  wave: '<path d="M2 10v3"/><path d="M6 6v11"/><path d="M10 3v18"/><path d="M14 8v7"/><path d="M18 5v13"/><path d="M22 10v3"/>',
  phone: '<rect x="5" y="2" width="14" height="20" rx="2"/><line x1="12" y1="18" x2="12.01" y2="18"/>',
  sliders: '<line x1="4" y1="21" x2="4" y2="14"/><line x1="4" y1="10" x2="4" y2="3"/><line x1="12" y1="21" x2="12" y2="12"/><line x1="12" y1="8" x2="12" y2="3"/><line x1="20" y1="21" x2="20" y2="16"/><line x1="20" y1="12" x2="20" y2="3"/><line x1="1" y1="14" x2="7" y2="14"/><line x1="9" y1="8" x2="15" y2="8"/><line x1="17" y1="16" x2="23" y2="16"/>',
  plus: '<line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/>',
  search: '<circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>',
  upload: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/>',
  download: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/>',
  play: '<polygon points="7 4 20 12 7 20 7 4" fill="currentColor" stroke="none"/>',
  pause: '<rect x="6" y="4" width="4" height="16" fill="currentColor" stroke="none"/><rect x="14" y="4" width="4" height="16" fill="currentColor" stroke="none"/>',
  stop: '<rect x="6" y="6" width="12" height="12" rx="2" fill="currentColor" stroke="none"/>',
  mic: '<path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/><path d="M19 10v2a7 7 0 0 1-14 0v-2"/><line x1="12" y1="19" x2="12" y2="23"/><line x1="8" y1="23" x2="16" y2="23"/>',
  type: '<polyline points="4 7 4 4 20 4 20 7"/><line x1="9" y1="20" x2="15" y2="20"/><line x1="12" y1="4" x2="12" y2="20"/>',
  edit: '<path d="M12 20h9"/><path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"/>',
  trash: '<polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>',
  refresh: '<polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>',
  eye: '<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/>',
  copy: '<rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>',
  chevron: '<polyline points="9 18 15 12 9 6"/>',
  check: '<polyline points="20 6 9 17 4 12"/>',
  alert: '<circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>',
  info: '<circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/>',
  x: '<line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>',
  lock: '<rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>',
  volume: '<polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/><path d="M15.54 8.46a5 5 0 0 1 0 7.07"/>',
  folder: '<path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/>',
  calendar: '<rect x="3" y="4" width="18" height="18" rx="2"/><line x1="16" y1="2" x2="16" y2="6"/><line x1="8" y1="2" x2="8" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/>',
};
function icon(name, cls) {
  const s = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  s.setAttribute('viewBox', '0 0 24 24');
  s.setAttribute('fill', 'none');
  s.setAttribute('stroke', 'currentColor');
  s.setAttribute('stroke-width', '1.8');
  s.setAttribute('stroke-linecap', 'round');
  s.setAttribute('stroke-linejoin', 'round');
  s.setAttribute('aria-hidden', 'true');
  s.setAttribute('class', 'ic' + (cls ? ' ' + cls : ''));
  s.innerHTML = ICONS[name] || '';
  return s;
}
App.icon = icon;

// ---------------------------------------------------------------------------
// Formatting
// ---------------------------------------------------------------------------

const pad2 = (n) => String(n).padStart(2, '0');
App.fmt = {
  time(iso) { const d = new Date(iso); return pad2(d.getHours()) + ':' + pad2(d.getMinutes()); },
  date(iso) { const d = new Date(iso); return pad2(d.getDate()) + '.' + pad2(d.getMonth() + 1) + '.' + d.getFullYear(); },
  dateShort(iso) { const d = new Date(iso); return pad2(d.getDate()) + '.' + pad2(d.getMonth() + 1); },
  isToday(iso) { const d = new Date(iso), n = new Date(); return d.toDateString() === n.toDateString(); },
  dur(sec) {
    sec = Math.max(0, Math.round(sec || 0));
    const hh = Math.floor(sec / 3600), mm = Math.floor(sec / 60) % 60, ss = sec % 60;
    return hh > 0 ? hh + ':' + pad2(mm) + ':' + pad2(ss) : pad2(mm) + ':' + pad2(ss);
  },
  durLong(sec) {
    sec = Math.round(sec || 0);
    const hh = Math.floor(sec / 3600), mm = Math.floor(sec / 60) % 60;
    if (hh > 0) return hh + ' sa ' + mm + ' dk';
    if (mm > 0) return mm + ' dk';
    return sec + ' sn';
  },
  ms(ms) { const s = Math.round((ms || 0) / 1000); return Math.floor(s / 60) + ':' + pad2(s % 60); },
  bytes(n) {
    n = n || 0;
    const u = ['B', 'KB', 'MB', 'GB', 'TB'];
    let i = 0;
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
    return (i === 0 ? String(n) : n.toFixed(n >= 100 ? 0 : 1).replace('.', ',')) + ' ' + u[i];
  },
  ago(iso) {
    if (!iso || iso.startsWith('0001')) return '';
    const s = (Date.now() - new Date(iso)) / 1000;
    if (s < 60) return 'az önce';
    if (s < 3600) return Math.floor(s / 60) + ' dk önce';
    if (s < 172800) return Math.floor(s / 3600) + ' saat önce';
    return Math.floor(s / 86400) + ' gün önce';
  },
};

App.MODES = [['ring', 'Çaldır'], ['annring', 'Anons + Çaldır'], ['ann', 'Sadece anons'], ['busy', 'Çalıp meşgul']];
App.modeText = Object.fromEntries(App.MODES);
App.resultText = { answered: 'Cevaplandı', missed: 'Cevapsız', voicemail: 'Sesli mesaj', announcement: 'Sadece anons', busy: 'Meşgul verildi', interrupted: 'Yarıda kaldı', live: 'Sürüyor' };
App.lineState = (st) => ({
  registered: { cls: 'ok', text: 'Kayıtlı' },
  failed: { cls: 'fail', text: 'Hata' },
  trying: { cls: 'warn', text: 'Bağlanıyor' },
  disabled: { cls: 'off', text: 'Kapalı' },
}[st && st.state] || { cls: 'off', text: 'Bilinmiyor' });

App.flowText = (l) => {
  const exts = (l.exts || []).join(', ') || '—';
  if (l.mode === 'annring') return '“' + (l.annName || '?') + '” → ' + exts + ' çalar';
  if (l.mode === 'ann') return '“' + (l.annName || '?') + '” → ' + (l.after === 'vm' ? 'sesli mesaj' : 'kapat');
  if (l.mode === 'busy') return (l.ringSec || 30) + ' sn çalar → meşgul';
  return '→ ' + exts + ' çalar';
};

App.liveText = (c) => ({
  started: 'Bağlanıyor',
  announcement: 'Anons çalıyor',
  ringing: c.targets ? 'Çalıyor · ' + c.targets.split(' ').join(', ') : 'Çalıyor (dahili yok)',
  answered: 'Konuşuyor · ' + (c.by || '') + (c.byName ? ' ' + c.byName : ''),
  noanswer: 'Kimse açmadı',
  voicemail: 'Mesaj bırakıyor',
  busy: 'Meşgul veriliyor',
}[c.state] || c.state);

App.slug = (s) => {
  const map = { 'ç': 'c', 'Ç': 'C', 'ğ': 'g', 'Ğ': 'G', 'ı': 'i', 'İ': 'I', 'ö': 'o', 'Ö': 'O', 'ş': 's', 'Ş': 'S', 'ü': 'u', 'Ü': 'U', 'ä': 'a', 'Ä': 'A', 'ß': 'ss' };
  let out = '';
  let dash = false;
  for (const ch of (s || '')) {
    const c = map[ch] !== undefined ? map[ch] : ch;
    if (/^[A-Za-z0-9._]+$/.test(c)) { out += c; dash = false; } else if (!dash && out) { out += '-'; dash = true; }
  }
  out = out.replace(/^[-.]+|[-.]+$/g, '');
  return out || 'hat';
};

// ---------------------------------------------------------------------------
// API
// ---------------------------------------------------------------------------

async function api(method, path, body, opts = {}) {
  const init = { method, headers: { 'X-Requested-With': 'sipsantral' }, credentials: 'same-origin' };
  if (body instanceof FormData) init.body = body;
  else if (body !== undefined) {
    init.headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(body);
  }
  const r = await fetch(path, init);
  if (r.status === 401 && !opts.noAuth) {
    App.showAuth();
    throw new Error('Oturum süresi doldu, yeniden giriş yapın');
  }
  if (opts.raw) {
    if (!r.ok) {
      const d = await r.json().catch(() => ({}));
      throw new Error(d.error || 'HTTP ' + r.status);
    }
    return r;
  }
  const data = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(data.error || 'HTTP ' + r.status);
  return data;
}
App.api = api;

// ---------------------------------------------------------------------------
// Toasts, modals, clipboard, downloads
// ---------------------------------------------------------------------------

App.toast = (msg, kind) => {
  const t = h('div', { class: 'toast' + (kind ? ' ' + kind : ''), role: kind === 'err' ? 'alert' : 'status' },
    icon(kind === 'err' ? 'alert' : kind === 'ok' ? 'check' : 'info'), h('span', null, msg));
  document.getElementById('toasts').append(t);
  setTimeout(() => t.remove(), kind === 'err' ? 7000 : 3500);
};
App.err = (e) => App.toast(e && e.message ? e.message : String(e), 'err');

App.modal = ({ title, body, actions = [], wide = false, onClose }) => {
  const root = document.getElementById('modal-root');
  const prevFocus = document.activeElement;
  let closed = false;
  const close = () => {
    if (closed) return;
    closed = true;
    back.remove();
    document.body.classList.remove('noscroll');
    document.removeEventListener('keydown', onKey);
    if (onClose) onClose();
    if (prevFocus && prevFocus.focus) prevFocus.focus();
  };
  const onKey = (e) => { if (e.key === 'Escape') close(); };
  const foot = h('div', { class: 'modal-foot' });
  for (const a of actions) {
    const b = h('button', { type: 'button', class: 'btn' + (a.primary ? ' primary' : '') + (a.danger ? ' danger' : '') }, a.label);
    b.addEventListener('click', async () => {
      if (!a.onClick) return close();
      b.disabled = true;
      try {
        const keep = await a.onClick(close);
        if (keep !== true) close();
      } catch (e) { App.err(e); } finally { b.disabled = false; }
    });
    foot.append(b);
  }
  const titleId = 'm' + Math.random().toString(36).slice(2);
  const dlg = h('div', { class: 'modal' + (wide ? ' wide' : ''), role: 'dialog', 'aria-modal': 'true', 'aria-labelledby': titleId },
    h('div', { class: 'modal-head' }, h('h2', { id: titleId }, title),
      h('button', { type: 'button', class: 'icon-btn', 'aria-label': 'Kapat', onclick: close }, icon('x'))),
    h('div', { class: 'modal-body' }, body), actions.length ? foot : null);
  const back = h('div', { class: 'modal-back', onmousedown: (e) => { if (e.target === back) close(); } }, dlg);
  root.append(back);
  document.body.classList.add('noscroll');
  document.addEventListener('keydown', onKey);
  setTimeout(() => {
    const f = dlg.querySelector('input,select,textarea,.modal-foot .primary');
    (f || dlg).focus && (f || dlg).focus();
  }, 20);
  return { close, el: dlg };
};

App.confirm = (text, { title = 'Emin misiniz?', ok = 'Evet', danger = false } = {}) => new Promise((resolve) => {
  let answered = false;
  App.modal({
    title, body: h('p', null, text),
    actions: [
      { label: 'Vazgeç', onClick: () => { answered = true; resolve(false); } },
      { label: ok, primary: !danger, danger, onClick: () => { answered = true; resolve(true); } },
    ],
    onClose: () => { if (!answered) resolve(false); },
  });
});

App.prompt = (title, label, value) => new Promise((resolve) => {
  const inp = h('input', { class: 'input', value: value || '' });
  let done = false;
  App.modal({
    title, body: h('label', { class: 'field' }, label, inp),
    actions: [
      { label: 'Vazgeç', onClick: () => { done = true; resolve(null); } },
      { label: 'Kaydet', primary: true, onClick: () => { done = true; resolve(inp.value); } },
    ],
    onClose: () => { if (!done) resolve(null); },
  });
  inp.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); inp.closest('.modal').querySelector('.btn.primary').click(); } });
});

App.copy = async (text) => {
  try {
    await navigator.clipboard.writeText(text);
  } catch (e) {
    const ta = h('textarea', { style: { position: 'fixed', opacity: '0' } }, text);
    document.body.append(ta);
    ta.select();
    document.execCommand('copy');
    ta.remove();
  }
  App.toast('Kopyalandı', 'ok');
};

App.saveBlob = (blob, name) => {
  const url = URL.createObjectURL(blob);
  const a = h('a', { href: url, download: name });
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 5000);
};

App.switch = (checked, label, onChange, disabled) => {
  const b = h('button', { type: 'button', class: 'switch', role: 'switch', 'aria-checked': String(!!checked), 'aria-label': label, disabled },
    h('span', { class: 'track', 'aria-hidden': 'true' }, h('span', { class: 'knob' })));
  b.addEventListener('click', async () => {
    const next = b.getAttribute('aria-checked') !== 'true';
    b.setAttribute('aria-checked', String(next));
    try {
      if (onChange) await onChange(next);
    } catch (e) {
      b.setAttribute('aria-checked', String(!next));
      App.err(e);
    }
  });
  return b;
};

App.seg = (options, value, onPick, { cls = '', disabled = false, label = '' } = {}) => {
  const box = h('div', { class: 'seg ' + cls, role: 'group', 'aria-label': label });
  for (const [v, text] of options) {
    box.append(h('button', {
      type: 'button', 'aria-pressed': String(v === value), disabled,
      onclick: () => {
        for (const b of box.children) b.setAttribute('aria-pressed', 'false');
        box.querySelector(`[data-v="${CSS.escape(v)}"]`).setAttribute('aria-pressed', 'true');
        onPick(v);
      }, 'data-v': v,
    }, text));
  }
  return box;
};

// ---------------------------------------------------------------------------
// Polling and page lifecycle
// ---------------------------------------------------------------------------

App.poll = (fn, ms) => {
  let busy = false;
  const t = setInterval(async () => {
    if (document.hidden || busy) return;
    busy = true;
    try { await fn(); } catch (e) { /* shown on next manual action */ } finally { busy = false; }
  }, ms);
  App.cleanups.push(() => clearInterval(t));
};
App.onLeave = (fn) => App.cleanups.push(fn);

// ---------------------------------------------------------------------------
// Auth screens
// ---------------------------------------------------------------------------

App.showAuth = async () => {
  App.leave();
  let st = App.status;
  try { st = await api('GET', '/api/status', undefined, { noAuth: true }); App.status = st; } catch (e) { /* keep */ }
  const setup = st && st.setupRequired;
  const pw = h('input', { class: 'input', type: 'password', autocomplete: setup ? 'new-password' : 'current-password', required: true, id: 'pw' });
  const pw2 = setup ? h('input', { class: 'input', type: 'password', autocomplete: 'new-password', id: 'pw2' }) : null;
  const err = h('p', { class: 'err-text', role: 'alert' });
  const btn = h('button', { type: 'submit', class: 'btn primary' }, setup ? 'Parolayı belirle' : 'Giriş yap');
  const form = h('form', { class: 'card' },
    h('div', { class: 'brand' }, h('img', { src: '/favicon.ico', alt: '' }), h('div', null, h('b', null, 'SIP Santral'), h('small', null, st ? st.hostname : ''))),
    setup ? h('p', null, 'İlk kurulum: panele girmek için bir yönetici parolası belirleyin (en az 8 karakter).') : null,
    h('label', { class: 'field', for: 'pw' }, setup ? 'Yeni parola' : 'Parola', pw),
    setup ? h('label', { class: 'field', for: 'pw2' }, 'Parola (tekrar)', pw2) : null,
    err, btn);
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    err.textContent = '';
    if (setup && pw.value !== pw2.value) { err.textContent = 'Parolalar aynı değil'; return; }
    btn.disabled = true;
    try {
      await api(setup ? 'POST' : 'POST', setup ? '/api/setup' : '/api/login', { password: pw.value }, { noAuth: true });
      App.status = await api('GET', '/api/status', undefined, { noAuth: true });
      App.route();
    } catch (ex) {
      err.textContent = ex.message;
    } finally { btn.disabled = false; }
  });
  document.getElementById('app').replaceChildren(h('div', { class: 'auth' }, form));
  pw.focus();
};

App.logout = async () => {
  try { await api('POST', '/api/logout'); } catch (e) { /* ignore */ }
  App.status.authenticated = false;
  App.showAuth();
};

// ---------------------------------------------------------------------------
// Layout and router
// ---------------------------------------------------------------------------

const NAV = [
  ['pano', 'Pano', 'grid'],
  ['hatlar', 'Hatlar', 'phoneIn'],
  ['anonslar', 'Anonslar', 'megaphone'],
  ['aramalar', 'Aramalar ve kayıtlar', 'wave'],
  ['dahililer', 'Dahililer', 'phone'],
  ['ayarlar', 'Ayarlar', 'sliders'],
];

App.layout = (active) => {
  let shell = document.querySelector('.layout');
  if (!shell) {
    const nav = h('nav', { class: 'nav', 'aria-label': 'Ana menü' });
    for (const [id, text, ic] of NAV) nav.append(h('a', { href: '#/' + id, 'data-id': id }, icon(ic), h('span', null, text)));
    const foot = h('div', { class: 'side-foot' }, h('span', { id: 'foot-ver' }), h('button', { type: 'button', onclick: App.logout }, 'Çıkış'));
    shell = h('div', { class: 'layout' },
      h('aside', { class: 'side' },
        h('div', { class: 'brand' }, h('img', { src: '/favicon.ico', alt: '' }),
          h('div', null, h('b', null, 'SIP Santral'), h('small', null, 'ASUSTOR NAS'))),
        nav, foot),
      h('main', { class: 'main', id: 'main' }));
    document.getElementById('app').replaceChildren(shell);
  }
  for (const a of shell.querySelectorAll('.nav a')) {
    const on = a.dataset.id === active;
    a.classList.toggle('active', on);
    if (on) a.setAttribute('aria-current', 'page'); else a.removeAttribute('aria-current');
  }
  App.setFooter();
  return document.getElementById('main');
};

App.setFooter = (info) => {
  if (info) Object.assign(App.footer, info);
  const el = document.getElementById('foot-ver');
  if (!el) return;
  const v = App.status ? 'v' + App.status.version : '';
  el.textContent = v + (App.footer.asterisk ? ' · Asterisk ' + App.footer.asterisk.split(' ')[0] : '');
};

App.leave = () => {
  for (const f of App.cleanups.splice(0)) { try { f(); } catch (e) { /* ignore */ } }
};

App.params = () => {
  const q = location.hash.split('?')[1] || '';
  return Object.fromEntries(new URLSearchParams(q));
};

App.route = async () => {
  App.leave();
  if (!App.status) {
    try { App.status = await api('GET', '/api/status', undefined, { noAuth: true }); } catch (e) {
      document.getElementById('app').replaceChildren(h('div', { class: 'boot' }, 'Panele ulaşılamıyor: ' + e.message));
      return;
    }
  }
  if (!App.status.authenticated) { App.showAuth(); return; }
  const hash = location.hash.split('?')[0] || '#/pano';
  let m;
  let page = 'pano', arg = null;
  if ((m = hash.match(/^#\/hat\/([\w-]+)$/))) { page = 'hat'; arg = m[1]; } else if ((m = hash.match(/^#\/(\w+)$/)) && App.pages[m[1]]) page = m[1];
  const main = App.layout(page === 'hat' ? 'hatlar' : page);
  main.replaceChildren(h('div', { class: 'boot' }, 'Yükleniyor…'));
  window.scrollTo(0, 0);
  try {
    await App.pages[page](main, arg);
  } catch (e) {
    main.replaceChildren(h('div', { class: 'page' }, h('div', { class: 'callout err' }, icon('alert'), h('span', null, 'Sayfa yüklenemedi: ' + e.message))));
  }
};

window.addEventListener('hashchange', App.route);
document.addEventListener('DOMContentLoaded', App.route);

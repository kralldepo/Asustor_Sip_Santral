'use strict';
/* Pano, Hatlar, Hat düzenle */
(() => {
  const { h, icon, api, fmt } = App;

  // ------------------------------------------------------------------ Pano
  App.pages.pano = async (main) => {
    const root = h('div', { class: 'page' });
    let data = null;

    const statusPill = (pbx) => {
      const map = {
        running: ['ok', 'Santral çalışıyor'], preparing: ['warn', 'Hazırlanıyor'], starting: ['warn', 'Başlatılıyor'],
        error: ['fail', 'Santral hatası'], disabled: ['off', 'Asterisk kapalı'], stopped: ['off', 'Durdu'],
      };
      const [cls, text] = map[pbx.state] || ['off', pbx.state];
      return h('span', { class: 'pill ' + cls }, h('span', { class: 'dot ' + cls }), text);
    };

    const setMode = async (line, mode) => {
      if ((mode === 'annring' || mode === 'ann') && !line.annId) {
        App.toast('“' + line.name + '” için önce bir anons seçin (Hat düzenle)', 'err');
        render();
        return;
      }
      try {
        const l = await api('POST', '/api/lines/' + line.id + '/mode', { mode });
        Object.assign(line, l);
        App.toast(line.name + ': ' + App.modeText[mode], 'ok');
      } catch (e) { App.err(e); }
      render();
    };

    const kpi = (label, value, unit, sub) => h('div', { class: 'card kpi' },
      h('span', { class: 'kpi-label' }, label),
      h('span', { class: 'kpi-value' }, h('b', null, value), unit ? h('span', { class: 'muted' }, unit) : null), sub);

    function render() {
      const d = data;
      const pbx = d.pbx;
      const upText = [];
      if (pbx.asteriskVersion) upText.push('Asterisk ' + pbx.asteriskVersion.split(' ')[0]);
      if (pbx.state === 'running' && pbx.since) upText.push('son başlatma ' + fmt.ago(pbx.since));
      if (pbx.message && pbx.state !== 'running') upText.push(pbx.message);
      if (pbx.pendingRestart) upText.push('ayar değişikliği için aramalar bitince yeniden başlayacak');
      App.setFooter({ asterisk: pbx.asteriskVersion || '' });

      const head = h('div', { class: 'page-head' },
        h('div', { class: 'stack' }, h('h1', null, 'Pano'),
          h('div', { class: 'row wrap muted' }, statusPill(pbx), h('span', null, upText.join(' · ')))),
        h('div', { class: 'actions' },
          h('a', { class: 'btn', href: '#/aramalar' }, 'Tüm aramalar'),
          h('a', { class: 'btn primary', href: '#/hat/new' }, icon('plus'), 'Hat ekle')));

      const c = d.counts;
      const problems = [];
      if (c.failed) problems.push(c.failed + ' hata');
      if (c.disabled) problems.push(c.disabled + ' kapalı');
      const t = d.today;
      const st = d.storage;
      const pct = st.used + st.free > 0 ? Math.max(0.5, (st.used / (st.used + st.free)) * 100) : 0;
      const kpis = h('section', { class: 'kpis', 'aria-label': 'Bugünün özeti' },
        kpi('Hatlar', String(c.registered), '/ ' + c.total + ' kayıtlı',
          problems.length ? h('a', { href: '#/hatlar', class: 'kpi-sub', style: { color: c.failed ? 'var(--fail)' : '' } }, problems.join(' · ') + ' · incele')
            : h('span', { class: 'kpi-sub' }, c.total ? 'Hepsi sorunsuz' : 'Henüz hat yok')),
        kpi('Bugün gelen', String(t.total), 'arama', h('span', { class: 'kpi-sub' }, t.answered + ' cevaplandı · ' + t.missed + ' cevapsız · ' + t.voicemail + ' mesaj')),
        kpi('Bugün kaydedilen', fmt.durLong(t.recSec), '', h('span', { class: 'kpi-sub' }, t.files + ' dosya · ' + fmt.bytes(t.recBytes))),
        kpi('Kayıt alanı', fmt.bytes(st.used), 'kullanılıyor', [
          h('div', { class: 'bar', 'aria-hidden': 'true' }, h('div', { style: { width: pct.toFixed(1) + '%' } })),
          h('span', { class: 'kpi-sub' }, (st.root || '').split('/').pop() + ' · ' + fmt.bytes(st.free) + ' boş'),
        ]));

      // lines
      const lineList = h('ul', { class: 'list' });
      const shown = d.lines.slice(0, 8);
      for (const l of shown) {
        const s = App.lineState(l.status);
        lineList.append(h('li', { class: 'line-row' },
          h('div', { class: 'info' },
            h('div', { class: 'row wrap' }, h('b', null, l.name), h('span', { class: 'mono small muted' }, l.number)),
            h('div', { class: 'meta' },
              h('span', { class: 'status ' + s.cls, title: l.status.detail || '' }, h('span', { class: 'dot ' + s.cls }), s.text),
              l.status.state === 'failed' && l.status.detail ? h('span', { class: 'small', style: { color: 'var(--fail)' } }, l.status.detail) : null,
              h('span', { class: 'muted' }, App.flowText(l)))),
          App.seg(App.MODES, l.mode, (m) => setMode(l, m),
            { cls: 'tall', disabled: !l.enabled, label: l.name + ': gelen arama modu' }),
          h('span', { class: 'today' }, h('b', { class: 'mono' }, String(l.today)), h('span', null, 'bugün'))));
      }
      const linesCard = h('section', { class: 'card col-3', 'aria-labelledby': 'pano-hatlar' },
        h('div', { class: 'card-head' },
          h('div', { class: 'stack' }, h('h2', { id: 'pano-hatlar' }, 'Hatlar · gelen arama modu'),
            h('p', null, 'Modu değiştir; bir sonraki arama yeni moda göre karşılanır.')),
          h('a', { href: '#/hatlar', class: 'small' }, d.lines.length + ' hattın tümü')),
        d.lines.length ? lineList : h('div', { class: 'empty' }, 'Henüz hat yok. ', h('a', { href: '#/hat/new' }, 'İlk hattı ekleyin'), ' ya da ', h('a', { href: '#/hatlar' }, 'toplu içe aktarın'), '.'),
        d.lines.length > shown.length ? h('div', { class: 'card-foot' }, h('span', null, (d.lines.length - shown.length) + ' hat daha var · ', h('a', { href: '#/hatlar' }, 'Hatlar sayfasında gör'))) : null);

      // live calls
      const liveList = h('ul', { class: 'list' });
      for (const c2 of d.live) {
        const color = c2.state === 'answered' ? 'var(--ok)' : c2.state === 'ringing' ? 'var(--warn)' : 'var(--primary)';
        liveList.append(h('li', { class: 'call-row' },
          h('div', { class: 'who' }, h('span', { class: 'mono', style: { fontWeight: '600' } }, c2.caller),
            h('span', { class: 'small muted' }, '→ ' + c2.lineName + ' · ', h('b', { style: { color } }, App.liveText(c2)))),
          h('div', { class: 'stack', style: { alignItems: 'flex-end' } },
            h('span', { class: 'mono', style: { fontWeight: '600', fontSize: '15px' } }, fmt.dur(c2.seconds)),
            c2.recording ? h('span', { class: 'rec' }, 'KAYIT') : null)));
      }
      const liveCard = h('section', { class: 'card', 'aria-labelledby': 'pano-canli' },
        h('div', { class: 'card-head' }, h('h2', { id: 'pano-canli' }, 'Canlı aramalar'), h('span', { class: 'count' }, String(d.live.length))),
        d.live.length ? liveList : h('div', { class: 'empty' }, 'Şu an arama yok.'));

      // recent calls
      const recList = h('ul', { class: 'list' });
      for (const r of d.recent) {
        const resColor = { answered: 'var(--ok)', missed: 'var(--warn)', interrupted: 'var(--warn)', voicemail: 'var(--primary)', announcement: 'var(--text-2)', busy: 'var(--busy)' }[r.result];
        recList.append(h('li', { class: 'call-row' },
          r.rec ? h('a', { class: 'icon-btn round', href: '#/aramalar?range=all&id=' + encodeURIComponent(r.id), 'aria-label': 'Kaydı dinle: ' + r.caller + ', saat ' + fmt.time(r.start) }, icon('play'))
            : h('span', { class: 'missed-ic', 'aria-hidden': 'true' }, icon('phoneMissed')),
          h('div', { class: 'who' },
            h('span', { class: 'top' }, h('span', { class: 'mono', style: { fontWeight: '600' } }, r.caller), h('span', { class: 'mono muted' }, r.rec ? fmt.dur(r.recSec || r.durSec) : '—')),
            h('span', { class: 'small muted' }, h('span', { class: 'mono' }, fmt.isToday(r.start) ? fmt.time(r.start) : fmt.dateShort(r.start) + ' ' + fmt.time(r.start)), ' · ' + r.lineName + ' · ',
              h('b', { style: { color: resColor } }, App.resultText[r.result] + (r.answeredBy ? ' · ' + r.answeredBy : ''))))));
      }
      const recentCard = h('section', { class: 'card', 'aria-labelledby': 'pano-son' },
        h('div', { class: 'card-head' }, h('h2', { id: 'pano-son' }, 'Son aramalar'), h('a', { href: '#/aramalar', class: 'small' }, 'Tümü')),
        d.recent.length ? recList : h('div', { class: 'empty' }, 'Henüz arama yok.'));

      root.replaceChildren(head, kpis, h('div', { class: 'cols' }, linesCard, h('div', { class: 'col-2' }, liveCard, recentCard)));
    }

    data = await api('GET', '/api/dashboard');
    render();
    main.replaceChildren(root);
    App.poll(async () => { data = await api('GET', '/api/dashboard'); render(); }, 3000);
  };

  // ------------------------------------------------------------------ Hatlar
  App.pages.hatlar = async (main) => {
    const root = h('div', { class: 'page' });
    let d = await api('GET', '/api/lines');
    let filter = 'all';
    let query = '';
    const tableBox = h('div');
    const chipsBox = h('div', { class: 'filters', role: 'group', 'aria-label': 'Duruma göre süz' });

    const cols = '44px 116px minmax(220px, 1.4fr) minmax(220px, 1.6fr) 110px 70px 56px 96px';
    const selected = new Set();
    const bulkBox = h('div');

    async function bulkAction(action, changes) {
      const ids = [...selected];
      if (action === 'delete' && !await App.confirm(ids.length + ' hat silinsin mi? Geçmiş aramalar ve kayıtlar silinmez.', { ok: 'Sil', danger: true })) return;
      if (action === 'disable' && !await App.confirm(ids.length + ' hat kapatılsın mı? Kapalı hat sağlayıcıya kayıt olmaz ve arama almaz.', { ok: 'Kapat' })) return;
      const r = await api('POST', '/api/lines/bulk', { ids, action, changes });
      App.toast(r.count + ' hat ' + { update: 'güncellendi', enable: 'açıldı', disable: 'kapatıldı', delete: 'silindi' }[action], 'ok');
      if (action === 'delete') selected.clear();
      d = await api('GET', '/api/lines');
      renderTable();
    }
    const safely = (fn) => async () => { try { await fn(); } catch (e) { App.err(e); } };

    function bulkEdit() {
      const ch = {};
      const extSet = new Set(d.exts.map((e) => e.num));
      const summary = h('p', { class: 'small', style: { color: 'var(--text-2)' } });
      const refresh = () => {
        extBox.hidden = !('exts' in ch);
        const n = Object.keys(ch).length;
        summary.textContent = n ? n + ' alan, seçili ' + selected.size + ' hatta uygulanacak.' : 'Yalnız “değiştir” dediğiniz alanlar seçili ' + selected.size + ' hatta uygulanır.';
      };
      const pick = (label, key, options, conv) => h('label', { class: 'field' }, label,
        h('select', { class: 'input', onchange: (e) => { if (e.target.value === '') delete ch[key]; else ch[key] = conv ? conv(e.target.value) : e.target.value; refresh(); } },
          h('option', { value: '' }, '— Değiştirme —'), ...options.map(([v, t]) => h('option', { value: v }, t))));
      const anns = d.anns.map((a) => [a.id, a.name]);
      const onOff = [['1', 'Açık'], ['0', 'Kapalı']];
      const ringInp = h('input', { class: 'input mono', type: 'number', min: '5', max: '300', placeholder: '— Değiştirme —' });
      ringInp.addEventListener('input', () => { if (ringInp.value) ch.ringSec = parseInt(ringInp.value, 10) || 0; else delete ch.ringSec; refresh(); });
      const extBox = h('div', { class: 'row wrap', hidden: true }, ...d.exts.map((e) => {
        const box = h('label', { class: 'check-chip on' }, h('input', { type: 'checkbox', checked: true, onchange: (ev) => {
          if (ev.target.checked) extSet.add(e.num); else extSet.delete(e.num);
          box.classList.toggle('on', ev.target.checked);
        } }), h('b', { class: 'mono' }, e.num), h('span', null, e.name));
        return box;
      }));
      const body = h('div', { class: 'stack', style: { gap: '14px' } }, summary,
        h('div', { class: 'grid-form' },
          pick('Gelen arama modu', 'mode', App.MODES),
          pick('Anons', 'annId', anns),
          h('label', { class: 'field' }, 'Çalma süresi (sn)', ringInp),
          pick('Kimse açmazsa', 'noAnswer', [['hang', 'Kapat'], ['busy', 'Meşgul ver'], ['ann', 'Anons çal'], ['vm', 'Sesli mesaj al']]),
          pick('Cevapsız arama anonsu', 'noAnsAnnId', anns),
          pick('Anonstan sonra', 'after', [['hang', 'Kapat'], ['vm', 'Sesli mesaj al']]),
          pick('Kayıt', 'record', onOff, (v) => v === '1'),
          pick('Arayanın önüne hat adı', 'cidPrefix', onOff, (v) => v === '1')),
        h('fieldset', null, h('legend', null, 'Çalacak dahililer'),
          h('div', { class: 'stack', style: { gap: '8px' } },
            h('select', { class: 'input', onchange: (e) => { if (e.target.value) ch.exts = []; else delete ch.exts; refresh(); } },
              h('option', { value: '' }, '— Değiştirme —'), h('option', { value: 'set' }, 'Şu dahililer çalsın')),
            d.exts.length ? extBox : h('p', { class: 'small muted' }, 'Henüz dahili yok.'))),
        h('p', { class: 'small muted' }, '“Çalıp meşgul” modunda dahili gerekmez. Anonslu modlar için anons da seçin; eksik ayarlı hat olursa hiçbir hat değişmez.'));
      refresh();
      App.modal({
        title: 'Toplu düzenle · ' + selected.size + ' hat', body, wide: true,
        actions: [{ label: 'Vazgeç' }, { label: 'Uygula', primary: true, onClick: async () => {
          if ('exts' in ch) ch.exts = [...extSet];
          if (!Object.keys(ch).length) throw new Error('Değiştirilecek bir alan seçin');
          await bulkAction('update', ch);
        } }],
      });
    }

    function renderBulk() {
      for (const id of [...selected]) if (!d.lines.some((l) => l.id === id)) selected.delete(id);
      if (!selected.size) { bulkBox.replaceChildren(); return; }
      App.set(bulkBox, h('div', { class: 'bulk-bar', role: 'region', 'aria-label': 'Toplu işlem' },
        h('b', null, selected.size + ' hat seçildi'),
        h('div', { class: 'actions' },
          h('button', { type: 'button', class: 'btn small primary', onclick: bulkEdit }, icon('edit'), 'Toplu düzenle'),
          h('button', { type: 'button', class: 'btn small', onclick: safely(() => bulkAction('enable')) }, 'Aç'),
          h('button', { type: 'button', class: 'btn small', onclick: safely(() => bulkAction('disable')) }, 'Kapat'),
          h('button', { type: 'button', class: 'btn small danger', onclick: safely(() => bulkAction('delete')) }, icon('trash'), 'Sil'),
          h('button', { type: 'button', class: 'linkish', onclick: () => { selected.clear(); renderTable(); } }, 'Seçimi kaldır'))));
    }

    function renderTable() {
      const counts = { all: d.lines.length, registered: 0, failed: 0, disabled: 0 };
      for (const l of d.lines) if (counts[l.status.state] !== undefined) counts[l.status.state]++;
      chipsBox.replaceChildren(...[['all', 'Tümü'], ['registered', 'Kayıtlı'], ['failed', 'Hata'], ['disabled', 'Kapalı']].map(([k, t]) =>
        h('button', { type: 'button', 'aria-pressed': String(filter === k), onclick: () => { filter = k; renderTable(); } }, t, h('span', { class: 'mono muted' }, String(counts[k])))));
      const q = query.toLowerCase();
      const rows = d.lines.filter((l) => (filter === 'all' || l.status.state === filter) &&
        (!q || [l.name, l.number, l.username, l.server].some((x) => (x || '').toLowerCase().includes(q))));
      const all = h('input', { type: 'checkbox', 'aria-label': 'Görünen hatların tümünü seç', checked: rows.length > 0 && rows.every((l) => selected.has(l.id)) });
      all.addEventListener('change', () => { for (const l of rows) { if (all.checked) selected.add(l.id); else selected.delete(l.id); } renderTable(); });
      const tbl = h('div', { class: 'tbl tbl-lines', role: 'table', 'aria-label': 'Hatlar' },
        h('div', { class: 'tr th', role: 'row', style: { gridTemplateColumns: cols } },
          h('span', { role: 'columnheader', class: 'row', style: { justifyContent: 'center' } }, all),
          ...['Durum', 'Hat', 'Gelen arama', 'Çalacaklar', 'Kayıt'].map((t) => h('span', { role: 'columnheader' }, t)),
          h('span', { role: 'columnheader', class: 'right' }, 'Bugün'), h('span', { role: 'columnheader' }, h('span', { class: 'sr-only' }, 'İşlem'))));
      for (const l of rows) {
        const s = App.lineState(l.status);
        let detail = '';
        if (l.mode === 'annring') detail = '“' + (l.annName || '?') + '”, sonra çalar';
        else if (l.mode === 'ann') detail = '“' + (l.annName || '?') + '” → ' + (l.after === 'vm' ? 'sesli mesaj' : 'kapat');
        else if (l.mode === 'busy') detail = l.ringSec + ' sn çalar, sonra meşgul';
        else detail = 'doğrudan çalar';
        const cb = h('input', { type: 'checkbox', 'aria-label': l.name + ' hattını seç', checked: selected.has(l.id) });
        cb.addEventListener('change', () => { // update in place so keyboard focus stays on the box
          if (cb.checked) selected.add(l.id); else selected.delete(l.id);
          cb.closest('.tr').classList.toggle('sel', cb.checked);
          all.checked = rows.length > 0 && rows.every((x) => selected.has(x.id));
          renderBulk();
        });
        const ringsExts = (l.mode === 'ring' || l.mode === 'annring') && l.exts.length;
        tbl.append(h('div', { class: 'tr' + (selected.has(l.id) ? ' sel' : ''), role: 'row', style: { gridTemplateColumns: cols } },
          h('div', { role: 'cell', class: 'row', style: { justifyContent: 'center', height: '44px' } }, cb),
          h('div', { role: 'cell', class: 'cell-2' },
            h('span', { class: 'status ' + s.cls }, h('span', { class: 'dot ' + s.cls }), s.text),
            l.status.detail ? h('span', { class: 'small', style: { color: 'var(--fail)' } }, l.status.detail) : null),
          h('div', { role: 'cell', class: 'cell-2' },
            h('span', { class: 'row wrap' }, h('b', null, l.name), h('span', { class: 'mono small muted' }, l.number)),
            h('span', { class: 'sub mono' }, l.server + ' · ' + l.username)),
          h('div', { role: 'cell', class: 'row wrap' }, h('span', { class: 'tag ' + l.mode }, App.modeText[l.mode]), h('span', { class: 'small muted' }, detail)),
          h('div', { role: 'cell', class: 'row wrap', style: { gap: '4px' } },
            ringsExts ? l.exts.map((e) => h('span', { class: 'chip mono' }, e)) : h('span', { class: 'muted' }, '—')),
          h('div', { role: 'cell', class: 'status ' + (l.record && l.mode !== 'busy' ? '' : 'off') },
            l.mode === 'busy' ? '—' : l.record ? [h('span', { class: 'dot fail' }), 'Açık'] : 'Kapalı'),
          h('div', { role: 'cell', class: 'mono right', style: { fontWeight: '600' } }, String(l.today)),
          h('div', { role: 'cell', class: 'right' }, h('a', { class: 'btn small', href: '#/hat/' + l.id, 'aria-label': l.name + ' hattını düzenle' }, 'Düzenle'))));
      }
      if (!rows.length) tbl.append(h('div', { class: 'empty' }, d.lines.length ? 'Bu süzgece uyan hat yok.' : 'Henüz hat yok. “Hat ekle” ya da aşağıdan toplu içe aktarın.'));
      renderBulk();
      tableBox.replaceChildren(bulkBox, h('div', { class: 'tbl-wrap' }, tbl));
    }

    // ---- bulk import
    const imp = { mode: 'busy', annId: '', exts: d.exts.map((e) => e.num) };
    const ta = h('textarea', { class: 'input mono', rows: '7', placeholder: 'Satış 2;+49 221 555 0109;sip.saglayici.de;2215550109;şifre\nSatış 3;+49 221 555 0110;sip.saglayici.de;2215550110;şifre' });
    const preview = h('div', { class: 'stack' });
    const previewWrap = h('div', { class: 'callout plain', hidden: true, style: { flexDirection: 'column', gap: '6px' } }, preview);
    new MutationObserver(() => { previewWrap.hidden = !preview.childNodes.length; }).observe(preview, { childList: true });
    const defaultsBox = h('div', { class: 'stack', style: { gap: '12px' } });
    const importBtn = h('button', { type: 'button', class: 'btn primary', disabled: true }, 'İçe aktar');
    const fileInp = h('input', { type: 'file', accept: '.csv,.txt,text/csv,text/plain', class: 'sr-only' });
    fileInp.addEventListener('change', async () => {
      const f = fileInp.files[0];
      if (f) { ta.value = await f.text(); runPreview(); }
    });
    function renderDefaults() {
      const extChips = d.exts.map((e) => {
        const on = imp.exts.includes(e.num);
        return h('label', { class: 'check-chip' + (on ? ' on' : '') },
          h('input', { type: 'checkbox', checked: on, onchange: (ev) => { imp.exts = ev.target.checked ? [...imp.exts, e.num] : imp.exts.filter((x) => x !== e.num); renderDefaults(); } }),
          h('b', { class: 'mono' }, e.num), h('span', null, e.name));
      });
      App.set(defaultsBox,
        h('label', { class: 'field' }, 'Yeni hatlarda gelen arama',
          h('select', { class: 'input', onchange: (e) => { imp.mode = e.target.value; renderDefaults(); } },
            ...App.MODES.map(([k, t]) => h('option', { value: k, selected: imp.mode === k ? 'selected' : null }, t)))),
        imp.mode === 'busy' ? h('p', { class: 'small muted' }, 'Dahili gerekmez: arayan 30 sn çalma sesi duyar, sonra meşgul. Sonradan hatları seçip toplu düzenleyebilirsiniz.') : null,
        imp.mode === 'annring' || imp.mode === 'ann' ? h('label', { class: 'field' }, 'Anons',
          h('select', { class: 'input', onchange: (e) => { imp.annId = e.target.value; } },
            h('option', { value: '' }, d.anns.length ? 'Anons seçin' : 'Önce Anonslar sayfasında anons ekleyin'),
            ...d.anns.map((a) => h('option', { value: a.id, selected: imp.annId === a.id ? 'selected' : null }, a.name)))) : null,
        imp.mode === 'ring' || imp.mode === 'annring' ? h('fieldset', null, h('legend', null, 'Çalacak dahililer'),
          d.exts.length ? h('div', { class: 'row wrap' }, extChips) : h('p', { class: 'small muted' }, 'Önce ', h('a', { href: '#/dahililer' }, 'dahili ekleyin'), '.')) : null);
    }
    let lastValid = 0;
    async function runPreview(commit) {
      if (!ta.value.trim()) { preview.replaceChildren(); importBtn.disabled = true; return; }
      try {
        const r = await api('POST', '/api/lines/import', { text: ta.value, mode: imp.mode, annId: imp.annId, exts: imp.exts, dryRun: !commit });
        lastValid = r.valid;
        if (commit) {
          App.toast(r.added + ' hat eklendi', 'ok');
          ta.value = '';
          preview.replaceChildren();
          importBtn.disabled = true;
          d = await api('GET', '/api/lines');
          renderTable();
          return;
        }
        const ok = r.rows.filter((x) => x.ok).length;
        preview.replaceChildren(
          h('b', { class: 'small' }, 'Önizleme'),
          h('span', { class: 'row small', style: { color: 'var(--ok)' } }, icon('check'), h('span', null, h('b', null, ok + ' yeni hat'), ' eklenmeye hazır')),
          ...r.rows.filter((x) => !x.ok).map((x) => h('span', { class: 'row small', style: { color: 'var(--warn)', alignItems: 'flex-start' } }, icon('alert'),
            h('span', null, h('b', null, 'Satır ' + x.row + (x.name ? ' (' + x.name + ')' : '')), ' atlanacak: ' + x.error))));
        importBtn.disabled = ok === 0;
        importBtn.textContent = ok ? ok + ' hattı ekle' : 'İçe aktar';
      } catch (e) {
        preview.replaceChildren(h('span', { class: 'err-text' }, e.message));
        importBtn.disabled = true;
      }
    }
    let pt = null;
    ta.addEventListener('input', () => { clearTimeout(pt); pt = setTimeout(() => runPreview(false), 500); });
    importBtn.addEventListener('click', async () => {
      if (!lastValid) return;
      importBtn.disabled = true;
      await runPreview(true);
    });
    renderDefaults();

    const searchInp = h('input', { type: 'search', class: 'input', placeholder: 'Ad, numara, sunucu ya da kullanıcı adı', 'aria-label': 'Hatlarda ara' });
    searchInp.addEventListener('input', () => { query = searchInp.value; renderTable(); });

    root.append(
      h('div', { class: 'page-head' },
        h('div', null, h('h1', null, 'Hatlar'), h('p', null, 'SIP hesapların. İstediğin kadar hat ekle; her hattın gelen arama modu ayrı.')),
        h('div', { class: 'actions' },
          h('a', { class: 'btn', href: '#toplu', onclick: (e) => { e.preventDefault(); document.getElementById('toplu').scrollIntoView({ behavior: 'smooth' }); ta.focus(); } }, icon('upload'), 'Toplu içe aktar'),
          h('a', { class: 'btn primary', href: '#/hat/new' }, icon('plus'), 'Hat ekle'))),
      h('div', { class: 'card', style: { padding: '14px 18px' } },
        h('div', { class: 'row wrap', style: { gap: '12px 18px', alignItems: 'flex-end' } },
          h('div', { class: 'search grow', style: { flex: '1 1 280px' } }, icon('search'), searchInp), chipsBox)),
      h('section', { class: 'card', 'aria-labelledby': 'tum-hatlar' },
        h('div', { class: 'card-head' }, h('h2', { id: 'tum-hatlar' }, 'Tüm hatlar'), h('span', { class: 'small muted' }, 'Kayıt durumu kendiliğinden yenilenir')),
        tableBox),
      h('section', { class: 'card', id: 'toplu', 'aria-labelledby': 'toplu-h' },
        h('div', { class: 'card-head' }, h('div', { class: 'stack' }, h('h2', { id: 'toplu-h' }, 'Toplu içe aktar'),
          h('p', null, 'Sağlayıcıdan aldığın bilgileri yapıştır; her satır bir hat. Sıra: ', h('span', { class: 'mono' }, 'ad;numara;sunucu;kullanıcı;şifre'),
            ' (isteğe bağlı 6. sütun: kimlik kullanıcısı). Virgül ya da sekme de olur.'))),
        h('div', { class: 'card-body' },
          h('div', { class: 'cols' },
            h('label', { class: 'field col-3' }, 'Hat bilgileri', ta),
            h('div', { class: 'col-2', style: { gap: '14px' } },
              previewWrap,
              defaultsBox,
              h('div', { class: 'actions' }, h('label', { class: 'btn' }, 'CSV dosyası seç', fileInp), importBtn))))));
    renderTable();
    main.replaceChildren(root);
    App.poll(async () => {
      if (selected.size || document.querySelector('.modal')) return; // do not disturb a selection in progress
      d = Object.assign(d, await api('GET', '/api/lines'));
      renderTable();
    }, 10000);
  };

  // ------------------------------------------------------------------ Hat düzenle
  App.pages.hat = async (main, id) => {
    const isNew = id === 'new';
    const d = await api('GET', '/api/lines/' + (isNew ? 'new' : id));
    const f = isNew ? Object.assign({}, d.defaults, { name: '', number: '', server: '', username: '', authUser: '', password: '' }) : Object.assign({}, d.line, { password: '' });
    if (!f.exts) f.exts = [];
    if (isNew && d.exts.length && !f.exts.length) f.exts = d.exts.map((e) => e.num);
    const root = h('div', { class: 'page' });
    const errBox = h('div', { role: 'alert' });

    const bind = (key, el, conv) => {
      el.addEventListener('input', () => { f[key] = conv ? conv(el.value) : el.value; });
      return el;
    };
    const field = (label, key, attrs = {}, conv) => h('label', { class: 'field' }, label,
      bind(key, h('input', Object.assign({ class: 'input', value: f[key] === undefined || f[key] === null ? '' : String(f[key]) }, attrs)), conv));
    const num = (v) => parseInt(v, 10) || 0;

    // ---- SIP account card
    const pwInput = h('input', { class: 'input', type: 'password', autocomplete: 'new-password', placeholder: isNew ? '' : 'Değiştirmek için yazın', id: 'sifre' });
    bind('password', pwInput);
    const sipCard = h('section', { class: 'card', 'aria-labelledby': 'sip-h' },
      h('div', { class: 'card-head' }, h('div', { class: 'stack' }, h('h2', { id: 'sip-h' }, 'SIP hesabı'),
        h('p', null, 'Sağlayıcının verdiği bilgiler. Kaydedince hat hemen yeniden kayıt olur.'))),
      h('div', { class: 'card-body' },
        h('div', { class: 'grid-form' },
          field('Hat adı', 'name', { required: true, maxlength: '40' }),
          field('Numara', 'number', { class: 'input mono', placeholder: '+49 221 555 0101' }),
          field('Sunucu', 'server', { class: 'input mono', placeholder: 'sip.saglayici.de' }),
          field('Port', 'port', { class: 'input mono', type: 'number', min: '1', max: '65535' }, num),
          field('Kullanıcı adı', 'username', { class: 'input mono', autocomplete: 'off' }),
          h('label', { class: 'field' }, h('span', null, 'Kimlik kullanıcısı ', h('span', { class: 'opt' }, '(isteğe bağlı)')),
            bind('authUser', h('input', { class: 'input mono', value: f.authUser || '', placeholder: 'Boşsa kullanıcı adı' }))),
          h('div', { class: 'field' }, h('label', { for: 'sifre' }, 'Şifre'),
            h('div', { class: 'input-group' }, pwInput,
              h('button', { type: 'button', class: 'icon-btn', 'aria-label': 'Şifreyi göster', onclick: () => { pwInput.type = pwInput.type === 'password' ? 'text' : 'password'; } }, icon('eye')))),
          h('fieldset', null, h('legend', null, 'Taşıma'),
            App.seg([['udp', 'UDP'], ['tcp', 'TCP']], f.transport, (v) => { f.transport = v; }, { cls: 'fill', label: 'Taşıma' })),
          field('Kayıt yenileme (sn)', 'expiry', { class: 'input mono', type: 'number', min: '60', max: '3600' }, num))),
      h('details', { class: 'adv' },
        h('summary', null, icon('chevron'), 'Gelişmiş', h('span', { class: 'muted', style: { fontWeight: '400' } }, '· outbound proxy, codec, DTMF')),
        h('div', { class: 'grid-form' },
          field('Outbound proxy', 'outboundProxy', { class: 'input mono', placeholder: 'Boş bırakılabilir' }),
          field('From domain', 'fromDomain', { class: 'input mono', placeholder: 'Boşsa sunucu' }),
          h('label', { class: 'field' }, 'DTMF',
            h('select', { class: 'input', onchange: (e) => { f.dtmf = e.target.value; } },
              ...[['rfc4733', 'RFC 4733 (önerilen)'], ['info', 'SIP INFO'], ['inband', 'Ses içinde']].map(([v, t]) => h('option', { value: v, selected: f.dtmf === v ? 'selected' : null }, t)))),
          h('fieldset', { class: 'span-all' }, h('legend', null, 'Codec’ler (sırayla)'),
            h('div', { class: 'row wrap' }, ...[['g722', 'G.722 (HD ses)'], ['alaw', 'G.711 A-law'], ['ulaw', 'G.711 µ-law']].map(([v, t]) =>
              h('label', { class: 'check-chip' + (f.codecs.includes(v) ? ' on' : '') },
                h('input', { type: 'checkbox', checked: f.codecs.includes(v), onchange: (e) => {
                  f.codecs = e.target.checked ? [...f.codecs, v] : f.codecs.filter((x) => x !== v);
                  e.target.closest('label').classList.toggle('on', e.target.checked);
                } }), t)))))));

    // ---- recording card
    const folderPreview = () => {
      const today = new Date();
      const date = today.getFullYear() + '/' + String(today.getMonth() + 1).padStart(2, '0') + '/' + String(today.getDate()).padStart(2, '0');
      const ln = App.slug(f.name || 'Hat');
      const lay = d.settings.layout;
      return d.recordingsRoot + '/' + (lay === 'date-line' ? date + '/' + ln : lay === 'flat' ? '' : ln + '/' + date) + '/';
    };
    const folderDD = h('dd', { class: 'mono' }, folderPreview());
    root.addEventListener('input', () => { folderDD.textContent = folderPreview(); });
    const recCard = h('section', { class: 'card', 'aria-labelledby': 'kayit-h' },
      h('div', { class: 'card-body', style: { paddingTop: '16px' } },
        h('div', { class: 'row', style: { justifyContent: 'space-between' } },
          h('div', { class: 'stack' }, h('h2', { id: 'kayit-h', style: { fontSize: '15px' } }, 'Kayıt'),
            h('span', { class: 'small muted' }, 'Bu hattın tüm aramaları baştan sona kaydedilir.')),
          App.switch(f.record, 'Bu hattın aramalarını kaydet', (v) => { f.record = v; })),
        h('dl', { class: 'dl' }, h('dt', null, 'Klasör'), folderDD,
          h('dt', null, 'Biçim'), h('dd', null, (d.settings.format || 'mp3').toUpperCase() + ' · ' + (d.settings.retentionDays ? d.settings.retentionDays + ' gün saklanır' : 'süresiz saklanır') + ' · ', h('a', { href: '#/ayarlar' }, 'Ayarlar’da değiştir'))),
        h('div', { class: 'callout warn' }, icon('alert'),
          h('span', null, 'Görüşme kaydı için arayanı bilgilendirmek yasal olarak gerekebilir. ', h('b', null, 'Anons + Çaldır'), ' modunda bir “kayıt bildirimi” anonsu kullanın.'))));

    // ---- flow card
    const flowBox = h('div', { class: 'stack', style: { gap: '16px' } });
    const annOptions = (sel, empty) => [h('option', { value: '' }, empty), ...d.anns.map((a) => h('option', { value: a.id, selected: sel === a.id ? 'selected' : null }, a.name + ' · ' + fmt.ms(a.durMs)))];
    let player = null;
    const playAnn = (annId) => {
      if (!annId) return;
      if (player) player.pause();
      player = new Audio('/api/announcements/' + annId + '/audio');
      player.play().catch(App.err);
    };
    App.onLeave(() => { if (player) player.pause(); });

    function renderFlow() {
      const modes = [['ring', 'Çaldır', 'Arama hemen seçtiğin dahililerde çalar.'],
        ['annring', 'Anons + Çaldır', 'Önce anons çalınır, sonra dahililer çalar.'],
        ['ann', 'Sadece anons', 'Anons çalınır; sonra kapanır ya da sesli mesaj alınır.'],
        ['busy', 'Çalıp meşgul', 'Hiçbir dahili çalmaz; arayan çalma sesi duyar, süre dolunca meşgul.']];
      const modeCards = h('fieldset', { class: 'mode-cards' }, h('legend', { class: 'sr-only' }, 'Gelen arama modu'),
        ...modes.map(([v, t, desc]) => h('label', { class: 'mode-card' + (f.mode === v ? ' on' : '') },
          h('b', null, h('input', { type: 'radio', name: 'mod', checked: f.mode === v, onchange: () => { f.mode = v; renderFlow(); } }), t),
          h('span', null, desc))));
      const parts = [modeCards];
      const rings = f.mode === 'ring' || f.mode === 'annring';
      const ringTime = () => h('label', { class: 'field' }, 'Çalma süresi',
        h('span', { class: 'row' }, bind('ringSec', h('input', { class: 'input mono', type: 'number', min: '5', max: '300', value: String(f.ringSec), style: { width: '96px' } }), num), h('span', { style: { fontWeight: '400' } }, 'saniye')));
      if (f.mode === 'annring' || f.mode === 'ann') {
        parts.push(h('div', { class: 'field' }, h('label', { for: 'anons-sec' }, 'Çalınacak anons'),
          h('div', { class: 'input-group' },
            h('select', { id: 'anons-sec', class: 'input', onchange: (e) => { f.annId = e.target.value; renderFlow(); } }, ...annOptions(f.annId, d.anns.length ? 'Anons seçin' : 'Henüz anons yok')),
            h('button', { type: 'button', class: 'icon-btn round', 'aria-label': 'Anonsu dinle', disabled: !f.annId, onclick: () => playAnn(f.annId) }, icon('play')),
            h('a', { class: 'btn', href: '#/anonslar' }, 'Yeni anons'))));
      }
      if (rings) {
        parts.push(h('fieldset', null, h('legend', null, 'Çalacak dahililer ', h('span', { class: 'muted', style: { fontWeight: '400' } }, '· hepsi aynı anda çalar')),
          d.exts.length ? h('div', { class: 'row wrap' }, ...d.exts.map((e) => {
            const on = f.exts.includes(e.num);
            return h('label', { class: 'check-chip' + (on ? ' on' : '') },
              h('input', { type: 'checkbox', checked: on, onchange: (ev) => { f.exts = ev.target.checked ? [...f.exts, e.num] : f.exts.filter((x) => x !== e.num); renderFlow(); } }),
              h('b', { class: 'mono' }, e.num), h('span', null, e.name));
          })) : h('p', { class: 'small muted' }, 'Henüz dahili yok. ', h('a', { href: '#/dahililer' }, 'Dahili ekleyin'), ' ya da dahilisiz çalması için “Çalıp meşgul” modunu seçin.')));
        parts.push(h('div', { class: 'row wrap', style: { gap: '14px 18px', alignItems: 'flex-end' } }, ringTime(),
          h('fieldset', { style: { flex: '1 1 280px' } }, h('legend', null, 'Kimse açmazsa'),
            App.seg([['hang', 'Kapat'], ['busy', 'Meşgul ver'], ['ann', 'Anons çal'], ['vm', 'Sesli mesaj al']], f.noAnswer, (v) => { f.noAnswer = v; renderFlow(); }, { label: 'Kimse açmazsa' }))));
        if (f.noAnswer === 'ann' || f.noAnswer === 'vm') {
          parts.push(h('label', { class: 'field' }, f.noAnswer === 'ann' ? 'Kimse açmazsa çalınacak anons' : 'Sesli mesaj öncesi anons (isteğe bağlı)',
            h('select', { class: 'input', onchange: (e) => { f.noAnsAnnId = e.target.value; renderFlow(); } }, ...annOptions(f.noAnsAnnId, f.noAnswer === 'ann' ? 'Anons seçin' : 'Anons yok, yalnız bip'))));
        }
        parts.push(h('div', { class: 'switch-row' },
          h('div', null, h('b', null, 'Arayanın önüne hat adını ekle'),
            h('span', { class: 'hint' }, 'Telefonda görünür: ', h('span', { class: 'mono' }, '[' + (f.name || 'Hat') + '] +49 170 …'))),
          App.switch(f.cidPrefix, 'Arayanın önüne hat adını ekle', (v) => { f.cidPrefix = v; })));
      } else if (f.mode === 'ann') {
        parts.push(h('fieldset', null, h('legend', null, 'Anonstan sonra'),
          App.seg([['hang', 'Kapat'], ['vm', 'Sesli mesaj al']], f.after, (v) => { f.after = v; renderFlow(); }, { label: 'Anonstan sonra' })));
      } else {
        parts.push(ringTime());
        parts.push(h('div', { class: 'callout plain' }, icon('info'),
          h('span', null, 'Dahili seçmeniz gerekmez. Arayan bu süre boyunca çalma sesi duyar, sonra hat meşgul döner. Kimse açmadığı için ses kaydı alınmaz; arama geçmişte “Meşgul verildi” olarak görünür.')));
      }
      const usesVM = (f.mode === 'ann' && f.after === 'vm') || (rings && f.noAnswer === 'vm');
      if (usesVM) {
        parts.push(h('label', { class: 'field' }, 'Sesli mesaj en fazla',
          h('span', { class: 'row' }, bind('vmMaxSec', h('input', { class: 'input mono', type: 'number', min: '10', max: '600', value: String(f.vmMaxSec), style: { width: '96px' } }), num), h('span', { style: { fontWeight: '400' } }, 'saniye'))));
      }
      // summary
      const annName = (id2) => (d.anns.find((a) => a.id === id2) || {}).name || '?';
      const steps = ['Arama gelir'];
      if (f.mode !== 'busy') steps.push(f.record ? 'Kayıt başlar' : 'Kayıt kapalı');
      if (f.mode === 'annring' || f.mode === 'ann') steps.push('Anons: ' + annName(f.annId));
      if (rings) {
        steps.push((f.exts.join(', ') || 'dahili seçilmedi') + ' çalar · ' + f.ringSec + ' sn');
        steps.push({ hang: 'Açılmazsa: kapat', busy: 'Açılmazsa: meşgul', ann: 'Açılmazsa: anons', vm: 'Açılmazsa: sesli mesaj' }[f.noAnswer]);
      } else if (f.mode === 'ann') {
        steps.push(f.after === 'vm' ? 'Sonra: sesli mesaj' : 'Sonra: kapat');
      } else {
        steps.push('Çalma sesi · ' + f.ringSec + ' sn', 'Meşgul');
      }
      const stepList = h('ol', { class: 'flow-steps' });
      steps.forEach((s, i) => stepList.append(h('li', null, h('span', null, s), i < steps.length - 1 ? icon('chevron') : null)));
      parts.push(h('div', { class: 'flow-sum' }, h('span', { class: 'lbl' }, 'Bu hatta arama gelince'), stepList,
        f.record && f.mode !== 'busy' ? h('span', { class: 'note' }, h('span', { class: 'dot fail' }), 'Kayıt, arama kapanana kadar sürer') : null));
      flowBox.replaceChildren(...parts);
    }
    renderFlow();
    const flowCard = h('section', { class: 'card col-12', 'aria-labelledby': 'akis-h' },
      h('div', { class: 'card-head' }, h('div', { class: 'stack' }, h('h2', { id: 'akis-h' }, 'Gelen arama akışı'), h('p', null, 'Bu numaraya arama gelince ne olsun?'))),
      h('div', { class: 'card-body' }, flowBox));

    // ---- header
    const save = async () => {
      errBox.replaceChildren();
      const body = Object.assign({}, f);
      try {
        const saved = await api(isNew ? 'POST' : 'PUT', '/api/lines' + (isNew ? '' : '/' + id), body);
        App.toast('“' + saved.name + '” kaydedildi', 'ok');
        location.hash = '#/hatlar';
      } catch (e) {
        errBox.replaceChildren(h('div', { class: 'callout err' }, icon('alert'), h('span', null, e.message)));
        errBox.scrollIntoView({ behavior: 'smooth', block: 'center' });
      }
    };
    const s = isNew ? null : App.lineState(d.line.status);
    const headActions = [];
    if (!isNew) {
      headActions.push(h('div', { class: 'row', style: { gap: '0' } }, App.switch(d.line.enabled, 'Hat açık', async (v) => {
        await api('POST', '/api/lines/' + id + '/enabled', { enabled: v });
        f.enabled = v;
        App.toast(v ? 'Hat açıldı' : 'Hat kapatıldı', 'ok');
      }), h('span', { class: 'small', style: { fontWeight: '600' } }, 'Hat açık')));
      const testBtn = h('button', { type: 'button', class: 'btn' }, icon('refresh'), 'Bağlantıyı test et');
      testBtn.addEventListener('click', async () => {
        testBtn.disabled = true;
        testBtn.lastChild.textContent = 'Deneniyor…';
        try {
          const r = await api('POST', '/api/lines/' + id + '/test');
          if (r.state === 'registered') App.toast('Kayıt başarılı', 'ok');
          else App.toast('Kayıt olmadı: ' + (r.detail || r.state), 'err');
        } catch (e) { App.err(e); }
        testBtn.disabled = false;
        testBtn.lastChild.textContent = 'Bağlantıyı test et';
      });
      headActions.push(testBtn);
      headActions.push(h('button', { type: 'button', class: 'btn danger', onclick: async () => {
        if (!await App.confirm('“' + d.line.name + '” hattı silinsin mi? Geçmiş aramalar ve kayıtlar silinmez.', { ok: 'Sil', danger: true })) return;
        try { await api('DELETE', '/api/lines/' + id); App.toast('Hat silindi', 'ok'); location.hash = '#/hatlar'; } catch (e) { App.err(e); }
      } }, icon('trash'), 'Sil'));
    }
    headActions.push(h('button', { type: 'button', class: 'btn primary', onclick: save }, 'Kaydet'));

    root.append(
      h('div', null,
        h('nav', { class: 'crumb', 'aria-label': 'Konum' }, h('a', { href: '#/hatlar' }, 'Hatlar'), h('span', { 'aria-hidden': 'true' }, '/'), h('span', null, isNew ? 'Yeni hat' : d.line.name)),
        h('div', { class: 'page-head' },
          h('div', { class: 'row wrap', style: { gap: '8px 14px' } },
            h('h1', null, isNew ? 'Yeni hat' : d.line.name),
            !isNew && d.line.number ? h('span', { class: 'mono muted' }, d.line.number) : null,
            s ? h('span', { class: 'pill ' + s.cls, title: d.line.status.detail || '' }, h('span', { class: 'dot ' + s.cls }), s.text + (d.line.status.detail ? ' · ' + d.line.status.detail : '')) : null),
          h('div', { class: 'actions' }, ...headActions))),
      errBox,
      h('div', { class: 'cols' }, h('div', { class: 'col-1' }, sipCard, recCard), flowCard));
    main.replaceChildren(root);
  };
})();

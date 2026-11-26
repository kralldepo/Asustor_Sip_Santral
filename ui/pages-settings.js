'use strict';
/* Dahililer, Ayarlar */
(() => {
  const { h, icon, api, fmt } = App;

  // ---------------------------------------------------------------- Dahililer
  App.pages.dahililer = async (main) => {
    let d = await api('GET', '/api/extensions');
    let sel = d.exts.length ? d.exts[0].num : null;
    const revealed = {};
    const root = h('div', { class: 'page' });
    const body = h('div', { class: 'cols' });

    const editModal = (ext) => {
      const num = h('input', { class: 'input mono', value: ext ? ext.num : nextNum(), inputmode: 'numeric', maxlength: '6' });
      const name = h('input', { class: 'input', value: ext ? ext.name : '', placeholder: 'ör. Ofis MicroSIP', maxlength: '40' });
      const pw = h('input', { class: 'input mono', placeholder: ext ? 'Değiştirmek için yazın' : 'Boş bırakın, güçlü şifre üretilsin', autocomplete: 'new-password' });
      App.modal({
        title: ext ? 'Dahili düzenle' : 'Dahili ekle',
        body: h('div', { class: 'stack', style: { gap: '14px' } },
          h('div', { class: 'grid-form' }, h('label', { class: 'field' }, 'Numara', num), h('label', { class: 'field' }, 'Ad', name)),
          h('label', { class: 'field' }, h('span', null, 'Şifre ', h('span', { class: 'opt' }, '(isteğe bağlı)')), pw),
          h('p', { class: 'small muted' }, 'Her cihaz (MicroSIP, cep, laptop, masa telefonu) için ayrı bir dahili açın.')),
        actions: [{ label: 'Vazgeç' }, { label: ext ? 'Kaydet' : 'Ekle', primary: true, onClick: async () => {
          const saved = await api(ext ? 'PUT' : 'POST', '/api/extensions' + (ext ? '/' + ext.num : ''), { num: num.value, name: name.value, password: pw.value });
          sel = saved.num;
          revealed[saved.num] = saved.password;
          d = await api('GET', '/api/extensions');
          render();
          App.toast(ext ? 'Dahili kaydedildi' : 'Dahili ' + saved.num + ' eklendi — bağlantı bilgileri sağda', 'ok');
        } }],
      });
    };
    const nextNum = () => {
      const nums = d.exts.map((e) => parseInt(e.num, 10)).filter((n) => !isNaN(n));
      return String(nums.length ? Math.max(...nums) + 1 : 101);
    };

    function render() {
      const left = h('div', { class: 'col-3' });
      for (const e of d.exts) {
        const st = e.status || {};
        const ua = st.ua ? st.ua.split(' ').slice(0, 2).join(' ') : '';
        const where = st.addr ? st.addr.replace(/^sip:[^@]*@/, '').split(';')[0] : '';
        const info = st.online ? [ua, where, 'bağlı'].filter(Boolean).join(' · ') : (e.lastSeenText ? 'son görülme ' + e.lastSeenText : 'henüz bağlanmadı');
        left.append(h('article', { class: 'card ext-card' + (sel === e.num ? ' sel' : ''), 'aria-labelledby': 'd' + e.num },
          h('div', { class: 'row wrap', style: { gap: '12px' } },
            h('span', { class: 'ext-num mono' + (st.online ? '' : ' off'), 'aria-hidden': 'true' }, e.num),
            h('div', { class: 'grow stack', style: { flex: '1 1 200px', gap: '0' } },
              h('h2', { id: 'd' + e.num, style: { fontSize: '15px' } }, e.num + ' · ' + e.name),
              h('span', { class: 'small muted' }, info)),
            st.online ? h('span', { class: 'pill ok' }, h('span', { class: 'dot ok' }), 'Bağlı') : h('span', { class: 'pill off' }, h('span', { class: 'dot off' }), 'Çevrimdışı')),
          h('div', { class: 'row wrap small', style: { gap: '6px' } }, h('span', { class: 'muted' }, 'Çaldıran hatlar:'),
            e.lines && e.lines.length ? e.lines.map((n) => h('span', { class: 'chip' }, n)) : h('span', { class: 'muted' }, 'yok')),
          h('div', { class: 'row wrap', style: { justifyContent: 'space-between' } },
            h('span', { class: 'small', style: { color: 'var(--text-2)' } }, 'Bugün ', h('b', null, String(e.todayAnswered)), ' arama açtı'),
            h('div', { class: 'actions' },
              h('button', { type: 'button', class: 'btn small', 'aria-pressed': String(sel === e.num), onclick: () => { sel = e.num; render(); } }, 'Bağlantı bilgileri'),
              h('button', { type: 'button', class: 'btn small', onclick: () => editModal(e) }, 'Düzenle'),
              h('button', { type: 'button', class: 'icon-btn danger', 'aria-label': 'Dahili ' + e.num + ' sil', onclick: async () => {
                if (!await App.confirm('Dahili ' + e.num + ' (' + e.name + ') silinsin mi? Bu cihaz artık çalmaz.', { ok: 'Sil', danger: true })) return;
                try { await api('DELETE', '/api/extensions/' + e.num); d = await api('GET', '/api/extensions'); sel = d.exts.length ? d.exts[0].num : null; render(); } catch (err) { App.err(err); }
              } }, icon('trash'))))));
      }
      if (!d.exts.length) left.append(h('div', { class: 'card empty', style: { borderTop: '1px solid var(--border)' } }, 'Henüz dahili yok. “Dahili ekle” ile ilk cihazınızı (ör. MicroSIP) ekleyin.'));

      const e = d.exts.find((x) => x.num === sel);
      const right = h('section', { class: 'card col-2', 'aria-labelledby': 'baglanti-h' });
      if (e) {
        const pwVal = h('dd', { class: 'mono', style: { letterSpacing: revealed[e.num] ? '0' : '.1em' } }, revealed[e.num] || '••••••••••••••');
        const copyBtn = (label, getter) => h('dd', null, h('button', { type: 'button', class: 'icon-btn', 'aria-label': label, onclick: async () => App.copy(await getter()) }, icon('copy')));
        const getPw = async () => {
          if (!revealed[e.num]) revealed[e.num] = (await api('GET', '/api/extensions/' + e.num + '/secret')).password;
          return revealed[e.num];
        };
        const server = d.server.host || '[NAS IP adresi]';
        right.append(
          h('div', { class: 'card-head' }, h('div', { class: 'stack' }, h('h2', { id: 'baglanti-h' }, e.num + ' · bağlantı bilgileri'),
            h('p', null, 'MicroSIP’te “Hesap ekle” ekranına bunları girin.'))),
          h('div', { class: 'card-body' },
            h('dl', { class: 'conn' },
              h('dt', null, 'SIP sunucusu'), h('dd', { class: 'mono' }, server), copyBtn('Sunucu adresini kopyala', () => server),
              h('dt', null, 'Port · taşıma'), h('dd', { class: 'mono' }, d.server.port + ' · ' + d.server.transports.join(' / ')), h('dd'),
              h('dt', null, 'Kullanıcı adı'), h('dd', { class: 'mono' }, e.num), copyBtn('Kullanıcı adını kopyala', () => e.num),
              h('dt', null, 'Şifre'), pwVal,
              h('dd', { class: 'row', style: { gap: '6px' } },
                h('button', { type: 'button', class: 'icon-btn', 'aria-label': 'Şifreyi göster', onclick: async () => { try { pwVal.textContent = await getPw(); pwVal.style.letterSpacing = '0'; } catch (err) { App.err(err); } } }, icon('eye')),
                h('button', { type: 'button', class: 'icon-btn', 'aria-label': 'Şifreyi kopyala', onclick: async () => { try { App.copy(await getPw()); } catch (err) { App.err(err); } } }, icon('copy'))),
              h('dt', null, 'Görünen ad'), h('dd', null, e.name), h('dd')),
            h('p', { class: 'small', style: { color: 'var(--text-2)' } }, 'Zoiper, Linphone ve masa telefonlarında da aynı bilgiler kullanılır: alan adı / domain = SIP sunucusu.'),
            h('div', { class: 'actions' },
              h('button', { type: 'button', class: 'btn small', onclick: async () => {
                if (!await App.confirm('Dahili ' + e.num + ' için yeni şifre üretilsin mi? Cihazda da şifreyi değiştirmeniz gerekir.', { ok: 'Yenile' })) return;
                try { revealed[e.num] = (await api('POST', '/api/extensions/' + e.num + '/password')).password; render(); App.toast('Yeni şifre üretildi', 'ok'); } catch (err) { App.err(err); }
              } }, icon('refresh'), 'Şifreyi yenile')),
            h('div', { class: 'callout info' }, icon('lock'),
              h('span', null, 'Ev dışından bağlanacaksanız ASUSTOR VPN Server’ı kullanın. 5060 portunu internete açmayın: SIP portları dakikalar içinde taranır.' +
                (d.server.lanOnly ? ' Dahililer şu an yalnız yerel ağdan ve VPN’den bağlanabilir.' : '')))));
      } else {
        right.append(h('div', { class: 'card-head' }, h('h2', { id: 'baglanti-h' }, 'Bağlantı bilgileri')), h('div', { class: 'card-body muted' }, 'Bir dahili seçin.'));
      }
      body.replaceChildren(left, right);
    }

    render();
    root.append(
      h('div', { class: 'page-head' },
        h('div', null, h('h1', null, 'Dahililer'), h('p', null, 'Gelen aramayı açan telefonlar. MicroSIP, Zoiper, Linphone ya da bir IP masa telefonu bu NAS’a bağlanır.')),
        h('button', { type: 'button', class: 'btn primary', onclick: () => editModal(null) }, icon('plus'), 'Dahili ekle')),
      body);
    main.replaceChildren(root);
    App.poll(async () => { d = await api('GET', '/api/extensions'); render(); }, 10000);
  };

  // ---------------------------------------------------------------- Ayarlar
  App.pages.ayarlar = async (main) => {
    let d = await api('GET', '/api/settings');
    const s = Object.assign({}, d.settings);
    if (!s.localNets) s.localNets = [];
    const root = h('div', { class: 'page' });

    const sel = (key, options, conv) => h('select', { class: 'input', onchange: (e) => { s[key] = conv ? conv(e.target.value) : e.target.value; } },
      ...options.map(([v, t]) => h('option', { value: String(v), selected: String(s[key]) === String(v) ? 'selected' : null }, t)));
    const inp = (key, attrs = {}, conv) => {
      const el = h('input', Object.assign({ class: 'input', value: s[key] === undefined ? '' : String(s[key]) }, attrs));
      el.addEventListener('input', () => { s[key] = conv ? conv(el.value) : el.value; });
      return el;
    };
    const num = (v) => parseInt(v, 10) || 0;

    // ---- storage
    const shares = Array.from(new Set([...(d.shares || []), d.recordingsRoot]));
    const shareSel = h('select', { class: 'input', onchange: (e) => { s.shareFolder = e.target.value; customShare.hidden = e.target.value !== '__custom'; } },
      ...shares.map((p) => h('option', { value: p, selected: p === d.recordingsRoot ? 'selected' : null }, p + (p === d.defaultShare ? ' (varsayılan)' : ''))),
      h('option', { value: '__custom' }, 'Başka klasör yaz…'));
    const customShare = h('input', { class: 'input mono', placeholder: '/volume1/KlasorAdi', hidden: true });
    customShare.addEventListener('input', () => { s.shareFolder = customShare.value; });
    s.shareFolder = d.recordingsRoot;
    const st = d.storage;
    const pct = st.used + st.free > 0 ? Math.max(0.5, st.used / (st.used + st.free) * 100) : 0;
    const storageCard = h('section', { class: 'card', 'aria-labelledby': 'depo-h' },
      h('div', { class: 'card-head' }, h('div', { class: 'stack' }, h('h2', { id: 'depo-h' }, 'Kayıt depolama'),
        h('p', null, 'Kayıtlar ADM’de paylaşım klasöründe durur; SMB ve Dosya Gezgini’nden de açılır.'))),
      h('div', { class: 'card-body' },
        h('div', { class: 'grid-form' },
          h('label', { class: 'field span-all' }, 'Kayıt klasörü', shareSel, customShare),
          h('label', { class: 'field' }, 'Klasör düzeni', sel('folderLayout', [['line-date', 'Hat / Yıl / Ay / Gün'], ['date-line', 'Yıl / Ay / Gün / Hat'], ['flat', 'Tek klasör']])),
          h('label', { class: 'field' }, 'Saklama süresi', sel('retentionDays', [[30, '30 gün'], [90, '90 gün'], [180, '180 gün'], [365, '365 gün'], [730, '2 yıl'], [0, 'Süresiz (silme)']], num)),
          h('fieldset', { class: 'span-all' }, h('legend', null, 'Biçim'),
            App.seg([['mp3', 'MP3 · ~0,24 MB/dk'], ['wav', 'WAV · ~0,96 MB/dk']], s.format, (v) => { s.format = v; }, { cls: 'fill', label: 'Kayıt biçimi' }))),
        h('div', { class: 'stack' },
          h('div', { class: 'bar', 'aria-hidden': 'true' }, h('div', { style: { width: pct.toFixed(1) + '%' } })),
          h('span', { class: 'small', style: { color: 'var(--text-2)' } }, h('b', null, fmt.bytes(st.used)), ' kullanılıyor · birimde ' + fmt.bytes(st.free) + ' boş'))));

    // ---- network
    const extIPInp = inp('extIp', { class: 'input mono', placeholder: '203.0.113.10' });
    const extIPWrap = h('label', { class: 'field', hidden: s.extIpMode !== 'manual' }, 'Dış IP adresi (elle)', extIPInp);
    const netsTa = h('textarea', { class: 'input mono', rows: '3', placeholder: '192.168.1.0/24' }, (s.localNets || []).join('\n'));
    netsTa.addEventListener('input', () => { s.localNets = netsTa.value.split(/[\s,]+/).filter(Boolean); });
    const netsWrap = h('label', { class: 'field span-all', hidden: s.localNetMode !== 'manual' }, 'Yerel ağlar (her satıra bir ağ)', netsTa);
    const netCard = h('section', { class: 'card', 'aria-labelledby': 'ag-h' },
      h('div', { class: 'card-head' }, h('div', { class: 'stack' }, h('h2', { id: 'ag-h' }, 'Ağ ve NAT'), h('p', null, 'Sesin iki yönlü gelmesi bu ayarlara bağlı.'))),
      h('div', { class: 'card-body' },
        h('div', { class: 'grid-form' },
          h('label', { class: 'field' }, 'SIP portu', inp('sipPort', { class: 'input mono', type: 'number', min: '1024', max: '65535' }, num)),
          h('fieldset', null, h('legend', null, 'Taşıma'), h('div', { class: 'row wrap', style: { gap: '4px 14px' } },
            ...[['udp', 'UDP'], ['tcp', 'TCP']].map(([k, t]) => h('label', { class: 'row', style: { minHeight: '44px' } },
              h('input', { type: 'checkbox', checked: s[k], style: { width: '18px', height: '18px', accentColor: 'var(--primary)' }, onchange: (e) => { s[k] = e.target.checked; } }), t)))),
          h('div', { class: 'field' }, h('span', null, 'Ses (RTP) portları'),
            h('span', { class: 'row' }, inp('rtpStart', { class: 'input mono', type: 'number', 'aria-label': 'İlk port' }, num), h('span', { class: 'muted' }, '–'), inp('rtpEnd', { class: 'input mono', type: 'number', 'aria-label': 'Son port' }, num))),
          h('label', { class: 'field' }, 'Dış IP adresi', h('select', { class: 'input', onchange: (e) => { s.extIpMode = e.target.value; extIPWrap.hidden = e.target.value !== 'manual'; } },
            ...[['auto', 'Otomatik bul (STUN, 10 dk’da bir)'], ['manual', 'Elle gir'], ['off', 'Kullanma']].map(([v, t]) => h('option', { value: v, selected: s.extIpMode === v ? 'selected' : null }, t)))),
          extIPWrap,
          h('label', { class: 'field' }, 'Yerel ağ', h('select', { class: 'input', onchange: (e) => { s.localNetMode = e.target.value; netsWrap.hidden = e.target.value !== 'manual'; } },
            ...[['auto', 'Otomatik algıla'], ['manual', 'Elle gir']].map(([v, t]) => h('option', { value: v, selected: s.localNetMode === v ? 'selected' : null }, t)))),
          netsWrap,
          h('label', { class: 'field' }, 'Çalma sesi (ülke)', sel('toneZone', (d.toneZones || []).map((z) => [z, { de: 'Almanya', tr: 'Türkiye', at: 'Avusturya', ch: 'İsviçre', nl: 'Hollanda', fr: 'Fransa', uk: 'Birleşik Krallık', us: 'ABD' }[z] || z]))),
          h('div', { class: 'stack small span-all', style: { color: 'var(--text-2)' } },
            h('span', null, 'Şu anki dış IP: ', h('b', { class: 'mono' }, d.pbx.extIp || '—'), ' · Algılanan yerel ağlar: ', h('span', { class: 'mono' }, (d.detectedNets || []).join(', ') || '—')),
            h('span', null, 'NAS’ın adresleri: ', h('span', { class: 'mono' }, (d.lanIPs || []).join(', ') || '—')))),
        h('div', { class: 'callout plain' }, icon('info'),
          h('span', null, 'Hatlar dışarıya kayıt olduğu için çoğu sağlayıcıda router’da port açmak gerekmez. Ses tek yönlü gelirse router’da UDP ' + s.rtpStart + '–' + s.rtpEnd + ' aralığını NAS’a yönlendirin. Port değişiklikleri Asterisk’i yeniden başlatır.'))));

    // ---- security
    const blockedBox = h('div', { class: 'stack' });
    const renderBlocked = () => {
      blockedBox.replaceChildren(...(d.blocked.length ? d.blocked.map((b) => h('div', { class: 'row', style: { justifyContent: 'space-between' } },
        h('span', { class: 'mono small' }, b.ip + ' · ' + fmt.time(b.until) + '’e kadar'),
        h('button', { type: 'button', class: 'btn small', onclick: async () => {
          try { await api('POST', '/api/system/unblock', { ip: b.ip }); d.blocked = d.blocked.filter((x) => x.ip !== b.ip); renderBlocked(); } catch (e) { App.err(e); }
        } }, 'Engeli kaldır'))) : [h('span', { class: 'small muted' }, 'Engellenmiş IP yok.')]));
    };
    renderBlocked();
    const secCard = h('section', { class: 'card', 'aria-labelledby': 'guv-h' },
      h('div', { class: 'card-head' }, h('h2', { id: 'guv-h' }, 'Güvenlik')),
      h('div', { class: 'card-body', style: { gap: '0' } },
        h('div', { class: 'switch-row plain', style: { borderTop: '0' } },
          h('div', null, h('b', null, 'Panel parolası'), h('span', { class: 'hint' }, 'Panel ADM sertifikasıyla HTTPS üzerinden açılır')),
          h('button', { type: 'button', class: 'btn small', onclick: passwordModal }, 'Parolayı değiştir')),
        h('div', { class: 'switch-row plain' },
          h('div', null, h('b', null, 'Dahililer yalnız yerel ağdan ve VPN’den bağlansın'), h('span', { class: 'hint' }, 'İnternetten gelen dahili kayıt denemeleri reddedilir')),
          App.switch(s.extLanOnly, 'Dahililer yalnız yerel ağdan bağlansın', (v) => { s.extLanOnly = v; })),
        h('div', { class: 'switch-row plain' },
          h('div', null, h('b', null, 'Hatalı şifre denemesinde IP’yi engelle'), h('span', { class: 'hint' }, '10 dakikada 5 hatalı deneme → 1 saat engel')),
          App.switch(s.blockFailed, 'Hatalı şifre denemesinde IP’yi engelle', (v) => { s.blockFailed = v; })),
        h('div', { class: 'switch-row plain', style: { flexDirection: 'column', alignItems: 'stretch' } },
          h('b', null, 'Engellenen IP’ler'), blockedBox),
        h('p', { class: 'small muted', style: { paddingTop: '8px' } }, 'Tanınmayan sunuculardan gelen aramalar her zaman reddedilir.')));

    function passwordModal() {
      const cur = h('input', { class: 'input', type: 'password', autocomplete: 'current-password' });
      const p1 = h('input', { class: 'input', type: 'password', autocomplete: 'new-password' });
      const p2 = h('input', { class: 'input', type: 'password', autocomplete: 'new-password' });
      App.modal({
        title: 'Panel parolasını değiştir',
        body: h('div', { class: 'stack', style: { gap: '12px' } }, h('label', { class: 'field' }, 'Mevcut parola', cur), h('label', { class: 'field' }, 'Yeni parola', p1), h('label', { class: 'field' }, 'Yeni parola (tekrar)', p2)),
        actions: [{ label: 'Vazgeç' }, { label: 'Değiştir', primary: true, onClick: async () => {
          if (p1.value !== p2.value) throw new Error('Yeni parolalar aynı değil');
          await api('POST', '/api/password', { current: cur.value, password: p1.value });
          App.toast('Parola değişti; diğer oturumlar kapatıldı', 'ok');
        } }],
      });
    }

    // ---- system
    const restoreInp = h('input', { type: 'file', accept: '.json,application/json', class: 'sr-only' });
    restoreInp.addEventListener('change', async () => {
      const f = restoreInp.files[0];
      restoreInp.value = '';
      if (!f) return;
      if (!await App.confirm('Hatlar, dahililer, anonslar ve ayarlar yedektekilerle değiştirilsin mi? Arama geçmişi ve kayıtlar etkilenmez.', { ok: 'Geri yükle', danger: true })) return;
      try {
        const r = await fetch('/api/backup', { method: 'POST', headers: { 'X-Requested-With': 'sipsantral', 'Content-Type': 'application/json' }, body: await f.text() });
        const j = await r.json().catch(() => ({}));
        if (!r.ok) throw new Error(j.error || 'HTTP ' + r.status);
        App.toast('Yedek geri yüklendi', 'ok');
      } catch (e) { App.err(e); }
    });
    const pbx = d.pbx;
    const pbxPill = { running: ['ok', 'Çalışıyor'], error: ['fail', 'Hata'], preparing: ['warn', 'Hazırlanıyor'], starting: ['warn', 'Başlatılıyor'] }[pbx.state] || ['off', pbx.state];
    const sysCard = h('section', { class: 'card', 'aria-labelledby': 'sis-h' },
      h('div', { class: 'card-head' }, h('h2', { id: 'sis-h' }, 'Sistem')),
      h('div', { class: 'card-body' },
        h('div', { class: 'row wrap', style: { justifyContent: 'space-between' } },
          h('div', { class: 'stack' },
            h('span', { class: 'row' }, h('b', null, 'Asterisk'), h('span', { class: 'pill ' + pbxPill[0] }, h('span', { class: 'dot ' + pbxPill[0] }), pbxPill[1])),
            h('span', { class: 'small muted' }, [pbx.asteriskVersion, pbx.state === 'running' && pbx.since ? 'son başlatma ' + fmt.ago(pbx.since) : pbx.message].filter(Boolean).join(' · ') + ' · yeniden başlatma süren aramaları keser')),
          h('button', { type: 'button', class: 'btn small', onclick: async () => {
            if (!await App.confirm('Asterisk yeniden başlatılsın mı? Süren aramalar kesilir.', { ok: 'Yeniden başlat', danger: true })) return;
            try { await api('POST', '/api/system/restart'); App.toast('Asterisk yeniden başlatılıyor', 'ok'); } catch (e) { App.err(e); }
          } }, icon('refresh'), 'Yeniden başlat')),
        h('div', { class: 'row wrap', style: { justifyContent: 'space-between', paddingTop: '14px', borderTop: '1px solid var(--line)' } },
          h('div', { class: 'stack' }, h('b', null, 'Yedek'), h('span', { class: 'small muted' }, 'Hatlar (şifreleriyle), dahililer, anonslar ve ayarlar tek dosyada')),
          h('div', { class: 'actions' }, h('a', { class: 'btn small', href: '/api/backup' }, icon('download'), 'Yedeği indir'), h('label', { class: 'btn small' }, icon('upload'), 'Yedekten yükle', restoreInp))),
        h('label', { class: 'field', style: { paddingTop: '14px', borderTop: '1px solid var(--line)' } }, h('span', null, 'Saat dilimi ', h('span', { class: 'opt' }, '· şu an: ' + d.timezoneName)),
          inp('timezone', { class: 'input mono', placeholder: 'Boş = NAS saati (ör. Europe/Berlin)' })),
        h('div', { class: 'row wrap small muted' }, 'Paket ', h('span', { class: 'mono', style: { color: 'var(--text)' } }, 'sipsantral ' + d.version))));

    // ---- log
    const logPre = h('pre', { class: 'log', 'aria-live': 'off' });
    let raw = false;
    const rawBtn = h('button', { type: 'button', class: 'btn small', 'aria-pressed': 'false' }, 'Ham Asterisk günlüğü');
    const loadLog = async () => {
      const r = await api('GET', '/api/system/log' + (raw ? '?raw=1' : ''));
      if (raw) {
        logPre.textContent = r.raw || '(boş)';
        logPre.scrollTop = logPre.scrollHeight;
        return;
      }
      logPre.replaceChildren(...r.events.map((e) => h('span', { class: e.level === 'error' ? 'error' : e.level === 'warn' ? 'warn' : '' },
        h('span', { class: 't' }, fmt.dateShort(e.time) + ' ' + new Date(e.time).toLocaleTimeString('tr-TR')), '  ',
        h('span', { class: 'src' }, (e.source || '').padEnd(14).slice(0, 14)), '  ', e.text, '\n')));
      if (!r.events.length) logPre.textContent = 'Henüz kayıt yok.';
    };
    rawBtn.addEventListener('click', () => { raw = !raw; rawBtn.setAttribute('aria-pressed', String(raw)); rawBtn.textContent = raw ? 'Olay günlüğü' : 'Ham Asterisk günlüğü'; loadLog().catch(App.err); });
    const logCard = h('section', { class: 'card', 'aria-labelledby': 'log-h' },
      h('div', { class: 'card-head' }, h('h2', { id: 'log-h' }, 'Günlük'),
        h('div', { class: 'actions' },
          h('span', { class: 'row small', style: { fontWeight: '600', gap: '0' } }, 'Ayrıntılı SIP günlüğü',
            App.switch(d.settings.sipDebug, 'Ayrıntılı SIP günlüğü', async (v) => { await api('POST', '/api/system/sipdebug', { on: v }); App.toast(v ? 'SIP ayrıntıları ham günlüğe yazılıyor' : 'Ayrıntılı günlük kapandı', 'ok'); })),
          rawBtn, h('button', { type: 'button', class: 'icon-btn', 'aria-label': 'Yenile', onclick: () => loadLog().catch(App.err) }, icon('refresh')))),
      h('div', { class: 'card-body' }, logPre));

    const saveBtn = h('button', { type: 'button', class: 'btn primary' }, 'Kaydet');
    saveBtn.addEventListener('click', async () => {
      const body = Object.assign({}, s);
      if (body.shareFolder === '__custom') body.shareFolder = customShare.value;
      saveBtn.disabled = true;
      try {
        const r = await api('PUT', '/api/settings', body);
        App.toast(r.restart ? 'Kaydedildi — Asterisk yeniden başlatılıyor (süren arama varsa bitince)' : 'Ayarlar kaydedildi', 'ok');
        d = await api('GET', '/api/settings');
      } catch (e) { App.err(e); }
      saveBtn.disabled = false;
    });

    root.append(
      h('div', { class: 'page-head' }, h('div', null, h('h1', null, 'Ayarlar'), h('p', null, 'Kaydedince santrale hemen uygulanır; süren görüşmeler kesilmez.')), saveBtn),
      h('div', { class: 'cols' }, h('div', { class: 'col-1' }, storageCard, secCard), h('div', { class: 'col-1' }, netCard, sysCard)),
      logCard);
    main.replaceChildren(root);
    await loadLog();
    App.poll(loadLog, 5000);
  };
})();

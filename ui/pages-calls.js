'use strict';
/* Anonslar, Aramalar ve kayıtlar */
(() => {
  const { h, icon, api, fmt } = App;

  // ---------------------------------------------------------------- audio helpers
  function encodeWav16(samples, rate) {
    const buf = new ArrayBuffer(44 + samples.length * 2);
    const v = new DataView(buf);
    const w = (o, s) => { for (let i = 0; i < s.length; i++) v.setUint8(o + i, s.charCodeAt(i)); };
    w(0, 'RIFF'); v.setUint32(4, 36 + samples.length * 2, true); w(8, 'WAVE'); w(12, 'fmt ');
    v.setUint32(16, 16, true); v.setUint16(20, 1, true); v.setUint16(22, 1, true);
    v.setUint32(24, rate, true); v.setUint32(28, rate * 2, true); v.setUint16(32, 2, true); v.setUint16(34, 16, true);
    w(36, 'data'); v.setUint32(40, samples.length * 2, true);
    for (let i = 0; i < samples.length; i++) {
      const s = Math.max(-1, Math.min(1, samples[i]));
      v.setInt16(44 + i * 2, s < 0 ? s * 0x8000 : s * 0x7fff, true);
    }
    return new Blob([buf], { type: 'audio/wav' });
  }

  // Decodes any audio the browser understands and returns a mono WAV (16 kHz
  // when the browser can resample, else the original rate).
  async function toWav(arrayBuffer) {
    const AC = window.AudioContext || window.webkitAudioContext;
    const ctx = new AC();
    let audio;
    try {
      audio = await new Promise((res, rej) => {
        const p = ctx.decodeAudioData(arrayBuffer, res, rej);
        if (p && p.then) p.then(res, rej);
      });
    } finally { ctx.close && ctx.close(); }
    if (audio.duration > 300) throw new Error('Anons en fazla 5 dakika olabilir');
    const n = audio.length;
    const mono = new Float32Array(n);
    for (let c = 0; c < audio.numberOfChannels; c++) {
      const d = audio.getChannelData(c);
      for (let i = 0; i < n; i++) mono[i] += d[i] / audio.numberOfChannels;
    }
    let rate = audio.sampleRate;
    let data = mono;
    if (rate > 16000) {
      try {
        const target = 16000;
        const off = new OfflineAudioContext(1, Math.ceil(n * target / rate), target);
        const b = off.createBuffer(1, n, rate);
        b.copyToChannel(mono, 0);
        const src = off.createBufferSource();
        src.buffer = b;
        src.connect(off.destination);
        src.start();
        const out = await off.startRendering();
        data = out.getChannelData(0);
        rate = target;
      } catch (e) { /* keep the original rate; the NAS resamples */ }
    }
    return { blob: encodeWav16(data, rate), seconds: audio.duration };
  }

  // ---------------------------------------------------------------- Anonslar
  App.pages.anonslar = async (main) => {
    let d = await api('GET', '/api/announcements');
    const root = h('div', { class: 'page' });
    const listBox = h('div');
    let playing = null;
    App.onLeave(() => { if (playing) playing.audio.pause(); });

    const play = (id, btn) => {
      if (playing) {
        playing.audio.pause();
        playing.btn.replaceChildren(icon('play'));
        if (playing.id === id) { playing = null; return; }
      }
      const audio = new Audio('/api/announcements/' + id + '/audio');
      audio.addEventListener('ended', () => { btn.replaceChildren(icon('play')); playing = null; });
      audio.play().catch(App.err);
      btn.replaceChildren(icon('stop'));
      playing = { id, audio, btn };
    };

    const srcText = (a) => ({
      file: 'Dosya' + (a.fileName ? ' · ' + a.fileName : ''),
      mic: 'Mikrofon kaydı',
      tts: 'Yazıdan · ' + ((d.voices.find((v) => v.id === a.voice) || {}).name || a.voice || ''),
    }[a.source] || a.source);

    function renderList() {
      const ul = h('ul', { class: 'list' });
      for (const { ann: a, users } of d.anns) {
        const pb = h('button', { type: 'button', class: 'icon-btn round', 'aria-label': a.name + ' anonsunu dinle' }, icon('play'));
        pb.addEventListener('click', () => play(a.id, pb));
        ul.append(h('li', null, pb,
          h('div', { class: 'grow stack', style: { flex: '1 1 240px' } },
            h('div', { class: 'row wrap' }, h('b', null, a.name), h('span', { class: 'mono small muted' }, fmt.ms(a.durMs)),
              h('span', { class: 'chip', style: { background: 'none', border: '1px solid var(--border-2)', color: 'var(--text-2)' } }, srcText(a))),
            a.text ? h('span', { class: 'small', style: { color: 'var(--text-2)' } }, '“' + a.text + '”') : null,
            h('div', { class: 'row wrap', style: { gap: '6px' } },
              users.length ? users.map((u) => h('span', { class: 'chip' }, u)) : h('span', { class: 'small muted' }, 'Henüz bir hatta kullanılmıyor'))),
          h('div', { class: 'row' },
            h('button', { type: 'button', class: 'icon-btn', 'aria-label': a.name + ' anonsunu yeniden adlandır', onclick: async () => {
              const name = await App.prompt('Anonsu yeniden adlandır', 'Anons adı', a.name);
              if (name === null) return;
              try { await api('PUT', '/api/announcements/' + a.id, { name }); d = await api('GET', '/api/announcements'); renderList(); } catch (e) { App.err(e); }
            } }, icon('edit')),
            h('a', { class: 'icon-btn', href: '/api/announcements/' + a.id + '/audio?download=1', 'aria-label': a.name + ' anonsunu indir' }, icon('download')),
            h('button', { type: 'button', class: 'icon-btn danger', 'aria-label': a.name + ' anonsunu sil', onclick: async () => {
              if (users.length) { App.toast('Bu anons şu hatlarda kullanılıyor: ' + users.join(', '), 'err'); return; }
              if (!await App.confirm('“' + a.name + '” anonsu silinsin mi?', { ok: 'Sil', danger: true })) return;
              try { await api('DELETE', '/api/announcements/' + a.id); d = await api('GET', '/api/announcements'); renderList(); App.toast('Anons silindi', 'ok'); } catch (e) { App.err(e); }
            } }, icon('trash')))));
      }
      const total = d.anns.reduce((s, x) => s + x.ann.durMs, 0);
      listBox.replaceChildren(h('section', { class: 'card', 'aria-labelledby': 'kutuphane-h' },
        h('div', { class: 'card-head' }, h('h2', { id: 'kutuphane-h' }, 'Kayıtlı anonslar'), h('span', { class: 'small muted' }, d.anns.length + ' anons · toplam ' + fmt.ms(total))),
        d.anns.length ? ul : h('div', { class: 'empty' }, 'Henüz anons yok. Sağdan dosya yükleyin, mikrofonla kaydedin ya da yazıyı seslendirin.')));
    }

    // ---- new announcement panel
    let tab = 'file';
    let pending = null; // {blob, source, fileName} or {source:'tts', text, voice, speed}
    const nameInp = h('input', { class: 'input', placeholder: 'ör. Kayıt bildirimi', maxlength: '60' });
    const tabBox = h('div');
    const previewBox = h('div');
    const saveBtn = h('button', { type: 'button', class: 'btn primary', disabled: true }, 'Anonsu kaydet');
    const tts = { text: '', voice: (d.voices.find((v) => v.state === 'ready') || d.voices[0] || {}).id, speed: 1 };
    let rec = null; // mic recorder state

    const setPreview = (blob) => {
      const url = URL.createObjectURL(blob);
      const au = h('audio', { controls: true, src: url });
      previewBox.replaceChildren(h('div', { class: 'preview' }, icon('volume'), au));
      saveBtn.disabled = false;
    };
    const clearPending = () => { pending = null; previewBox.replaceChildren(); saveBtn.disabled = true; };

    const handleFile = async (file) => {
      if (!file) return;
      if (file.size > 60 * 1024 * 1024) { App.toast('Dosya en fazla 60 MB olabilir', 'err'); return; }
      previewBox.replaceChildren(h('p', { class: 'small muted' }, 'Dosya okunuyor…'));
      try {
        const { blob } = await toWav(await file.arrayBuffer());
        pending = { blob, source: 'file', fileName: file.name };
        if (!nameInp.value) nameInp.value = file.name.replace(/\.[^.]+$/, '');
        setPreview(blob);
      } catch (e) {
        clearPending();
        App.toast('Ses dosyası okunamadı: ' + (e.message || e), 'err');
      }
    };

    function stopMic() {
      if (!rec) return;
      try { rec.mr.state !== 'inactive' && rec.mr.stop(); } catch (e) { /* ignore */ }
      rec.stream.getTracks().forEach((t) => t.stop());
      clearInterval(rec.timer);
      if (rec.ctx) rec.ctx.close();
    }
    App.onLeave(stopMic);

    function renderTab() {
      const panels = [];
      if (tab === 'file') {
        const inp = h('input', { type: 'file', accept: 'audio/*,.mp3,.wav,.m4a,.ogg,.aac', class: 'sr-only' });
        inp.addEventListener('change', () => handleFile(inp.files[0]));
        const drop = h('label', { class: 'drop' }, icon('upload'), h('b', null, 'Dosyayı buraya bırak ya da seç'),
          h('span', { class: 'small muted' }, 'MP3, WAV, M4A, OGG · en çok 5 dakika'), inp);
        drop.addEventListener('dragover', (e) => { e.preventDefault(); drop.classList.add('over'); });
        drop.addEventListener('dragleave', () => drop.classList.remove('over'));
        drop.addEventListener('drop', (e) => { e.preventDefault(); drop.classList.remove('over'); handleFile(e.dataTransfer.files[0]); });
        panels.push(drop);
      } else if (tab === 'mic') {
        if (!window.isSecureContext || !navigator.mediaDevices) {
          panels.push(h('div', { class: 'callout warn' }, icon('alert'), h('span', null, 'Mikrofon yalnız güvenli bağlantıda (HTTPS) çalışır. Paneli https:// adresiyle açın.')));
        } else {
          const timeEl = h('span', { class: 'mono', style: { fontSize: '22px', fontWeight: '600' } }, '00:00');
          const meter = h('div', { class: 'meter', 'aria-hidden': 'true' }, ...Array.from({ length: 20 }, () => h('span')));
          const btn = h('button', { type: 'button', class: 'mic-btn', 'aria-label': 'Kaydı başlat' }, icon('mic'));
          btn.addEventListener('click', async () => {
            if (rec && rec.mr.state === 'recording') { stopMic(); return; }
            try {
              const stream = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true } });
              const mr = new MediaRecorder(stream);
              const chunks = [];
              mr.ondataavailable = (e) => { if (e.data.size) chunks.push(e.data); };
              const started = Date.now();
              const AC = window.AudioContext || window.webkitAudioContext;
              const ctx = new AC();
              const an = ctx.createAnalyser();
              an.fftSize = 256;
              ctx.createMediaStreamSource(stream).connect(an);
              const buf = new Uint8Array(an.fftSize);
              rec = { mr, stream, ctx, timer: setInterval(() => {
                const s = Math.floor((Date.now() - started) / 1000);
                timeEl.textContent = fmt.dur(s);
                an.getByteTimeDomainData(buf);
                let peak = 0;
                for (const v of buf) peak = Math.max(peak, Math.abs(v - 128));
                const lit = Math.round(Math.min(1, peak / 90) * 20);
                [...meter.children].forEach((sp, i) => { sp.classList.toggle('on', i < lit); sp.style.height = (i < lit ? 6 + i : 3) + 'px'; });
                if (s >= 300) stopMic();
              }, 80) };
              mr.onstop = async () => {
                btn.classList.remove('recording');
                btn.setAttribute('aria-label', 'Kaydı başlat');
                btn.replaceChildren(icon('mic'));
                try {
                  const blob = new Blob(chunks, { type: mr.mimeType });
                  const out = await toWav(await blob.arrayBuffer());
                  pending = { blob: out.blob, source: 'mic', fileName: '' };
                  setPreview(out.blob);
                } catch (e) { App.toast('Kayıt işlenemedi: ' + (e.message || e), 'err'); }
              };
              mr.start(250);
              btn.classList.add('recording');
              btn.setAttribute('aria-label', 'Kaydı bitir');
              btn.replaceChildren(icon('stop'));
              clearPending();
            } catch (e) { App.toast('Mikrofon açılamadı: ' + (e.message || e), 'err'); }
          });
          panels.push(h('div', { class: 'mic-box' }, btn, timeEl, meter,
            h('span', { class: 'small muted', style: { textAlign: 'center', maxWidth: '300px' } }, 'Tarayıcı mikrofon izni ister. Sessiz bir yerde, telefona konuşur gibi kaydedin; bitirmek için tekrar basın.')));
        }
      } else {
        if (!d.ttsAvailable) {
          panels.push(h('div', { class: 'callout warn' }, icon('alert'), h('span', null, 'Yazıdan seslendirme yalnız NAS üzerinde çalışır.')));
        }
        const ta = h('textarea', { class: 'input', rows: '4', maxlength: '1500', placeholder: 'Merhaba, görüşmeniz kalite amacıyla kaydedilmektedir.' }, tts.text);
        ta.addEventListener('input', () => { tts.text = ta.value; clearPending(); });
        const voice = d.voices.find((v) => v.id === tts.voice) || {};
        const sel = h('select', { class: 'input', onchange: (e) => { tts.voice = e.target.value; clearPending(); renderTab(); } },
          ...d.voices.map((v) => h('option', { value: v.id, selected: v.id === tts.voice ? 'selected' : null }, v.name + (v.state === 'ready' ? '' : ' (indirilmedi)'))));
        const speedLbl = h('span', { class: 'mono', style: { fontWeight: '400' } }, tts.speed.toFixed(2).replace('.', ',') + '×');
        const range = h('input', { type: 'range', min: '0.7', max: '1.3', step: '0.05', value: String(tts.speed), style: { accentColor: 'var(--primary)', minHeight: '44px', margin: '0' } });
        range.addEventListener('input', () => { tts.speed = parseFloat(range.value); speedLbl.textContent = tts.speed.toFixed(2).replace('.', ',') + '×'; clearPending(); });
        let voiceState;
        if (voice.state === 'ready') voiceState = h('span', { class: 'small', style: { color: 'var(--ok)' } }, 'Ses hazır');
        else if (voice.state === 'downloading') {
          const pct = voice.size ? Math.round((voice.done || 0) / voice.size * 100) : 0;
          voiceState = h('span', { class: 'row small' }, h('div', { class: 'progress' }, h('div', { style: { width: pct + '%' } })), 'İndiriliyor %' + pct);
        } else {
          voiceState = h('span', { class: 'row small' }, voice.state === 'error' ? h('span', { class: 'err-text' }, voice.error) : null,
            h('button', { type: 'button', class: 'btn small', disabled: !d.ttsAvailable, onclick: async () => {
              try { await api('POST', '/api/tts/voices/' + voice.id + '/download'); d = await api('GET', '/api/announcements'); renderTab(); } catch (e) { App.err(e); }
            } }, icon('download'), 'Sesi indir (' + fmt.bytes(voice.size) + ')'));
        }
        const listenBtn = h('button', { type: 'button', class: 'btn', disabled: voice.state !== 'ready' }, icon('play'), 'Önce dinle');
        listenBtn.addEventListener('click', async () => {
          if (!tts.text.trim()) { App.toast('Okunacak metni yazın', 'err'); return; }
          listenBtn.disabled = true;
          listenBtn.lastChild.textContent = 'Seslendiriliyor…';
          try {
            const r = await api('POST', '/api/tts/preview', { text: tts.text, voice: tts.voice, speed: tts.speed }, { raw: true });
            const blob = await r.blob();
            pending = { source: 'tts', text: tts.text, voice: tts.voice, speed: tts.speed };
            setPreview(blob);
            previewBox.querySelector('audio').play().catch(() => {});
          } catch (e) { App.err(e); }
          listenBtn.disabled = false;
          listenBtn.lastChild.textContent = 'Önce dinle';
        });
        panels.push(h('div', { class: 'stack', style: { gap: '14px' } },
          h('label', { class: 'field' }, 'Okunacak metin', ta),
          h('div', { class: 'row wrap', style: { gap: '12px 16px', alignItems: 'flex-end' } },
            h('label', { class: 'field', style: { flex: '1 1 190px' } }, 'Ses', sel),
            h('label', { class: 'field', style: { flex: '1 1 150px' } }, h('span', null, 'Okuma hızı · ', speedLbl), range)),
          h('div', { class: 'row wrap', style: { justifyContent: 'space-between' } }, voiceState, listenBtn),
          h('span', { class: 'small muted' }, 'Ses NAS’ın kendisinde üretilir; metin internete gönderilmez. Sesler bir kez indirilir (~60 MB).')));
      }
      tabBox.replaceChildren(...panels);
    }

    const tabs = App.seg([['file', 'Dosya yükle'], ['mic', 'Mikrofonla kaydet'], ['tts', 'Yazıdan seslendir']], tab, (v) => {
      stopMic();
      tab = v;
      clearPending();
      renderTab();
    }, { cls: 'fill tall', label: 'Anons kaynağı' });

    saveBtn.addEventListener('click', async () => {
      if (!pending) return;
      if (!nameInp.value.trim()) { App.toast('Anons adını yazın', 'err'); nameInp.focus(); return; }
      saveBtn.disabled = true;
      saveBtn.textContent = 'Kaydediliyor…';
      try {
        if (pending.source === 'tts') {
          await api('POST', '/api/announcements', { name: nameInp.value, text: pending.text, voice: pending.voice, speed: pending.speed });
        } else {
          const fd = new FormData();
          fd.append('name', nameInp.value);
          fd.append('source', pending.source);
          fd.append('fileName', pending.fileName || '');
          fd.append('audio', pending.blob, 'anons.wav');
          await api('POST', '/api/announcements', fd);
        }
        App.toast('Anons kaydedildi', 'ok');
        nameInp.value = '';
        tts.text = '';
        clearPending();
        d = await api('GET', '/api/announcements');
        renderList();
        renderTab();
      } catch (e) { App.err(e); saveBtn.disabled = false; }
      saveBtn.textContent = 'Anonsu kaydet';
    });

    renderList();
    renderTab();
    root.append(
      h('div', { class: 'page-head' }, h('div', null, h('h1', null, 'Anonslar'), h('p', null, 'Arayana çalınan sesler. Bir anonsu istediğin kadar hatta kullanabilirsin.'))),
      h('div', { class: 'cols' },
        h('div', { class: 'col-3' }, listBox),
        h('section', { class: 'card col-2', 'aria-labelledby': 'yeni-h' },
          h('div', { class: 'card-head' }, h('h2', { id: 'yeni-h' }, 'Yeni anons')),
          h('div', { class: 'card-body' },
            h('label', { class: 'field' }, 'Anons adı', nameInp), tabs, tabBox, previewBox,
            h('p', { class: 'callout plain small' }, icon('volume'), h('span', null, 'Kaydederken telefona uygun hale getirilir: 8 kHz, mono, ses seviyesi dengelenir.')),
            h('div', { class: 'actions', style: { justifyContent: 'flex-end' } }, saveBtn)))));
    main.replaceChildren(root);
    App.poll(async () => {
      if (!d.voices.some((v) => v.state === 'downloading')) return;
      d = await api('GET', '/api/announcements');
      if (tab === 'tts') renderTab();
    }, 2000);
  };

  // ---------------------------------------------------------------- Aramalar
  App.pages.aramalar = async (main) => {
    const params = App.params();
    const q = { range: params.range || 'today', from: '', to: '', line: '', result: '', q: '', limit: 50 };
    let d = null;
    let items = [];
    const selected = new Set();
    let current = null; // record playing
    const root = h('div', { class: 'page' });
    const summaryEl = h('p', { class: 'row wrap', style: { gap: '6px 18px', color: 'var(--text-2)' } });
    const tableBox = h('section', { class: 'card', 'aria-label': 'Arama listesi' });
    const audio = new Audio();
    audio.preload = 'metadata';
    App.onLeave(() => audio.pause());

    const qs = (extra) => {
      const p = new URLSearchParams();
      for (const [k, v] of Object.entries(Object.assign({}, q, extra))) if (v !== '' && v !== undefined) p.set(k, v);
      return p.toString();
    };
    async function load(append) {
      const r = await api('GET', '/api/calls?' + qs({ offset: append ? items.length : 0 }));
      d = r;
      items = append ? items.concat(r.items) : r.items;
      renderSummary();
      renderTable();
    }

    // ---- player
    const playBtn = h('button', { type: 'button', class: 'play-big', 'aria-label': 'Oynat', disabled: true }, icon('play'));
    const seek = h('input', { type: 'range', min: '0', max: '1000', value: '0', 'aria-label': 'Konum', disabled: true });
    const tCur = h('span', { class: 'mono' }, '00:00');
    const tDur = h('span', { class: 'mono' }, '00:00');
    const pTitle = h('b', { class: 'mono', style: { fontSize: '15px' } }, 'Kayıt seçilmedi');
    const pMeta = h('span', { class: 'small muted' }, 'Dinlemek için listeden bir kaydın oynat düğmesine basın.');
    const pPath = h('span', { class: 'path mono' });
    const dl = h('a', { class: 'btn', 'aria-disabled': 'true' }, icon('download'), 'İndir');
    let speed = 1;
    const speedSeg = App.seg([['1', '1×'], ['1.5', '1,5×'], ['2', '2×']], '1', (v) => { speed = parseFloat(v); audio.playbackRate = speed; }, { label: 'Oynatma hızı' });
    playBtn.addEventListener('click', () => { if (audio.paused) audio.play().catch(App.err); else audio.pause(); });
    audio.addEventListener('play', () => { playBtn.replaceChildren(icon('pause')); playBtn.setAttribute('aria-label', 'Duraklat'); });
    audio.addEventListener('pause', () => { playBtn.replaceChildren(icon('play')); playBtn.setAttribute('aria-label', 'Oynat'); });
    audio.addEventListener('timeupdate', () => {
      tCur.textContent = fmt.dur(audio.currentTime);
      if (audio.duration) seek.value = String(Math.round(audio.currentTime / audio.duration * 1000));
    });
    audio.addEventListener('loadedmetadata', () => { tDur.textContent = fmt.dur(audio.duration); audio.playbackRate = speed; });
    seek.addEventListener('input', () => { if (audio.duration) audio.currentTime = seek.value / 1000 * audio.duration; });
    const select = (r, autoplay) => {
      current = r;
      audio.src = '/api/calls/' + encodeURIComponent(r.id) + '/audio';
      playBtn.disabled = false;
      seek.disabled = false;
      pTitle.textContent = r.caller;
      pMeta.textContent = fmt.date(r.start) + ' ' + fmt.time(r.start) + ' · ' + r.lineName + ' · ' +
        (r.answeredBy ? r.answeredBy + (r.answeredByName ? ' ' + r.answeredByName : '') + ' açtı' : App.resultText[r.result]);
      pPath.textContent = (r.recRoot || d.recordingsRoot) + '/' + r.rec;
      dl.href = audio.src + '?download=1';
      dl.removeAttribute('aria-disabled');
      tCur.textContent = '00:00';
      tDur.textContent = fmt.dur(r.recSec);
      if (autoplay) audio.play().catch(() => {});
      renderTable();
    };
    const player = h('section', { class: 'card player', 'aria-label': 'Kayıt oynatıcı' }, playBtn,
      h('div', { class: 'mid' }, h('div', { class: 'row wrap', style: { gap: '2px 12px', alignItems: 'baseline' } }, pTitle, pMeta), seek,
        h('div', { class: 'times' }, tCur, tDur)),
      h('div', { class: 'row wrap' }, speedSeg, dl), pPath);

    // ---- filters
    const customBox = h('div', { class: 'row', hidden: q.range !== 'custom' });
    const fromInp = h('input', { type: 'date', class: 'input', 'aria-label': 'Başlangıç tarihi' });
    const toInp = h('input', { type: 'date', class: 'input', 'aria-label': 'Bitiş tarihi' });
    const todayStr = new Date().toISOString().slice(0, 10);
    fromInp.value = toInp.value = todayStr;
    const onDates = () => { q.from = fromInp.value; q.to = toInp.value; load().catch(App.err); };
    fromInp.addEventListener('change', onDates);
    toInp.addEventListener('change', onDates);
    customBox.append(fromInp, h('span', { class: 'muted' }, '–'), toInp);
    const rangeSeg = App.seg([['today', 'Bugün'], ['yesterday', 'Dün'], ['7d', '7 gün'], ['30d', '30 gün'], ['all', 'Tümü'], ['custom', 'Tarih seç']], q.range, (v) => {
      q.range = v;
      customBox.hidden = v !== 'custom';
      if (v === 'custom') { q.from = fromInp.value; q.to = toInp.value; }
      load().catch(App.err);
    }, { label: 'Tarih' });
    const lineSel = h('select', { class: 'input', onchange: (e) => { q.line = e.target.value; load().catch(App.err); } }, h('option', { value: '' }, 'Tüm hatlar'));
    const resSel = h('select', { class: 'input', onchange: (e) => { q.result = e.target.value; load().catch(App.err); } },
      h('option', { value: '' }, 'Tümü'), ...['answered', 'missed', 'voicemail', 'announcement', 'busy'].map((k) => h('option', { value: k }, App.resultText[k])));
    const searchInp = h('input', { type: 'search', class: 'input mono', placeholder: 'Numara ara' });
    let st = null;
    searchInp.addEventListener('input', () => { clearTimeout(st); st = setTimeout(() => { q.q = searchInp.value; load().catch(App.err); }, 300); });

    function renderSummary() {
      const s = d.summary;
      summaryEl.replaceChildren(
        h('span', null, h('b', { style: { color: 'var(--text)' } }, String(s.total)), ' arama'),
        h('span', null, h('b', { style: { color: 'var(--ok)' } }, String(s.answered)), ' cevaplandı'),
        h('span', null, h('b', { style: { color: 'var(--warn)' } }, String(s.missed)), ' cevapsız'),
        h('span', null, h('b', { style: { color: 'var(--primary)' } }, String(s.voicemail)), ' sesli mesaj'),
        h('span', null, h('b', { style: { color: 'var(--text)' } }, String(s.announcement)), ' sadece anons'),
        h('span', null, h('b', { style: { color: 'var(--busy)' } }, String(s.busy || 0)), ' meşgul'),
        h('span', null, 'Toplam kayıt ', h('b', { style: { color: 'var(--text)' } }, fmt.durLong(s.recSec)), ' · ' + fmt.bytes(s.recBytes)));
      if (lineSel.options.length - 1 !== d.lines.length) {
        lineSel.replaceChildren(h('option', { value: '' }, 'Tüm hatlar'), ...d.lines.map((l) => h('option', { value: l.id, selected: q.line === l.id ? 'selected' : null }, l.name)));
      }
    }

    const cols = '44px 64px minmax(170px, 1.3fr) minmax(130px, 1fr) minmax(150px, 1fr) minmax(110px, .8fr) 64px 104px';
    const zipBtn = h('button', { type: 'button', class: 'btn primary', disabled: true }, icon('download'), 'Seçilenleri ZIP indir');
    function renderTable() {
      const tbl = h('div', { class: 'tbl tbl-calls', role: 'table', 'aria-label': 'Aramalar' });
      const withRec = items.filter((r) => r.rec);
      const all = h('input', { type: 'checkbox', 'aria-label': 'Tümünü seç', checked: withRec.length > 0 && withRec.every((r) => selected.has(r.id)) });
      all.addEventListener('change', () => { for (const r of withRec) all.checked ? selected.add(r.id) : selected.delete(r.id); renderTable(); });
      tbl.append(h('div', { class: 'tr th', role: 'row', style: { gridTemplateColumns: cols } },
        h('span', { role: 'columnheader', class: 'row', style: { justifyContent: 'center' } }, all),
        ...['Saat', 'Arayan', 'Hat', 'Sonuç', 'Açan'].map((t) => h('span', { role: 'columnheader' }, t)),
        h('span', { role: 'columnheader', class: 'right' }, 'Süre'), h('span', { role: 'columnheader', class: 'right' }, 'Kayıt')));
      for (const c of d.live) {
        tbl.append(h('div', { class: 'tr', role: 'row', style: { gridTemplateColumns: cols } },
          h('span', { role: 'cell' }), h('span', { role: 'cell', class: 'mono muted' }, fmt.time(c.start)),
          h('span', { role: 'cell', class: 'mono', style: { fontWeight: '600' } }, c.caller), h('span', { role: 'cell' }, c.lineName),
          h('span', { role: 'cell' }, h('span', { class: 'res live' }, 'Sürüyor · ' + App.liveText(c).split(' · ')[0])),
          h('span', { role: 'cell', class: 'muted' }, c.by ? c.by + ' ' + (c.byName || '') : '—'),
          h('span', { role: 'cell', class: 'mono right' }, fmt.dur(c.seconds)),
          h('span', { role: 'cell', class: 'right' }, c.recording ? h('span', { class: 'rec' }, 'KAYITTA') : null)));
      }
      const showDate = q.range !== 'today';
      for (const r of items) {
        const cb = h('input', { type: 'checkbox', 'aria-label': r.caller + ', ' + fmt.time(r.start) + ' aramasını seç', checked: selected.has(r.id), disabled: !r.rec });
        cb.addEventListener('change', () => { cb.checked ? selected.add(r.id) : selected.delete(r.id); updateZip(); });
        tbl.append(h('div', { class: 'tr' + (current && current.id === r.id ? ' sel' : ''), role: 'row', style: { gridTemplateColumns: cols } },
          h('span', { role: 'cell', class: 'row', style: { justifyContent: 'center', height: '44px' } }, cb),
          h('span', { role: 'cell', class: 'mono muted cell-2' }, showDate ? h('span', { class: 'small' }, fmt.dateShort(r.start)) : null, fmt.time(r.start)),
          h('span', { role: 'cell', class: 'cell-2' }, h('span', { class: 'mono', style: { fontWeight: '600' } }, r.caller), r.callerName && r.callerName !== r.caller ? h('span', { class: 'sub' }, r.callerName) : null),
          h('span', { role: 'cell' }, r.lineName),
          h('span', { role: 'cell' }, h('span', { class: 'res ' + r.result }, App.resultText[r.result] || r.result)),
          h('span', { role: 'cell', style: { color: 'var(--text-2)' } }, r.answeredBy ? r.answeredBy + ' ' + (r.answeredByName || '') : '—'),
          h('span', { role: 'cell', class: 'mono right' }, fmt.dur(r.durSec)),
          h('span', { role: 'cell', class: 'row', style: { justifyContent: 'flex-end' } },
            r.rec ? [
              h('button', { type: 'button', class: 'icon-btn round', 'aria-label': r.caller + ', ' + fmt.time(r.start) + ' kaydını dinle', onclick: () => select(r, true) }, icon('play')),
              h('a', { class: 'icon-btn', href: '/api/calls/' + encodeURIComponent(r.id) + '/audio?download=1', 'aria-label': 'Kaydı indir' }, icon('download')),
            ] : h('span', { class: 'small muted' }, 'Kayıt yok'))));
      }
      if (!items.length && !d.live.length) tbl.append(h('div', { class: 'empty' }, 'Bu aralıkta arama yok.'));
      const more = items.length < d.total ? h('button', { type: 'button', class: 'linkish', onclick: () => load(true).catch(App.err) }, 'Daha fazla yükle') : null;
      tableBox.replaceChildren(h('div', { class: 'tbl-wrap' }, tbl),
        h('div', { class: 'card-foot' }, h('span', null, (items.length < d.total ? items.length + ' / ' + d.total + ' arama gösteriliyor · ' : d.total + ' arama '), more),
          h('span', null, d.retentionDays ? 'Kayıtlar ' + d.retentionDays + ' gün saklanır, sonra kendiliğinden silinir · ' : 'Kayıtlar süresiz saklanır · ', h('a', { href: '#/ayarlar' }, 'Ayarlar'))));
      updateZip();
    }
    function updateZip() {
      zipBtn.disabled = selected.size === 0;
      zipBtn.lastChild.textContent = selected.size ? selected.size + ' kaydı ZIP indir' : 'Seçilenleri ZIP indir';
    }
    zipBtn.addEventListener('click', async () => {
      zipBtn.disabled = true;
      try {
        const r = await api('POST', '/api/calls/zip', { ids: [...selected] }, { raw: true });
        const name = (r.headers.get('Content-Disposition') || '').match(/filename="([^"]+)"/);
        App.saveBlob(await r.blob(), name ? name[1] : 'kayitlar.zip');
      } catch (e) { App.err(e); }
      updateZip();
    });
    const csvBtn = h('a', { class: 'btn', href: '#', onclick: (e) => { e.preventDefault(); location.href = '/api/calls/export.csv?' + qs({ limit: '' }); } }, 'CSV olarak indir');

    root.append(
      h('div', { class: 'page-head' },
        h('div', null, h('h1', null, 'Aramalar ve kayıtlar'), h('p', null, 'Gelen her arama burada. Ses dosyaları NAS’ta kayıt klasöründe.')),
        h('div', { class: 'actions' }, csvBtn, zipBtn)),
      h('div', { class: 'card', style: { padding: '14px 18px' } },
        h('div', { class: 'row wrap', style: { gap: '12px 16px', alignItems: 'flex-end' } },
          h('fieldset', null, h('legend', null, 'Tarih'), h('div', { class: 'row wrap' }, rangeSeg, customBox)),
          h('label', { class: 'field', style: { flex: '1 1 160px' } }, 'Hat', lineSel),
          h('label', { class: 'field', style: { flex: '1 1 160px' } }, 'Sonuç', resSel),
          h('label', { class: 'field', style: { flex: '1 1 200px' } }, 'Arayan', searchInp))),
      summaryEl, player, tableBox);
    await load();
    main.replaceChildren(root);
    if (params.id) {
      const r = items.find((x) => x.id === params.id);
      if (r) select(r, false);
    }
    App.poll(async () => {
      const r = await api('GET', '/api/calls?' + qs({ offset: 0 }));
      const changed = r.total !== d.total || r.live.length !== d.live.length || JSON.stringify(r.live) !== JSON.stringify(d.live);
      d = Object.assign(d, { live: r.live, summary: r.summary, total: r.total });
      if (changed && items.length <= r.items.length) items = r.items;
      renderSummary();
      if (changed) renderTable();
    }, 4000);
  };
})();

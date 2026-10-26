// ochq.uz: hero network, tunnel diagram, install terminal and comments.
// No dependencies. Animations stop off-screen and in background tabs, and
// "reduce motion" gets still pictures.
(() => {
  'use strict';
  const T = JSON.parse(document.getElementById('i18n').textContent);
  const lang = document.documentElement.lang;
  const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;
  const small = innerWidth < 640;
  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];
  const sleep = ms => new Promise(r => setTimeout(r, ms));
  const esc = s => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

  // ---- visibility: run work only while an element is on screen ----
  function watch(el, onChange) {
    let seen = false;
    const update = () => onChange(seen && !document.hidden);
    new IntersectionObserver(([e]) => { seen = e.isIntersecting; update(); }).observe(el);
    document.addEventListener('visibilitychange', update);
  }
  // until resolves once el is visible (used between animation loops).
  function visibility(el) {
    let on = false, waiters = [];
    watch(el, v => { on = v; if (v) { waiters.forEach(f => f()); waiters = []; } });
    return () => on ? Promise.resolve() : new Promise(r => waiters.push(r));
  }
  function loop(el, frame) {
    let raf = 0;
    const tick = t => { frame(t); raf = requestAnimationFrame(tick); };
    watch(el, on => {
      if (on && !raf) raf = requestAnimationFrame(tick);
      if (!on && raf) { cancelAnimationFrame(raf); raf = 0; }
    });
  }

  // ---- copy buttons ----
  async function copyText(text) {
    try { await navigator.clipboard.writeText(text); return; } catch (_) { /* fall back below */ }
    const ta = document.createElement('textarea');
    ta.value = text; ta.style.position = 'fixed'; ta.style.opacity = '0';
    document.body.append(ta); ta.select(); document.execCommand('copy'); ta.remove();
  }
  document.addEventListener('click', async e => {
    const b = e.target.closest('.copy');
    if (!b) return;
    await copyText(b.dataset.copy ?? b.parentElement.querySelector('code').textContent);
    b.textContent = T.copied;
    setTimeout(() => { b.textContent = T.copy; }, 1500);
  });

  // ---- neural mesh (hero + faint backdrop of the diagram) ----
  function mesh(canvas, { density, max, pulses, mouse, trail, alpha }) {
    const x = canvas.getContext('2d');
    let W = 0, H = 0;
    const nodes = [], live = [], m = { x: -1e4, y: -1e4 };
    const fit = () => {
      const r = canvas.getBoundingClientRect(), d = devicePixelRatio || 1;
      W = r.width; H = r.height; canvas.width = W * d; canvas.height = H * d; x.setTransform(d, 0, 0, d, 0, 0);
      const want = Math.min(max, Math.round(W * H / density) / (small ? 2 : 1));
      while (nodes.length < want) nodes.push({ x: Math.random() * W, y: Math.random() * H, vx: (Math.random() - .5) * .25, vy: (Math.random() - .5) * .25, r: Math.random() * 1.6 + .6 });
      nodes.length = want;
      for (const n of nodes) { n.x = Math.min(n.x, W); n.y = Math.min(n.y, H); }
      x.fillStyle = '#030712'; x.fillRect(0, 0, W, H);
    };
    fit();
    addEventListener('resize', fit);
    if (mouse) {
      const host = canvas.parentElement;
      host.addEventListener('pointermove', e => { const r = canvas.getBoundingClientRect(); m.x = e.clientX - r.left; m.y = e.clientY - r.top; });
      host.addEventListener('pointerleave', () => { m.x = m.y = -1e4; });
    }
    const D = 140;
    return function frame() {
      if (trail) { x.fillStyle = 'rgba(3,7,18,.35)'; x.fillRect(0, 0, W, H); } else x.clearRect(0, 0, W, H);
      for (const n of nodes) {
        n.x += n.vx; n.y += n.vy;
        if (n.x < 0 || n.x > W) n.vx *= -1;
        if (n.y < 0 || n.y > H) n.vy *= -1;
      }
      x.lineWidth = .7;
      for (let i = 0; i < nodes.length; i++) for (let j = i + 1; j < nodes.length; j++) {
        const a = nodes[i], b = nodes[j], d = Math.hypot(a.x - b.x, a.y - b.y);
        if (d > D) continue;
        const near = mouse ? Math.max(0, 1 - Math.hypot((a.x + b.x) / 2 - m.x, (a.y + b.y) / 2 - m.y) / 220) : 0;
        x.strokeStyle = `rgba(47,107,255,${(1 - d / D) * alpha + near * .4})`;
        x.beginPath(); x.moveTo(a.x, a.y); x.lineTo(b.x, b.y); x.stroke();
        if (pulses && Math.random() < .0009 + near * .01) live.push({ a, b, t: 0, s: .012 + Math.random() * .02 });
      }
      for (let k = live.length - 1; k >= 0; k--) {
        const p = live[k];
        if ((p.t += p.s) >= 1) { live.splice(k, 1); continue; }
        const px = p.a.x + (p.b.x - p.a.x) * p.t, py = p.a.y + (p.b.y - p.a.y) * p.t;
        const g = x.createRadialGradient(px, py, 0, px, py, 9);
        g.addColorStop(0, 'rgba(56,225,255,.95)'); g.addColorStop(1, 'rgba(56,225,255,0)');
        x.fillStyle = g; x.beginPath(); x.arc(px, py, 9, 0, 7); x.fill();
      }
      if (pulses) {
        x.fillStyle = 'rgba(150,190,255,.8)';
        for (const n of nodes) { x.beginPath(); x.arc(n.x, n.y, n.r, 0, 7); x.fill(); }
      }
    };
  }

  function initHero(sec) {
    const frame = mesh($('canvas', sec), { density: 9000, max: 140, pulses: true, mouse: true, trail: true, alpha: .28 });
    if (reduced) { for (let i = 0; i < 3; i++) frame(); return; }
    loop(sec, frame);
  }

  // ---- how it works: dial out → handshake → requests in → responses out ----
  function initHow(sec) {
    const stage = $('.stage', sec), svg = $('svg', stage), NS = 'http://www.w3.org/2000/svg';
    const meshFrame = mesh($('canvas', stage), { density: 12000, max: 55, pulses: false, trail: false, alpha: .12 });
    const steps = $$('.step', sec), cap = $('[data-cap]', stage), tube = $('[data-path="tube"]', svg);
    const url = $('[data-url]', svg), label = $('[data-tube-label]', svg), term = $('[data-term]', svg);
    const path = id => $(`[data-path="${id}"]`, svg);
    const step = (k, text) => {
      steps.forEach(e => e.classList.toggle('on', +e.dataset.s === k));
      cap.innerHTML = `<b>${k}.</b> ${esc(text)}`;
    };
    if (reduced) { steps.forEach(e => e.classList.add('on')); meshFrame(); return; }
    loop(stage, meshFrame);
    const visible = visibility(stage);
    const tween = (ms, fn) => new Promise(r => {
      const s = performance.now();
      (function f(now) {
        const t = Math.min(1, (now - s) / ms);
        fn(t < .5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2);
        t < 1 ? requestAnimationFrame(f) : r();
      })(s);
    });
    async function travel(color, segs, ms = 650) {
      const dot = document.createElementNS(NS, 'circle');
      dot.setAttribute('r', 7); dot.setAttribute('fill', color); dot.setAttribute('filter', 'url(#glow)');
      $('[data-packets]', svg).append(dot);
      for (const [id, a, b] of segs) {
        const p = path(id), L = p.getTotalLength();
        await tween(ms * (L / 200 + .4), t => { const q = p.getPointAtLength(L * (a + (b - a) * t)); dot.setAttribute('cx', q.x); dot.setAttribute('cy', q.y); });
      }
      dot.remove();
    }
    const request = v => travel('#a98bff', [[v, 1, 0], ['tube', 1, 0], ['local', 1, 0]]);
    const response = v => travel('#7dffb2', [['local', 0, 1], ['tube', 0, 1], [v, 0, 1]]);
    (async () => {
      for (;;) {
        await visible();
        tube.style.strokeDashoffset = 323; url.style.opacity = 0; label.style.opacity = 0; term.textContent = '$ ';
        step(1, T.cap1);
        for (const ch of 'ochq 3000') { term.textContent += ch; await sleep(90); }
        await tween(1400, t => { tube.style.strokeDashoffset = 323 * (1 - t); });
        step(2, T.cap2); label.style.opacity = 1;
        await travel('#38e1ff', [['tube', 0, 1]], 500); await travel('#38e1ff', [['tube', 1, 0]], 500);
        await tween(500, t => { url.style.opacity = t; }); await sleep(900);
        for (let round = 0; round < 2; round++) {
          step(3, T.cap3);
          await Promise.all(['v0', 'v1', 'v2'].map(async (v, i) => {
            await sleep(i * 450 + round * 200); await request(v);
            step(4, T.cap4); await response(v);
          }));
          await sleep(600);
        }
        await sleep(1800);
      }
    })();
  }

  // ---- install terminal: install, run, banner, online, logs ----
  const OS = {
    mac: { title: 'user@macbook: ~', prompt: '~ %', out: [
      ['==> <b>Tapping mukhiddinov/tap</b>', 300],
      ["Cloning into '/opt/homebrew/Library/Taps/mukhiddinov/homebrew-tap'...", 250],
      ['<span class="ok">✔︎</span> Tapped 1 cask (14 files, 9.8KB).', 300],
      ['==> <b>Downloading</b> https://github.com/mukhiddinov/ochq/releases/download/…', 200],
      ['@progress', 0],
      ['==> <b>Installing Cask ochq</b>', 300],
      ["==> Linking Binary 'ochq' to '/opt/homebrew/bin/ochq'", 250],
      ['🍺  ochq was successfully installed!', 700]] },
    linux: { title: 'user@ubuntu: ~', prompt: '~ $', out: [
      ['<b>==></b> Adding the OCHQ apt repository', 500],
      ['Get:1 https://mukhiddinov.github.io/ochq/apt stable InRelease', 250],
      ['Reading package lists... Done', 300],
      ['@progress', 0],
      ['Setting up ochq ...', 300],
      ['<b>==></b> Installed: ochq', 700]] },
    win: { title: 'PowerShell', prompt: 'PS C:\\Users\\user>', out: [
      ['The ochq bucket was added successfully.', 350],
      ["Installing 'ochq' [64bit] from 'ochq' bucket", 250],
      ['@progress', 0],
      ['Checking hash of ochq_windows_amd64.zip ... ok.', 250],
      ["Creating shim for 'ochq'.", 250],
      ["'ochq' was installed successfully!", 700]] },
  };
  const REQS = [['GET', '/', '200 OK', '12ms'], ['GET', '/assets/app.css', '200 OK', '4ms'], ['GET', '/assets/app.js', '200 OK', '6ms'],
    ['POST', '/api/login', '200 OK', '38ms'], ['GET', '/favicon.ico', '404 Not Found', '2ms']];

  function initInstall(sec) {
    const tabs = $$('[role="tab"]', sec), term = $('.term', sec), screen = $('[data-screen]', term), title = $('[data-title]', term);
    const version = term.dataset.version;
    const visible = reduced ? () => Promise.resolve() : visibility(term);
    let run = 0, banner = null, current = 'mac';

    const cmdOf = os => $(`[role="tabpanel"][data-os="${os}"] code`, sec).textContent.split('\n').join('; ');
    function select(tab, focus) {
      tabs.forEach(t => {
        const on = t === tab;
        t.setAttribute('aria-selected', on); t.tabIndex = on ? 0 : -1;
        $('#' + t.getAttribute('aria-controls')).hidden = !on;
      });
      if (focus) tab.focus();
      current = tab.dataset.os;
      if (banner) play();
    }
    tabs.forEach((t, i) => {
      t.addEventListener('click', () => select(t));
      t.addEventListener('keydown', e => {
        const d = { ArrowRight: 1, ArrowLeft: -1 }[e.key];
        if (d) { e.preventDefault(); select(tabs[(i + d + tabs.length) % tabs.length], true); }
      });
    });
    const ua = navigator.userAgent;
    select(tabs[/Mac/.test(ua) ? 0 : /Win/.test(ua) ? 2 : 1]);

    function bannerHTML(rows, withVersion) {
      const width = banner.final[0].length;
      return rows.map((l, r) => {
        let s = `<span class="b" style="color:${banner.colors[r]}">${esc(l.padEnd(width))}</span>`;
        if (withVersion && r === 4 && version) s += `   <span class="b u">${esc(version)}</span>`;
        if (withVersion && r === 5) s += `   <span class="d">${esc(banner.tagline)}</span>`;
        return s;
      });
    }

    async function play() {
      const id = ++run, os = OS[current];
      const wait = async ms => { if (!reduced) await sleep(ms); if (id !== run) throw 0; };
      let lines = [], live = '';
      const paint = (cursor = true) => {
        screen.innerHTML = lines.join('\n') + (lines.length ? '\n' : '') + live + (cursor ? '<span class="cur"></span>' : '');
        screen.scrollTop = screen.scrollHeight;
      };
      const P = `<span class="p">${esc(os.prompt)}</span> `;
      const type = async text => {
        if (reduced) { live = P + esc(text); return; }
        let buf = '';
        for (const ch of text) { buf += ch; live = P + esc(buf); paint(); await wait(28 + Math.random() * 45); }
      };
      title.textContent = os.title;
      try {
        await visible(); if (id !== run) return;
        live = P; paint(); await wait(700);
        await type(cmdOf(current)); await wait(350);
        lines.push(live); live = ''; paint(false);
        for (const [t, ms] of os.out) {
          if (t === '@progress') {
            for (let p = reduced ? 100 : 0; p <= 100; p += 4) { live = `${'#'.repeat(Math.round(p * .45)).padEnd(45)} ${p.toFixed(1)}%`; paint(false); await wait(28); }
            lines.push(live); live = ''; continue;
          }
          lines.push(t); paint(false); await wait(ms);
        }
        live = P; paint(); await wait(600);
        await type('ochq 3000'); await wait(300);
        lines.push(live); live = ''; lines.push('');
        const base = lines.length;
        const show = (rows, withVersion) => { lines.length = base; lines.push(...bannerHTML(rows, withVersion)); paint(false); };
        if (!reduced) for (const f of banner.frames) { show(f.rows); await wait(f.delay_ms); }
        show(banner.final, true); lines.push(''); paint(false); await wait(250);
        lines.push('<span class="d">[…]</span> Connecting to tunnel.ochq.uz:9000 ...'); paint(false); await wait(700);
        const ok = '<span class="ok">[✓]</span>';
        lines.push(`${ok} Status:       <span class="ok b">Online</span>`, `${ok} Forwarding:   http://localhost:3000`,
          `${ok} Public URL:   <span class="u">https://a7f3.ochq.uz</span>`, '', '<span class="d">Logs (Ctrl+C to stop):</span>');
        paint(false); await wait(1100);
        for (const [m, p, s, d] of REQS) {
          const t = new Date().toTimeString().slice(0, 8);
          lines.push(`<span class="d">${t}</span>  ${m.padEnd(6)} ${p.padEnd(18)}  <span style="color:${s[0] === '4' ? '#facc15' : '#4ade80'}">${s.padEnd(14)}</span>  <span class="d">${d}</span>`);
          paint(false); await wait(500 + Math.random() * 700);
        }
        if (reduced) return;
        await wait(3500);
        play();
      } catch (_) { /* a newer run took over */ }
    }

    fetch('/assets/banner.json').then(r => r.json()).then(b => { banner = b; play(); }).catch(() => { term.hidden = true; });
  }

  // ---- comments ----
  function ago(iso) {
    const s = (Date.parse(iso) - Date.now()) / 1000, rtf = new Intl.RelativeTimeFormat(lang, { numeric: 'auto' });
    for (const [u, n] of [['year', 31536000], ['month', 2592000], ['day', 86400], ['hour', 3600], ['minute', 60]]) {
      if (Math.abs(s) >= n) return rtf.format(Math.round(s / n), u);
    }
    return rtf.format(0, 'second');
  }

  function initComments(sec) {
    const list = $('[data-list]', sec), more = $('[data-more]', sec), form = $('[data-form]', sec), status = $('[data-status]', sec);
    const btn = $('button[type="submit"]', form);
    let oldest = 0;

    // Comments are text from strangers: only ever textContent, never markup.
    function item(c) {
      const li = document.createElement('li'), who = document.createElement('div'), name = document.createElement(c.name ? 'b' : 'span');
      const badge = document.createElement('span'), time = document.createElement('time'), body = document.createElement('p');
      who.className = 'who';
      name.textContent = c.name || T.ghost;
      if (!c.name) name.className = 'ghost';
      badge.className = 'badge'; badge.textContent = c.lang.toUpperCase();
      time.dateTime = c.created_at; time.textContent = ago(c.created_at);
      body.textContent = c.body;
      who.append(name, badge, time); li.append(who, body);
      return li;
    }
    function empty() {
      if (list.children.length) return;
      const li = document.createElement('li'); li.className = 'none'; li.textContent = T.empty; list.append(li);
    }
    async function load() {
      const r = await fetch('/api/comments' + (oldest ? '?before=' + oldest : ''));
      if (!r.ok) return;
      const cs = await r.json();
      for (const c of cs) list.append(item(c));
      if (cs.length) oldest = cs[cs.length - 1].id;
      more.hidden = cs.length < 20;
      empty();
    }
    more.addEventListener('click', load);
    form.addEventListener('submit', async e => {
      e.preventDefault();
      const f = new FormData(form);
      btn.disabled = true; btn.textContent = T.f_sending; status.textContent = ''; status.classList.remove('err');
      try {
        const r = await fetch('/api/comments', { method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name: f.get('name'), body: f.get('body'), lang, website: f.get('website') }) });
        if (r.status === 201) {
          const c = await r.json();
          $('.none', list)?.remove();
          if (c.id) list.prepend(item(c));
          form.reset();
        } else {
          status.textContent = r.status === 429 ? T.err_rate : T.err_bad; status.classList.add('err');
        }
      } catch (_) {
        status.textContent = T.err_net; status.classList.add('err');
      } finally {
        btn.disabled = false; btn.textContent = T.f_send;
      }
    });
    load().catch(() => {});
  }

  initHero($('#hero'));
  initHow($('#how'));
  initInstall($('#install'));
  initComments($('#comments'));
})();

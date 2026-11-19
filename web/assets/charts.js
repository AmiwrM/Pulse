/* Pulse charts — tiny dependency-free canvas charts with hover tooltips. */

const fa = (s) => String(s).replace(/\d/g, (d) => '۰۱۲۳۴۵۶۷۸۹'[Number(d)]);
const fmtTime = new Intl.DateTimeFormat('fa-IR', { hour: '2-digit', minute: '2-digit' });
const fmtDateTime = new Intl.DateTimeFormat('fa-IR', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
const fmtNum = new Intl.NumberFormat('fa-IR');

function prep(canvas) {
  const dpr = Math.min(window.devicePixelRatio || 1, 2);
  const rect = canvas.getBoundingClientRect();
  const w = Math.max(1, Math.round(rect.width));
  const h = Math.max(1, Math.round(rect.height));
  if (canvas.width !== w * dpr || canvas.height !== h * dpr) {
    canvas.width = w * dpr;
    canvas.height = h * dpr;
  }
  const ctx = canvas.getContext('2d');
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, w, h);
  return { ctx, w, h };
}

function watch(canvas, draw) {
  if (canvas._pulseRO) canvas._pulseRO.disconnect();
  let last = '';
  const ro = new ResizeObserver(() => {
    const box = canvas.getBoundingClientRect();
    const key = `${Math.round(box.width)}x${Math.round(box.height)}`;
    if (key === last) return;
    last = key;
    if (canvas.isConnected) draw();
  });
  ro.observe(canvas);
  canvas._pulseRO = ro;
  canvas._pulseDraw = draw;
  draw();
}

function smoothPath(ctx, pts) {
  if (!pts.length) return;
  ctx.moveTo(pts[0].x, pts[0].y);
  if (pts.length === 1) return;
  for (let i = 0; i < pts.length - 1; i++) {
    const p0 = pts[i - 1] || pts[i];
    const p1 = pts[i];
    const p2 = pts[i + 1];
    const p3 = pts[i + 2] || p2;
    const t = 0.18;
    ctx.bezierCurveTo(
      p1.x + (p2.x - p0.x) * t, p1.y + (p2.y - p0.y) * t,
      p2.x - (p3.x - p1.x) * t, p2.y - (p3.y - p1.y) * t,
      p2.x, p2.y
    );
  }
}

function niceMax(v) {
  if (!Number.isFinite(v) || v <= 0) return 10;
  const pow = Math.pow(10, Math.floor(Math.log10(v)));
  const n = v / pow;
  const step = n <= 1 ? 1 : n <= 2 ? 2 : n <= 5 ? 5 : 10;
  return step * pow;
}

export function lineChart(canvas, options = {}) {
  const series = (options.series || []).map((s) => ({
    name: s.name || '',
    color: s.color || '#38bdf8',
    fill: !!s.fill,
    points: (s.points || [])
      .filter((p) => p && Number.isFinite(Number(p.y)))
      .map((p) => ({ x: Number(p.x) || 0, y: Number(p.y) || 0 })),
  })).filter((s) => s.points.length);
  const parent = canvas.parentElement;
  let tip = parent ? parent.querySelector(':scope > .chart-tooltip') : null;
  let hoverIndex = -1;

  function draw() {
    const { ctx, w, h } = prep(canvas);
    if (!series.length) return;
    const axis = { t: 8, r: 10, b: 22, l: options.yAxis === false ? 10 : 42 };
    const cw = Math.max(10, w - axis.l - axis.r);
    const ch = Math.max(10, h - axis.t - axis.b);
    const all = series.flatMap((s) => s.points);
    const xs = all.map((p) => p.x);
    const minX = Math.min(...xs);
    const maxX = Math.max(...xs);
    const spanX = maxX - minX || 1;
    const maxYRaw = Math.max(...all.map((p) => p.y), 1);
    const yMax = options.yMax || niceMax(maxYRaw * 1.12);
    const X = (x) => axis.l + ((x - minX) / spanX) * cw;
    const Y = (y) => axis.t + ch - (Math.min(y, yMax) / yMax) * ch;

    ctx.font = '9px IRANSans, Tahoma, sans-serif';
    ctx.fillStyle = 'rgba(135,151,183,.72)';
    ctx.strokeStyle = 'rgba(147,178,255,.08)';
    ctx.lineWidth = 1;
    const ticks = 4;
    for (let i = 0; i <= ticks; i++) {
      const y = axis.t + (ch / ticks) * i;
      ctx.beginPath();
      ctx.moveTo(axis.l, y + 0.5);
      ctx.lineTo(axis.l + cw, y + 0.5);
      ctx.stroke();
      if (options.yAxis !== false) {
        const val = yMax - (yMax / ticks) * i;
        const label = options.yFormat ? options.yFormat(val) : fa(Math.round(val));
        ctx.textAlign = 'right';
        ctx.textBaseline = 'middle';
        ctx.fillText(label, axis.l - 9, y);
      }
    }

    // x labels
    const labelCount = Math.max(2, Math.min(6, Math.floor(cw / 75)));
    if (options.xAxis !== false) {
      ctx.textAlign = 'center';
      ctx.textBaseline = 'top';
      for (let i = 0; i < labelCount; i++) {
        const t = minX + (spanX / (labelCount - 1 || 1)) * i;
        const x = X(t);
        const label = options.xFormat ? options.xFormat(t) : fmtTime.format(new Date(t * 1000));
        ctx.fillStyle = 'rgba(135,151,183,.6)';
        ctx.fillText(label, Math.max(axis.l + 14, Math.min(x, axis.l + cw - 14)), axis.t + ch + 9);
      }
    }

    series.forEach((s) => {
      const pts = s.points.map((p) => ({ x: X(p.x), y: Y(p.y), rawX: p.x, rawY: p.y }));
      if (s.fill) {
        const grad = ctx.createLinearGradient(0, axis.t, 0, axis.t + ch);
        grad.addColorStop(0, hexA(s.color, 0.22));
        grad.addColorStop(0.65, hexA(s.color, 0.05));
        grad.addColorStop(1, hexA(s.color, 0));
        ctx.beginPath();
        smoothPath(ctx, pts);
        ctx.lineTo(pts[pts.length - 1].x, axis.t + ch);
        ctx.lineTo(pts[0].x, axis.t + ch);
        ctx.closePath();
        ctx.fillStyle = grad;
        ctx.fill();
      }
      ctx.beginPath();
      smoothPath(ctx, pts);
      ctx.strokeStyle = s.color;
      ctx.lineWidth = 2;
      ctx.lineJoin = 'round';
      ctx.lineCap = 'round';
      ctx.shadowColor = hexA(s.color, 0.35);
      ctx.shadowBlur = 9;
      ctx.stroke();
      ctx.shadowBlur = 0;

      if (pts.length === 1) {
        ctx.beginPath();
        ctx.arc(pts[0].x, pts[0].y, 3, 0, Math.PI * 2);
        ctx.fillStyle = s.color;
        ctx.fill();
      }
    });

    // hover line + dots
    if (hoverIndex >= 0) {
      const base = series[0].points;
      const p = base[Math.min(hoverIndex, base.length - 1)];
      if (p) {
        const hx = X(p.x);
        ctx.strokeStyle = 'rgba(147,178,255,.18)';
        ctx.beginPath();
        ctx.moveTo(hx, axis.t);
        ctx.lineTo(hx, axis.t + ch);
        ctx.stroke();
        series.forEach((s) => {
          const match = nearest(s.points, p.x);
          if (!match) return;
          const cx = X(match.x);
          const cy = Y(match.y);
          ctx.beginPath();
          ctx.arc(cx, cy, 3.2, 0, Math.PI * 2);
          ctx.fillStyle = '#070d1a';
          ctx.fill();
          ctx.beginPath();
          ctx.arc(cx, cy, 2.2, 0, Math.PI * 2);
          ctx.fillStyle = s.color;
          ctx.fill();
        });
      }
    }
  }

  function nearest(points, x) {
    let best = null;
    let dist = Infinity;
    for (const p of points) {
      const d = Math.abs(p.x - x);
      if (d < dist) { dist = d; best = p; }
    }
    return best;
  }

  function showTip(clientX) {
    if (!parent || !series.length) return;
    const base = series[0].points;
    const rect = canvas.getBoundingClientRect();
    const axisL = options.yAxis === false ? 10 : 42;
    const axisR = 10;
    const cw = Math.max(10, rect.width - axisL - axisR);
    const all = series.flatMap((s) => s.points);
    const minX = Math.min(...all.map((p) => p.x));
    const maxX = Math.max(...all.map((p) => p.x));
    const ratio = Math.max(0, Math.min(1, (clientX - rect.left - axisL) / cw));
    const targetX = minX + ratio * (maxX - minX || 1);
    let best = 0;
    let dist = Infinity;
    base.forEach((p, i) => {
      const d = Math.abs(p.x - targetX);
      if (d < dist) { dist = d; best = i; }
    });
    hoverIndex = best;
    draw();
    if (!tip) {
      tip = document.createElement('div');
      tip.className = 'chart-tooltip';
      parent.appendChild(tip);
    }
    const time = new Date(base[best].x * 1000);
    const rows = series.map((s) => {
      const m = nearest(s.points, base[best].x);
      const val = m ? (options.tipFormat ? options.tipFormat(m.y, s) : fa(fmtNum.format(Math.round(m.y)))) : '—';
      return `<div class="tt-row"><span><i class="tt-dot" style="background:${s.color}"></i>${escapeHtml(s.name)}</span><b>${val}</b></div>`;
    }).join('');
    tip.innerHTML = `${rows}<div class="tt-time">${fmtDateTime.format(time)}</div>`;
    const x = (axisL + ratio * cw);
    tip.style.left = `${Math.max(55, Math.min(x, rect.width - 55))}px`;
    tip.style.top = `${rect.height * 0.55}px`;
    tip.classList.add('show');
  }

  canvas.addEventListener('mousemove', (e) => showTip(e.clientX));
  canvas.addEventListener('mouseleave', () => {
    hoverIndex = -1;
    draw();
    if (tip) tip.classList.remove('show');
  });
  canvas.addEventListener('touchstart', (e) => {
    if (e.touches[0]) showTip(e.touches[0].clientX);
  }, { passive: true });
  canvas.addEventListener('touchend', () => {
    hoverIndex = -1;
    draw();
    if (tip) tip.classList.remove('show');
  });

  watch(canvas, draw);
}

export function sparkline(canvas, values, color = '#38bdf8') {
  const data = (values || []).map(Number).filter((v) => Number.isFinite(v));
  watch(canvas, () => {
    const { ctx, w, h } = prep(canvas);
    if (!data.length) {
      ctx.strokeStyle = 'rgba(130,160,220,.16)';
      ctx.lineWidth = 1.4;
      ctx.beginPath();
      ctx.moveTo(4, h / 2);
      ctx.lineTo(w - 4, h / 2);
      ctx.stroke();
      return;
    }
    const max = Math.max(...data, 1);
    const min = Math.min(...data, 0);
    const span = max - min || 1;
    const pad = 3;
    const pts = data.map((v, i) => ({
      x: pad + (i / Math.max(1, data.length - 1)) * (w - pad * 2),
      y: h - pad - ((v - min) / span) * (h - pad * 2),
    }));
    const grad = ctx.createLinearGradient(0, 0, 0, h);
    grad.addColorStop(0, hexA(color, 0.3));
    grad.addColorStop(1, hexA(color, 0));
    ctx.beginPath();
    smoothPath(ctx, pts);
    ctx.lineTo(pts[pts.length - 1].x, h);
    ctx.lineTo(pts[0].x, h);
    ctx.closePath();
    ctx.fillStyle = grad;
    ctx.fill();
    ctx.beginPath();
    smoothPath(ctx, pts);
    ctx.strokeStyle = color;
    ctx.lineWidth = 1.7;
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    ctx.stroke();
    const last = pts[pts.length - 1];
    ctx.beginPath();
    ctx.arc(last.x, last.y, 2, 0, Math.PI * 2);
    ctx.fillStyle = color;
    ctx.shadowColor = color;
    ctx.shadowBlur = 6;
    ctx.fill();
    ctx.shadowBlur = 0;
  });
}

function hexA(hex, alpha) {
  const h = hex.replace('#', '');
  const v = h.length === 3 ? h.split('').map((c) => c + c).join('') : h;
  const r = parseInt(v.slice(0, 2), 16);
  const g = parseInt(v.slice(2, 4), 16);
  const b = parseInt(v.slice(4, 6), 16);
  return `rgba(${r},${g},${b},${alpha})`;
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

// Redraw charts after fonts load or orientation changes.
window.addEventListener('load', () => {
  document.querySelectorAll('canvas').forEach((c) => { if (c._pulseDraw) c._pulseDraw(); });
});

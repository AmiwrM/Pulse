/* =========================================================================
   Pulse — single page control panel
   ========================================================================= */
import { lineChart, sparkline } from './charts.js?v=24';
import { initI18n, getLang, setLang, translateDOM, translateText } from './i18n.js?v=24';

initI18n();

const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));

const state = {
  auth: null,
  view: 'dashboard',
  routeId: null,
  summary: null,
  monitors: [],
  incidents: [],
  rules: [],
  agents: [],
  agentsOfflineAfter: 120,
  geoCache: {},
  settingsData: null,
  settingsTab: 'general',
  cfStatus: null,
  cfAccounts: [],
  cfAccount: 0,
  cfZones: [],
  cfMonitors: [],
  cfLoading: false,
  cfZone: '',
  cfRecords: [],
  cfRecordType: '',
  cfSearch: '',
  cfError: '',
  cfZoneSeq: 0,
  pushConfig: null,
  notifications: [],
  audit: [],
  logs: [],
  logAutoRefresh: (() => {
    try { return localStorage.getItem('pulse-logs-auto') === '1'; } catch { return false; }
  })(),
  logCategory: 'all',
  logLevel: 'all',
  logQuery: '',
  logTimer: null,
  routeSub: null,
  pushSubscribed: null,
  filters: { status: 'all', search: '' },
  incidentFilter: 'all',
  agentHours: 24,
  sse: null,
  live: false,
  refreshTimer: null,
  pendingRefresh: false,
  modal: null,
  modalReturnFocus: null,
  confirm: null,
  clockTimer: null,
  userMenu: null,
  installPrompt: null,
  booted: false,
  viewEpoch: 0,
  detailEpoch: 0,
};

/* ------------------------------------------------------------------ format */
const EN = getLang() === 'en';
const LOCALE = EN ? 'en-US' : 'fa-IR';
// Numbers are always Latin, even with the Persian font. Dates stay Jalali in
// Persian, but use Latin digits too (fa-IR-u-nu-latn).
const NUM_LOCALE = 'en-US';
const DATE_LOCALE = EN ? 'en-US' : 'fa-IR-u-nu-latn';
const nf0 = new Intl.NumberFormat(NUM_LOCALE, { maximumFractionDigits: 0 });
const nf1 = new Intl.NumberFormat(NUM_LOCALE, { maximumFractionDigits: 1 });
const dtShort = new Intl.DateTimeFormat(DATE_LOCALE, { dateStyle: 'short' });
const dtFull = new Intl.DateTimeFormat(DATE_LOCALE, { dateStyle: 'medium', timeStyle: 'short' });
const clockFmt = new Intl.DateTimeFormat(DATE_LOCALE, { hour: '2-digit', minute: '2-digit', hour12: false });
const clockDate = new Intl.DateTimeFormat(DATE_LOCALE, { weekday: 'long', day: 'numeric', month: 'long' });

function num(v, digits = 0) {
  const n = Number(v);
  if (!Number.isFinite(n)) return '0';
  return digits === 1 ? nf1.format(n) : nf0.format(n);
}
function fa(value) {
  return String(value);
}
function esc(value) {
  return String(value ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}
function clamp(n, a, b) { return Math.max(a, Math.min(b, n)); }
function duration(sec) {
  sec = Math.max(0, Math.floor(Number(sec) || 0));
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  if (EN) {
    if (d > 0) return `${num(d)} day${d === 1 ? '' : 's'}${h ? ', ' + num(h) + 'h' : ''}`;
    if (h > 0) return `${num(h)} hour${h === 1 ? '' : 's'}${m ? ', ' + num(m) + 'm' : ''}`;
    if (m > 0) return `${num(m)} minute${m === 1 ? '' : 's'}${s && m < 3 ? ', ' + num(s) + 's' : ''}`;
    return `${num(s)} second${s === 1 ? '' : 's'}`;
  }
  if (d > 0) return `${num(d)} روز${h ? ' و ' + num(h) + ' ساعت' : ''}`;
  if (h > 0) return `${num(h)} ساعت${m ? ' و ' + num(m) + ' دقیقه' : ''}`;
  if (m > 0) return `${num(m)} دقیقه${s && m < 3 ? ' و ' + num(s) + ' ثانیه' : ''}`;
  return `${num(s)} ثانیه`;
}
function timeAgo(ts) {
  if (!ts) return EN ? 'never' : 'هرگز';
  const diff = Math.floor(Date.now() / 1000 - Number(ts));
  if (diff < 0) return EN ? 'just now' : 'همین حالا';
  if (diff < 10) return EN ? 'just now' : 'همین حالا';
  if (EN) {
    if (diff < 60) return `${num(diff)} seconds ago`;
    if (diff < 3600) return `${num(Math.floor(diff / 60))} minutes ago`;
    if (diff < 86400) return `${num(Math.floor(diff / 3600))} hours ago`;
    return `${num(Math.floor(diff / 86400))} days ago`;
  }
  if (diff < 60) return `${num(diff)} ثانیه پیش`;
  if (diff < 3600) return `${num(Math.floor(diff / 60))} دقیقه پیش`;
  if (diff < 86400) return `${num(Math.floor(diff / 3600))} ساعت پیش`;
  return `${num(Math.floor(diff / 86400))} روز پیش`;
}
function dateTime(ts) { return ts ? dtFull.format(new Date(ts * 1000)) : '—'; }
function dateShort(ts) { return ts ? dtShort.format(new Date(ts * 1000)) : '—'; }
function bytes(n) {
  n = Number(n) || 0;
  if (EN) {
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    if (n < 1024) return `${num(n)} B`;
    let i = -1;
    do { n /= 1024; i++; } while (n >= 1024 && i < units.length - 1);
    return `${nf1.format(n)} ${units[i]}`;
  }
  if (n < 1024) return `${num(n)} بایت`;
  const units = ['کیلوبایت', 'مگابایت', 'گیگابایت', 'ترابایت'];
  let i = -1;
  do { n /= 1024; i++; } while (n >= 1024 && i < units.length - 1);
  return `${nf1.format(n)} ${units[i]}`;
}
function uptimePct(v) { return `${nf1.format(Number(v) || 0)}${EN ? '%' : '٪'}`; }
function daysLeft(ts) {
  if (!ts) return null;
  return Math.floor((Number(ts) * 1000 - Date.now()) / 86400000);
}
function sslBadge(m) {
  const d = daysLeft(m.ssl_expires_at);
  if (d === null) return '';
  if (d < 0) return `<span class="chip red" title="گواهی منقضی شده">${icon('shield')} گواهی منقضی</span>`;
  if (d <= 14) return `<span class="chip amber" title="${dateShort(m.ssl_expires_at)}">${icon('shield')} ${num(d)} روز تا انقضا</span>`;
  return '';
}
function typeMeta(type) {
  return TYPES[type] || { label: type || 'نامشخص', short: type || '?', icon: 'globe', color: '#38bdf8' };
}
function statusLabel(status) {
  if (EN) return { up: 'Healthy', down: 'Down', paused: 'Paused', unknown: 'Pending', degraded: 'Warning' }[status] || 'Unknown';
  return { up: 'سالم', down: 'قطع', paused: 'متوقف', unknown: 'در انتظار', degraded: 'هشدار' }[status] || 'نامشخص';
}
function sparkColor(status) {
  return { up: '#34d399', down: '#fb7185', unknown: '#fbbf24', paused: '#64748b', degraded: '#fb923c' }[status] || '#38bdf8';
}
function userNameInitial() {
  const u = state.auth?.username || 'admin';
  return u.slice(0, 1).toUpperCase();
}

/* ------------------------------------------------------------------ icons */
const ICONS = {
  dashboard: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
  activity: '<path d="M3 12h4l2.2-6 4 12 2.6-6H21"/>',
  alert: '<path d="M12 3 2.8 20h18.4L12 3Z"/><path d="M12 9v5M12 17.5v.5"/>',
  server: '<rect x="3" y="4" width="18" height="6" rx="2"/><rect x="3" y="14" width="18" height="6" rx="2"/><path d="M7 7h.01M7 17h.01"/>',
  sliders: '<path d="M4 7h10M18 7h2M4 17h2M10 17h10"/><circle cx="16" cy="7" r="2"/><circle cx="8" cy="17" r="2"/>',
  cloud: '<path d="M7 18a4.5 4.5 0 0 1-.4-9A6 6 0 0 1 18 10.5a3.5 3.5 0 0 1-.5 7H7Z"/><path d="M9 14h6"/>',
  bell: '<path d="M18 9a6 6 0 1 0-12 0c0 6-2 7-2 7h16s-2-1-2-7"/><path d="M10 20a2 2 0 0 0 4 0"/>',
  settings: '<circle cx="12" cy="12" r="3"/><path d="M19 12a7 7 0 0 0-.1-1.2l2-1.5-2-3.4-2.3 1a7 7 0 0 0-2-1.2L14.2 3H9.8l-.4 2.6a7 7 0 0 0-2 1.2l-2.3-1-2 3.4 2 1.5a7 7 0 0 0 0 2.5l-2 1.5 2 3.4 2.3-1a7 7 0 0 0 2 1.2l.4 2.6h4.4l.4-2.6a7 7 0 0 0 2-1.2l2.3 1 2-3.4-2-1.5c.1-.4.1-.8.1-1.2Z"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  refresh: '<path d="M20 11a8 8 0 1 0-2.3 6.3M20 4v7h-7"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
  check: '<path d="m5 12 4.5 4.5L19 7"/>',
  pause: '<path d="M9 5v14M15 5v14"/>',
  play: '<path d="m8 5 11 7-11 7V5Z"/>',
  trash: '<path d="M4 7h16M9 7V4h6v3M6 7l1 14h10l1-14M10 11v6M14 11v6"/>',
  edit: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L8 18l-4 1 1-4L16.5 3.5Z"/>',
  close: '<path d="M6 6l12 12M18 6 6 18"/>',
  chevron: '<path d="m9 6 6 6-6 6"/>',
  chevronDown: '<path d="m6 9 6 6 6-6"/>',
  copy: '<rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/>',
  download: '<path d="M12 3v12M7 10l5 5 5-5M4 21h16"/>',
  logout: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="m16 17 5-5-5-5M21 12H9"/>',
  user: '<circle cx="12" cy="8" r="4"/><path d="M4 21c0-4 3.6-6 8-6s8 2 8 6"/>',
  cpu: '<rect x="7" y="7" width="10" height="10" rx="2"/><path d="M10 1v3M14 1v3M10 20v3M14 20v3M1 10h3M1 14h3M20 10h3M20 14h3"/>',
  ram: '<rect x="3" y="8" width="18" height="8" rx="2"/><path d="M7 8v8M11 8v8M15 8v8M19 8v8M6 4v4M18 4v4M6 16v4M18 16v4"/>',
  disk: '<ellipse cx="12" cy="6" rx="8" ry="3"/><path d="M4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6"/><path d="M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"/>',
  globe: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c3 3.5 3 14 0 18M12 3c-3 3.5-3 14 0 18"/>',
  plug: '<path d="M9 3v6M15 3v6M6 9h12v3a6 6 0 0 1-12 0V9ZM12 18v3"/>',
  radio: '<circle cx="12" cy="12" r="2"/><path d="M7.8 16.2a6 6 0 0 1 0-8.4M16.2 7.8a6 6 0 0 1 0 8.4M4.9 19.1a10 10 0 0 1 0-14.2M19.1 4.9a10 10 0 0 1 0 14.2"/>',
  network: '<rect x="3" y="3" width="6" height="6" rx="1"/><rect x="15" y="15" width="6" height="6" rx="1"/><path d="M9 6h6a3 3 0 0 1 3 3v6M15 18H9a3 3 0 0 1-3-3V9"/>',
  shield: '<path d="M12 3 5 6v5c0 4.5 3 7.8 7 10 4-2.2 7-5.5 7-10V6l-7-3Z"/><path d="m9 12 2 2 4-4"/>',
  layers: '<path d="m12 3 9 5-9 5-9-5 9-5Z"/><path d="m3 12 9 5 9-5M3 16l9 5 9-5"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  eye: '<path d="M2 12s3.5-6 10-6 10 6 10 6-3.5 6-10 6S2 12 2 12Z"/><circle cx="12" cy="12" r="2.5"/>',
  send: '<path d="m22 2-7 20-4-9-9-4 20-7Z"/><path d="M22 2 11 13"/>',
  smartphone: '<rect x="6" y="2" width="12" height="20" rx="3"/><path d="M10 18h4"/>',
  webhook: '<path d="M18 16a3 3 0 1 0-2.4-1.2l-2.7 4.5A3 3 0 1 1 7 16.8"/><path d="M9 7.5A3 3 0 1 1 14.4 6l-2.7 4.6"/><path d="M6 16h6M12 8l3 5"/>',
  link: '<path d="M10 13a5 5 0 0 0 7 0l2-2a5 5 0 0 0-7-7l-1 1"/><path d="M14 11a5 5 0 0 0-7 0l-2 2a5 5 0 0 0 7 7l1-1"/>',
  key: '<circle cx="8" cy="15" r="4"/><path d="m11 12 9-9M17 6l3 3M14 9l3 3"/>',
  database: '<ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v14c0 1.7 3.6 3 8 3s8-1.3 8-3V5"/><path d="M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"/>',
  chart: '<path d="M4 20V4M4 20h16"/><path d="m7 15 4-5 3 2 5-7"/>',
  wifi: '<path d="M2 8a16 16 0 0 1 20 0M5 12a11 11 0 0 1 14 0M8.5 15.5a6 6 0 0 1 7 0"/><circle cx="12" cy="19" r="1"/>',
  bolt: '<path d="M13 2 4 14h6l-1 8 9-12h-6l1-8Z"/>',
  target: '<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="5"/><circle cx="12" cy="12" r="1"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8v.5"/>',
  error: '<circle cx="12" cy="12" r="9"/><path d="M12 8v5M12 16v.5"/>',
  success: '<circle cx="12" cy="12" r="9"/><path d="m8 12 2.5 2.5L16 9"/>',
  arrow: '<path d="M5 12h14M13 6l6 6-6 6"/>',
  external: '<path d="M14 4h6v6M20 4 10 14"/><path d="M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5"/>',
  menu: '<path d="M4 7h16M4 12h16M4 17h16"/>',
  telegram: '<path d="m21 4-3 16-6-4-3 3v-5l9-7-11 6-3-1 17-8Z"/>',
  fire: '<path d="M12 22c4 0 7-2.8 7-7 0-3-1.5-5.5-4-8 .2 2.5-1 4-2 4-1.2 0-2-2-2-4.5C8 9 5 12 5 15c0 4.2 3 7 7 7Z"/>',
  terminal: '<rect x="3" y="4" width="18" height="16" rx="3"/><path d="m7 9 3 3-3 3M13 15h4"/>',
  signal: '<path d="M4 20v-4M9 20v-8M14 20V8M19 20V4"/>',
  agent: '<rect x="3" y="4" width="18" height="7" rx="2"/><path d="M7 7.5h.01M11 7.5h.01"/><path d="M4 16h4l2-3 3 6 2-3h5"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
  moon: '<path d="M20.5 14.5A8.5 8.5 0 0 1 9.5 3.5 8.5 8.5 0 1 0 20.5 14.5Z"/>',
};
function icon(name, cls = '') {
  return `<svg class="ico ${cls}" viewBox="0 0 24 24" aria-hidden="true">${ICONS[name] || ICONS.info}</svg>`;
}

const THEME_KEY = 'pulse-theme';
function applyTheme(theme, persist = true) {
  theme = theme === 'light' ? 'light' : 'dark';
  document.documentElement.dataset.theme = theme;
  const metaTheme = document.querySelector('meta[name="theme-color"]');
  if (metaTheme) metaTheme.setAttribute('content', theme === 'light' ? '#f4f8ff' : '#060a14');
  if (persist) {
    try { localStorage.setItem(THEME_KEY, theme); } catch { /* storage may be disabled */ }
  }
  const btn = document.getElementById('theme-toggle');
  if (btn) {
    const light = theme === 'light';
    btn.innerHTML = icon(light ? 'moon' : 'sun');
    btn.title = light ? 'حالت تیره' : 'حالت روشن';
    btn.setAttribute('aria-label', light ? 'تغییر به حالت تیره' : 'تغییر به حالت روشن');
    btn.setAttribute('aria-pressed', light ? 'true' : 'false');
  }
  document.querySelectorAll('canvas').forEach((canvas) => {
    if (typeof canvas._pulseDraw === 'function') canvas._pulseDraw();
  });
}
function initTheme() {
  let saved = '';
  try { saved = localStorage.getItem(THEME_KEY) || ''; } catch { /* ignore */ }
  applyTheme(saved || 'dark', false);
}
initTheme();

const TYPES = {
  http: { label: 'وب‌سایت / HTTP', short: 'HTTP', icon: 'globe', color: '#38bdf8' },
  tcp: { label: 'پورت TCP', short: 'TCP', icon: 'plug', color: '#2dd4bf' },
  ping: { label: 'پینگ ICMP', short: 'PING', icon: 'radio', color: '#8b5cf6' },
  dns: { label: 'رکورد DNS', short: 'DNS', icon: 'network', color: '#fbbf24' },
  ssl: { label: 'گواهی SSL', short: 'SSL', icon: 'shield', color: '#34d399' },
  proxy: { label: 'کانفیگ پروکسی', short: 'PROXY', icon: 'layers', color: '#f472b6' },
  vps: { label: 'سرور مجازی / Virtualizor', short: 'VPS', icon: 'server', color: '#f97316' },
};

function cleanRuleName(name) {
  return String(name || '').replace(/^[^\p{L}\p{N}]+/u, '').trim() || String(name || '');
}

const TRIGGERS = {
  down: { label: 'قطع سرویس', icon: 'alert', tone: 'down', desc: 'وقتی یک مانیتور پس از رسیدن به آستانهٔ خطا قطع می‌شود.' },
  up: { label: 'بازیابی سرویس', icon: 'check', tone: 'up', desc: 'وقتی سرویس قطع‌شده دوباره سالم می‌شود.' },
  degraded: { label: 'هشدار سرویس', icon: 'alert', tone: 'warn', desc: 'وقتی سرویس سالم است اما بررسی با هشدار یا وضعیت degraded برمی‌گردد.' },
  cert_expiring: { label: 'انقضای گواهی SSL', icon: 'shield', tone: 'cert', desc: 'وقتی عمر گواهی از آستانهٔ تعیین‌شده کمتر شود.' },
  cpu_high: { label: 'مصرف بالای CPU', icon: 'cpu', tone: 'resource', desc: 'وقتی مصرف پردازندهٔ سرور از آستانه عبور کند.' },
  ram_high: { label: 'مصرف بالای RAM', icon: 'ram', tone: 'resource', desc: 'وقتی مصرف حافظهٔ سرور از آستانه عبور کند.' },
  disk_high: { label: 'پر شدن دیسک', icon: 'disk', tone: 'resource', desc: 'وقتی فضای دیسک سرور از آستانه عبور کند.' },
  agent_offline: { label: 'قطع ارتباط ایجنت', icon: 'wifi', tone: 'agent', desc: 'وقتی گزارش‌های یک سرور برای مدت مشخصی نرسد.' },
  dns_mismatch: { label: 'اختلاف رکورد DNS', icon: 'network', tone: 'cert', desc: 'وقتی مقدار واقعی DNS با مقدار مورد انتظار تفاوت داشته باشد.' },
};

const DEFAULT_NOTIFY_TITLE = '{emoji} {title}';
const DEFAULT_NOTIFY_BODY = `{emoji} <b>{title}</b>

🖥 <b>{monitor}</b>
🧠 دلیل: {reason}
📋 جزئیات: {details}
🕒 {time}`;
const NOTIFY_PLACEHOLDERS = ['{emoji}', '{title}', '{event}', '{monitor}', '{agent}', '{hostname}', '{type}', '{target}', '{status}', '{reason}', '{details}', '{failed_configs}', '{message}', '{value}', '{time}'];

/* ------------------------------------------------------------------ API */
class ApiError extends Error {
  constructor(message, status) { super(message); this.status = status; }
}

async function api(path, options = {}) {
  const opts = { method: options.method || 'GET', credentials: 'same-origin', headers: { Accept: 'application/json' } };
  if (options.body !== undefined) {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(options.body);
  }
  const timeout = options.timeout === undefined ? 30000 : Number(options.timeout);
  let timer = null;
  if (timeout > 0) {
    const controller = new AbortController();
    opts.signal = controller.signal;
    timer = setTimeout(() => controller.abort(), timeout);
  }
  let res;
  try {
    res = await fetch(path, opts);
  } catch (e) {
    if (e?.name === 'AbortError') throw new ApiError('پاسخ سرور بیش از حد طول کشید؛ دوباره تلاش کنید.', 0);
    throw new ApiError('ارتباط با سرور برقرار نشد. اتصال شبکه را بررسی کنید.', 0);
  } finally {
    if (timer) clearTimeout(timer);
  }
  const text = await res.text();
  let json = null;
  try { json = text ? JSON.parse(text) : null; } catch { /* non-json */ }
  const publicAuth = ['/api/auth/login', '/api/auth/setup', '/api/auth/state'];
  if (res.status === 401 && !publicAuth.includes(path)) {
    sessionExpired();
    throw new ApiError('نشست شما منقضی شده است', 401);
  }
  if (!res.ok || (json && json.ok === false)) {
    throw new ApiError(json?.error || `خطا (${res.status})`, res.status);
  }
  return json ? json.data : null;
}

/* ------------------------------------------------------------------ toast */
function toast(message, type = 'info', title = '') {
  const root = $('#toast-root');
  if (!root) return;
  const titles = { success: 'انجام شد', error: 'خطا', warn: 'توجه', info: 'پالس' };
  const ico = type === 'success' ? 'success' : type === 'error' ? 'error' : type === 'warn' ? 'alert' : 'info';
  const el = document.createElement('div');
  el.className = `toast ${type}`;
  el.innerHTML = `<div class="toast-ico">${icon(ico)}</div><div class="toast-body"><div class="toast-title">${esc(title || titles[type] || 'پالس')}</div><div class="toast-msg">${esc(message)}</div></div><button class="toast-close" type="button" aria-label="بستن">×</button>`;
  root.appendChild(el);
  if (EN) translateDOM(el);
  const remove = () => { el.classList.add('out'); setTimeout(() => el.remove(), 260); };
  el.querySelector('.toast-close').addEventListener('click', remove);
  setTimeout(remove, type === 'error' ? 7000 : 4500);
}

/* ------------------------------------------------------------------ modal */
function openModal({ title, subtitle = '', body = '', footer = '', size = '', onClose = null }) {
  closeModal(true);
  state.modalReturnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  const root = $('#modal-root');
  root.innerHTML = `<div class="modal-backdrop"><div class="modal ${size}" role="dialog" aria-modal="true" aria-label="${esc(title)}">
    <div class="sheet-handle" aria-hidden="true"></div>
    <div class="modal-head">
      <div class="modal-head-copy"><h3>${esc(title)}</h3>${subtitle ? `<p>${esc(subtitle)}</p>` : ''}</div>
      <button class="modal-close" type="button" data-action="close-modal" aria-label="Close">${icon('close')}</button>
    </div>
    <div class="modal-body">${body}</div>
    ${footer ? `<div class="modal-foot">${footer}</div>` : ''}
  </div></div>`;
  if (EN) translateDOM(root);
  state.modal = { onClose, dirty: false, confirming: false };
  const backdrop = $('.modal-backdrop', root);
  // Never close from a backdrop click. A small mouse movement outside a dialog
  // (or a touch that ends on the backdrop) used to discard half-filled forms.
  backdrop.addEventListener('click', (e) => {
    if (e.target === backdrop) e.stopPropagation();
  });
  const markDirty = () => { if (state.modal) state.modal.dirty = true; };
  root.addEventListener('input', markDirty);
  root.addEventListener('change', markDirty);
  document.body.classList.add('modal-open');
  wireLabels(root);
  const mobile = window.matchMedia('(max-width: 700px)').matches;
  const target = mobile
    ? root.querySelector('.modal-close')
    : (root.querySelector('.modal-body input:not([type=hidden]), .modal-body select, .modal-body textarea, .modal-body button') || root.querySelector('.modal-close'));
  requestAnimationFrame(() => { if (target?.isConnected) target.focus({ preventScroll: true }); });
}
function openDetailSkeleton(title, subtitle = '') {
  let closed = false;
  openModal({
    title,
    subtitle,
    size: 'lg',
    body: `<div class="loading-grid">${Array.from({ length: 3 }).map(() => '<div class="panel loading-card skeleton"></div>').join('')}</div>`,
    onClose: () => { closed = true; },
  });
  return { isClosed: () => closed };
}

function closeModal(silent = false) {
  const root = $('#modal-root');
  if (!root) return;
  const current = state.modal;
  if (!silent && current?.dirty) {
    if (current.confirming) return;
    current.confirming = true;
    confirmAction({
      title: 'Discard changes?',
      message: 'You have unsaved changes in this dialog. Closing it will discard them.',
      confirmText: 'Discard',
      danger: true,
    }).then((ok) => {
      if (state.modal) state.modal.confirming = false;
      if (ok && state.modal) {
        state.modal.dirty = false;
        closeModal();
      }
    });
    return;
  }
  const returnFocus = state.modalReturnFocus;
  root.innerHTML = '';
  document.body.classList.remove('modal-open');
  const cb = state.modal?.onClose;
  state.modal = null;
  state.modalReturnFocus = null;
  if (!silent && typeof cb === 'function') cb();
  if (!silent && returnFocus?.isConnected) {
    try { returnFocus.focus({ preventScroll: true }); } catch { /* ignore */ }
  }
  if (!silent && state.pendingRefresh) {
    state.pendingRefresh = false;
    if (state.auth?.authenticated && LIVE_VIEWS.has(state.view)) refreshCurrent(true);
  }
}

function confirmAction({ title = 'تأیید', message, confirmText = 'تأیید', danger = false }) {
  closeConfirm();
  return new Promise((resolve) => {
    const el = document.createElement('div');
    el.className = 'modal-backdrop';
    el.style.zIndex = '95';
    el.innerHTML = `<div class="modal sm" role="alertdialog" aria-modal="true" aria-label="${esc(title)}">
      <div class="modal-head"><div class="modal-head-copy"><h3>${esc(title)}</h3></div></div>
      <div class="modal-body"><p style="font-size:12px;color:var(--text-soft);line-height:2">${esc(message)}</p></div>
      <div class="modal-foot"><button class="btn btn-ghost" data-x="no" type="button">انصراف</button><button class="btn ${danger ? 'btn-danger' : 'btn-primary'}" data-x="yes" type="button">${esc(confirmText)}</button></div>
    </div>`;
    document.body.appendChild(el);
    let settled = false;
    const done = (value) => {
      if (settled) return;
      settled = true;
      if (state.confirm?.el === el) state.confirm = null;
      el.remove();
      resolve(value);
    };
    state.confirm = { el, finish: done };
    el.addEventListener('click', (e) => {
      if (e.target === el) done(false);
      if (e.target.closest('[data-x="no"]')) done(false);
      if (e.target.closest('[data-x="yes"]')) done(true);
    });
    el.querySelector('[data-x="no"]').focus();
  });
}

function closeConfirm() {
  const c = state.confirm;
  if (!c) return;
  c.finish(false);
}

/* ------------------------------------------------------------------ session / auth */
async function init() {
  startClock();
  bindGlobalEvents();
  registerServiceWorker();
  let auth = null;
  let bootError = '';
  try {
    auth = await api('/api/auth/state');
  } catch (e) {
    bootError = e?.message || 'ارتباط با سرور برقرار نشد.';
    auth = { needs_setup: false, authenticated: false, version: '', site_title: 'پالس' };
  }
  state.auth = auth;
  setTimeout(() => $('#boot')?.classList.add('done'), 300);
  if (auth.needs_setup) {
    renderAuth('setup');
  } else if (!auth.authenticated) {
    renderAuth('login', bootError);
  } else {
    startApp();
  }
}

function startApp() {
  state.booted = true;
  $('#auth-root').classList.add('hidden');
  $('#app').classList.remove('hidden');
  $('#side-version').textContent = fa(state.auth?.version || '');
  applySidebarCollapse(document.documentElement.classList.contains('sidebar-collapsed'));
  $('#avatar-btn').textContent = userNameInitial();
  document.title = EN
    ? `${translateText(state.auth?.site_title || 'Pulse')} | Command center`
    : `${state.auth?.site_title || 'پالس'} | مرکز فرمان`;
  connectSSE();
  if (!location.hash || location.hash === '#' || location.hash === '#/') {
    history.replaceState(null, '', '#/dashboard');
  }
  route();
  warmSummary();
}

function warmSummary() {
  api('/api/summary').then((summary) => {
    if (!summary || !state.auth?.authenticated) return;
    state.summary = summary;
    if (state.view === 'monitors' && !state.monitors.length) state.monitors = summary.monitors || [];
    updateBadges();
  }).catch(() => { /* the visible page already shows its own error */ });
}

function renderAuth(mode, message = '') {
  if (state.sse) { state.sse.close(); state.sse = null; }
  if (typeof stopLogsPoll === 'function') stopLogsPoll();
  setLive(false);
  $('#app').classList.add('hidden');
  const root = $('#auth-root');
  root.classList.remove('hidden');
  document.title = EN ? 'Pulse | Sign in to the command center' : 'پالس | ورود به مرکز فرمان';
  const isSetup = mode === 'setup';
  const err = message || '';
  root.innerHTML = `<div class="auth-shell">
    <div class="auth-visual">
      <div class="auth-brand">
        <span class="brand-mark"><span class="ring r1"></span><span class="ring r2"></span><svg viewBox="0 0 48 48"><defs><linearGradient id="lgAuth" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#38bdf8"/><stop offset="1" stop-color="#8b5cf6"/></linearGradient></defs><rect x="5" y="5" width="38" height="38" rx="13" fill="url(#lgAuth)"/><path d="M10 25h6l3.2-8 5.4 16 3.4-9h8" fill="none" stroke="#fff" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"/></svg></span>
        <div><b>پالس</b><span>PULSE MONITOR</span></div>
      </div>
      <div class="auth-pitch">
        <h1>آرامش خاطر،<br><em>با هر ضربان.</em></h1>
        <p>پایش لحظه‌ای سرورها، دامنه‌ها و کانفیگ‌های پروکسی؛ با اعلان‌های هوشمند و ترمیم خودکار DNS.</p>
        <div class="auth-points">
          <div class="auth-point"><span class="pico">${icon('activity')}</span> بررسی مداوم HTTP، TCP، ICMP، DNS و SSL</div>
          <div class="auth-point"><span class="pico">${icon('bell')}</span> اعلان لحظه‌ای در تلگرام و مرورگر</div>
          <div class="auth-point"><span class="pico">${icon('cloud')}</span> سوییچ خودکار رکورد DNS با Cloudflare</div>
        </div>
      </div>
      <div class="auth-radar"></div>
    </div>
    <div class="auth-form-side">
      <span class="eyebrow">${icon(isSetup ? 'bolt' : 'shield')} ${isSetup ? 'راه‌اندازی اولیه' : 'ورود امن'}</span>
      <h2>${isSetup ? 'حساب مدیریت را بسازید' : 'به مرکز فرمان خوش آمدید'}</h2>
      <p>${isSetup ? 'برای شروع، یک حساب مدیر با رمز عبور قوی بسازید.' : 'برای مشاهده وضعیت زیرساخت وارد شوید.'}</p>
      ${isSetup ? `<div class="setup-note">${icon('shield')} این حساب فقط روی سرور شما ذخیره می‌شود. یک رمز عبور قوی انتخاب کنید و دسترسی پنل را با HTTPS محدود کنید.</div>` : ''}
      <form class="auth-form" data-form="${isSetup ? 'setup' : 'login'}" novalidate>
        <div class="form-error ${err ? 'show' : ''}" data-auth-error>${esc(err)}</div>
        <div class="field"><label for="auth-user">نام کاربری</label><input class="input" style="direction:ltr;text-align:left" id="auth-user" name="username" autocomplete="username" value="${isSetup ? 'admin' : ''}" placeholder="admin" required></div>
        <div class="field"><label for="auth-pass">رمز عبور</label>
          <div class="password-wrap"><input class="input" style="direction:ltr;text-align:left" id="auth-pass" name="password" type="password" autocomplete="${isSetup ? 'new-password' : 'current-password'}" placeholder="••••••••" required><button class="password-toggle" type="button" data-action="toggle-password" aria-label="نمایش یا مخفی کردن رمز">${icon('eye')}</button></div>
          ${isSetup ? '<div class="auth-strength" id="auth-strength"><span></span><span></span><span></span><span></span></div>' : ''}
        </div>
        ${isSetup ? `<div class="field"><label for="auth-pass2">تکرار رمز عبور</label><input class="input" style="direction:ltr;text-align:left" id="auth-pass2" name="confirm" type="password" autocomplete="new-password" placeholder="••••••••" required></div>` : ''}
        <button class="btn btn-primary" type="submit">${isSetup ? 'ساخت حساب و شروع' : 'ورود به پالس'} ${icon('arrow')}</button>
      </form>
      <div class="auth-foot"><span>پالس نسخه ${fa(state.auth?.version || '1.6.0')}</span><span>${icon('shield')} اتصال امن</span></div>
    </div>
  </div>`;
  if (isSetup) {
    $('#auth-pass')?.addEventListener('input', updatePasswordStrength);
    updatePasswordStrength();
  }
}

function updatePasswordStrength() {
  const input = $('#auth-pass');
  const box = $('#auth-strength');
  if (!input || !box) return;
  const v = input.value;
  let score = 0;
  if (v.length >= 8) score++;
  if (v.length >= 12) score++;
  if (/[A-Z]/.test(v) && /[a-z]/.test(v)) score++;
  if (/\d/.test(v) && /[^A-Za-z0-9]/.test(v)) score++;
  $$('span', box).forEach((s, i) => s.classList.toggle('on', i < score));
}

function sessionExpired() {
  if (!state.auth?.authenticated) return;
  state.auth = { ...state.auth, authenticated: false };
  if (state.sse) { state.sse.close(); state.sse = null; }
  setLive(false);
  closeConfirm();
  closeModal(true);
  closeUserMenu();
  renderAuth('login', 'نشست شما منقضی شده است؛ دوباره وارد شوید.');
}

async function logout() {
  try { await api('/api/auth/logout', { method: 'POST', body: {} }); } catch { /* ignore */ }
  state.auth = { ...(state.auth || {}), authenticated: false };
  if (state.sse) { state.sse.close(); state.sse = null; }
  setLive(false);
  closeUserMenu();
  closeConfirm();
  closeModal(true);
  renderAuth('login');
}

async function submitAuth(form) {
  const mode = form.dataset.form;
  const err = $('[data-auth-error]', form);
  const btn = form.querySelector('button[type=submit]');
  const old = btn?.innerHTML;
  const username = form.elements.namedItem('username')?.value?.trim();
  const password = form.elements.namedItem('password')?.value || '';
  const confirm = form.elements.namedItem('confirm')?.value;
  err?.classList.remove('show');
  if (!username || !password) { if (err) { err.textContent = 'نام کاربری و رمز عبور الزامی است'; err.classList.add('show'); } return; }
  if (mode === 'setup' && password !== confirm) { if (err) { err.textContent = 'تکرار رمز عبور مطابقت ندارد'; err.classList.add('show'); } return; }
  if (mode === 'setup' && password.length < 8) { if (err) { err.textContent = 'رمز عبور باید حداقل 8 کاراکتر باشد'; err.classList.add('show'); } return; }
  if (btn) { btn.disabled = true; btn.innerHTML = 'در حال بررسی…'; }
  try {
    if (mode === 'setup') await api('/api/auth/setup', { method: 'POST', body: { username, password } });
    else await api('/api/auth/login', { method: 'POST', body: { username, password } });
    const st = await api('/api/auth/state');
    state.auth = st;
    if (st.authenticated) startApp();
    else throw new ApiError('ورود ناموفق بود', 401);
  } catch (e) {
    if (err) { err.textContent = e.message; err.classList.add('show'); }
    else toast(e.message, 'error');
    if (btn) { btn.disabled = false; btn.innerHTML = old; }
    return;
  }
  if (btn) { btn.disabled = false; btn.innerHTML = old; }
}

/* ------------------------------------------------------------------ clock / live */
function startClock() {
  const update = () => {
    const now = new Date();
    const t = $('#clock .clock-time');
    const d = $('#clock .clock-date');
    if (t) t.textContent = clockFmt.format(now);
    if (d) d.textContent = clockDate.format(now);
  };
  update();
  clearInterval(state.clockTimer);
  state.clockTimer = setInterval(update, 15000);
}
function setLive(on) {
  state.live = on;
  const pill = $('#live-pill');
  if (pill) {
    pill.classList.toggle('offline', !on);
    pill.querySelector('span:last-child').textContent = on ? 'زنده' : 'قطع ارتباط';
  }
  const health = $('#side-health');
  if (health) {
    health.classList.toggle('bad', !on);
    health.querySelector('.health-text').textContent = on ? 'اتصال زنده برقرار است' : 'ارتباط با سرور قطع است';
  }
  const fs = $('#footer-status');
  if (fs) {
    fs.classList.toggle('bad', !on);
    fs.innerHTML = `<span class="dot"></span> ${on ? 'متصل' : 'قطع'}`;
  }
  const brandLive = $('#brand-live');
  if (brandLive) brandLive.classList.toggle('off', !on);
}

/* ------------------------------------------------------------------ SSE */
function connectSSE() {
  if (state.sse) { state.sse.close(); state.sse = null; }
  const es = new EventSource('/api/stream');
  state.sse = es;
  let reconnecting = false;
  es.onopen = () => {
    setLive(true);
    if (reconnecting) {
      reconnecting = false;
      if (state.auth?.authenticated) {
        if (LIVE_VIEWS.has(state.view)) refreshCurrent(true);
        else warmSummary();
      }
    }
  };
  es.onerror = () => {
    reconnecting = true;
    setLive(false);
    if (state.sse === es && !state.auth?.authenticated) es.close();
    if (state.auth?.authenticated) {
      api('/api/auth/state').then((st) => { if (!st.authenticated) sessionExpired(); }).catch(() => {});
    }
  };
  es.addEventListener('ready', () => setLive(true));
  es.onmessage = (event) => {
    let msg = null;
    try { msg = JSON.parse(event.data); } catch { return; }
    handleSSE(msg);
  };
}
function agentSummaryHTML(agents) {
  const online = agents.filter(statusFromAgent).length;
  return `<span><b>${num(agents.length)}</b> سرور ثبت شده</span><span class="badge-dot ${online ? '' : 'bad'}"><span class="dot"></span>${num(online)} آنلاین</span>${agents.length - online ? `<span class="badge-dot bad"><span class="dot"></span>${num(agents.length - online)} آفلاین</span>` : ''}`;
}

function updateAgentSummary() {
  const el = document.querySelector('[data-agent-summary]');
  if (el) el.innerHTML = agentSummaryHTML(state.agents);
  const agents = state.agents || [];
  const online = agents.filter(statusFromAgent).length;
  const value = document.querySelector('[data-dashboard-agents-value]');
  if (value) value.innerHTML = `${num(online)}<small>از ${num(agents.length)}</small>`;
  const foot = document.querySelector('[data-dashboard-agents-foot]');
  if (foot) foot.textContent = agents.length ? (online === agents.length ? 'همه ایجنت‌ها متصل' : 'برخی ایجنت‌ها قطع هستند') : 'هنوز ایجنتی اضافه نشده';
  const chip = document.querySelector('[data-dashboard-agent-chip]');
  if (chip) chip.innerHTML = `${icon('server')} ${num(online)} سرور آنلاین`;
  // If the dashboard was rendered while no agent existed, the server-health
  // panel still shows its empty state. Refresh it once agents appear (or
  // disappear) so the panel and the summary chip never disagree.
  if (state.view === 'dashboard' && document.querySelector('.mini-agent-list')
    && Math.min(agents.length, 5) !== document.querySelectorAll('[data-mini-agent]').length) {
    scheduleRefresh();
  }
}

function updateMiniAgent(a) {
  const row = document.querySelector(`[data-mini-agent="${Number(a.id)}"]`);
  if (!row) return false;
  const q = agentQuality(a);
  const meta = row.querySelector('[data-mini-meta]');
  if (meta) {
    meta.title = 'IP عمومی خروجی؛ ممکن است با IP اصلی سرور متفاوت باشد.';
    meta.innerHTML = `${q.online ? 'آنلاین' : 'آفلاین'} · ${esc(a.ip || 'بدون IP')}<span data-agent-location data-ip="${esc(a.ip || '')}"></span>`;
    hydrateAgentLocations(meta);
  }
  const cpu = row.querySelector('[data-mini-cpu]');
  if (cpu) cpu.style.width = `${clamp(a.cpu_pct || 0, 0, 100)}%`;
  const ram = row.querySelector('[data-mini-ram]');
  if (ram) ram.style.width = `${clamp(a.ram_pct || 0, 0, 100)}%`;
  const online = row.querySelector('[data-mini-online]');
  if (online) online.classList.toggle('off', !q.online);
  return true;
}

function handleSSE(msg) {
  const type = msg?.type;
  if (!type) return;
  if (type === 'ready') { setLive(true); return; }
  if (type === 'cf.action') {
    const d = msg.data || {};
    toast(d.message || 'تغییر رکورد DNS انجام شد', d.ok ? 'success' : 'error', d.ok ? 'کلادفلر' : 'خطای کلادفلر');
  }
  if (type === 'toast') {
    toast(msg.data?.message || '', msg.data?.level || 'info');
  }
  if (type === 'agent.link') {
    const d = msg.data || {};
    const a = state.agents.find((item) => Number(item.id) === Number(d.agent_id));
    if (!a) {
      if (state.view === 'servers' || state.view === 'dashboard') scheduleRefresh();
      return;
    }
    a.connected = !!d.connected;
    a.transport = d.transport || '';
    if (d.protocol) a.protocol = d.protocol;
    if (typeof d.can_check === 'boolean') a.can_check = d.can_check;
    if (typeof d.xray_available === 'boolean') a.xray_available = d.xray_available;
    if (d.xray_version !== undefined) a.xray_version = d.xray_version || '';
    if (d.last_seen) a.last_seen = d.last_seen;
    if (d.version) a.version = d.version;
    if (d.hostname) a.hostname = d.hostname;
    if (d.ip) a.ip = d.ip;
    if (d.peer_ip) a.peer_ip = d.peer_ip;
    if (state.summary?.stats) state.summary.stats.agents_online = state.agents.filter(statusFromAgent).length;
    const cardPatched = updateAgentCard(a);
    updateMiniAgent(a);
    updateAgentSummary();
    if (state.modal?.kind === 'agent' && Number(state.modal?.agentId) === Number(a.id)) updateAgentDetailLive(a);
    if (!cardPatched && (state.view === 'servers' || state.view === 'dashboard')) scheduleRefresh();
    return;
  }
  if (type === 'agent.offline') {
    const d = msg.data || {};
    const a = state.agents.find((item) => Number(item.id) === Number(d.agent_id));
    if (a) {
      if (d.last_seen) a.last_seen = d.last_seen;
      a.connected = false;
      a.transport = '';
      updateAgentCard(a);
      updateMiniAgent(a);
      updateAgentSummary();
      if (state.modal?.kind === 'agent' && Number(state.modal?.agentId) === Number(a.id)) updateAgentDetailLive(a);
      return;
    }
  }
  if (type === 'agent.updated') {
    const d = msg.data || {};
    const a = state.agents.find((item) => Number(item.id) === Number(d.agent_id));
    if (a) {
      a.cpu_pct = d.cpu ?? a.cpu_pct;
      a.ram_pct = d.ram ?? a.ram_pct;
      a.disk_pct = d.disk ?? a.disk_pct;
      a.load1 = d.load1 ?? a.load1;
      a.net_rx_bps = d.net_rx_bps ?? a.net_rx_bps;
      a.net_tx_bps = d.net_tx_bps ?? a.net_tx_bps;
      a.ram_total = d.ram_total ?? a.ram_total;
      a.disk_total = d.disk_total ?? a.disk_total;
      a.version = d.version || a.version;
      a.hostname = d.hostname || a.hostname;
      a.os = d.os || a.os;
      a.ip = d.ip || a.ip;
      a.last_seen = d.last_seen || a.last_seen;
      if (typeof d.connected === 'boolean') a.connected = d.connected;
      if (d.transport) a.transport = d.transport;
      if (state.summary?.stats) {
        state.summary.stats.agents_online = state.agents.filter(statusFromAgent).length;
      }
      const cardPatched = updateAgentCard(a);
      updateMiniAgent(a);
      updateAgentSummary();
      if (state.modal?.kind === 'agent' && Number(state.modal?.agentId) === Number(a.id)) updateAgentDetailLive(a);
      if (state.view === 'servers') {
        if (!cardPatched) scheduleRefresh();
        return;
      }
      return;
    }
  }
  scheduleRefresh();
}
const LIVE_VIEWS = new Set(['dashboard', 'monitors', 'incidents', 'servers']);
function scheduleRefresh() {
  if (!LIVE_VIEWS.has(state.view) || state.refreshTimer) return;
  state.refreshTimer = setTimeout(() => {
    state.refreshTimer = null;
    if (!state.auth?.authenticated || !state.booted) return;
    if (state.modal || state.confirm) { state.pendingRefresh = true; return; }
    if (!LIVE_VIEWS.has(state.view)) return;
    const active = document.activeElement;
    if (active && ['INPUT', 'TEXTAREA', 'SELECT'].includes(active.tagName)) { state.pendingRefresh = true; return; }
    refreshCurrent(true);
  }, 1400);
}

/* ------------------------------------------------------------------ routing */
const VIEW_META = {
  dashboard: { title: 'داشبورد', crumb: 'مرکز فرمان', sub: 'نمای کلی زیرساخت', icon: 'dashboard' },
  monitors: { title: 'مانیتورها', crumb: 'پایش', sub: 'همه بررسی‌های فعال', icon: 'activity' },
  incidents: { title: 'رخدادها', crumb: 'پایش', sub: 'تاریخچه قطعی و بازیابی', icon: 'alert' },
  servers: { title: 'سرورها', crumb: 'زیرساخت', sub: 'ایجنت‌ها و منابع', icon: 'agent' },
  rules: { title: 'قوانین', crumb: 'اتوماسیون', sub: 'موتور هوشمند هشدار', icon: 'sliders' },
  cloudflare: { title: 'کلادفلر', crumb: 'اتوماسیون', sub: 'DNS و سوییچ خودکار', icon: 'cloud' },
  notifications: { title: 'اعلان‌ها', crumb: 'ارتباط', sub: 'کانال‌ها و سابقه ارسال', icon: 'bell' },
  logs: { title: 'لاگ‌ها', crumb: 'سیستم', sub: 'رویدادها و خطاهای همهٔ بخش‌ها', icon: 'terminal' },
  settings: { title: 'تنظیمات', crumb: 'سیستم', sub: 'پیکربندی پالس', icon: 'settings' },
};

function parseHash() {
  const raw = (location.hash || '#/dashboard').replace(/^#\/?/, '');
  const parts = raw.split('/').filter(Boolean);
  const view = VIEW_META[parts[0]] ? parts[0] : 'dashboard';
  const sub = view === 'logs' ? (parts[1] || 'all') : null;
  const id = sub ? null : (parts[1] ? Number(parts[1]) : null);
  return { view, id: Number.isFinite(id) && id > 0 ? id : null, sub };
}

async function route() {
  if (!state.auth?.authenticated) return;
  const { view, id, sub } = parseHash();
  state.view = view;
  state.routeId = id;
  state.routeSub = sub;
  state.pendingRefresh = false;
  state.detailEpoch++;
  clearTimeout(state.refreshTimer);
  state.refreshTimer = null;
  if (view !== 'logs') stopLogsPoll();
  closeConfirm();
  closeModal(true);
  closeUserMenu();
  closeSidebar();
  updateShell(view);
  await loadView(view, id, false);
}

function updateShell(view) {
  const meta = VIEW_META[view] || VIEW_META.dashboard;
  $('#page-title').textContent = meta.title;
  $('#page-crumb').textContent = meta.crumb;
  $('#page-sub').textContent = meta.sub;
  const translatedTitle = EN ? translateText(meta.title) : meta.title;
  const siteTitle = state.auth?.site_title || (EN ? 'Pulse' : 'پالس');
  document.title = `${translatedTitle} | ${EN ? translateText(siteTitle) : siteTitle}`;
  $$('.nav-item').forEach((el) => el.classList.toggle('active', el.dataset.route === view));
  updateBadges();
}

function updateBadges() {
  const s = state.summary?.stats;
  const total = s?.total ?? (Array.isArray(state.monitors) && state.monitors.length ? state.monitors.length : null);
  const active = s?.active_incidents ?? (Array.isArray(state.incidents) && state.incidents.length ? state.incidents.filter((i) => !i.ended_at).length : null);
  const monBadge = $('#nav-monitors-badge');
  if (monBadge && total !== null) {
    const n = Number(total) || 0;
    monBadge.textContent = num(n);
    monBadge.classList.toggle('show', n > 0);
  }
  const incBadge = $('#nav-incidents-badge');
  if (incBadge && active !== null) {
    const n = Number(active) || 0;
    incBadge.textContent = num(n);
    incBadge.classList.toggle('show', n > 0);
  }
}

function renderLoading() {
  const view = state.view;
  let cards = 3;
  if (view === 'dashboard') cards = 4;
  if (view === 'servers') cards = 2;
  $('#view').innerHTML = `<div class="section-head"><div><h2><span class="dot-mark"></span>در حال دریافت داده…</h2><p>یک لحظه صبر کنید</p></div></div>
    <div class="loading-grid">${Array.from({ length: cards }).map(() => '<div class="panel loading-card skeleton"></div>').join('')}</div>`;
}

function renderError(error) {
  $('#view').innerHTML = `<div class="panel empty" style="margin-top:24px">
    <div class="empty-ico">${icon('alert')}</div>
    <h3>دریافت اطلاعات ناموفق بود</h3>
    <p>${esc(error?.message || 'خطای ناشناخته')}</p>
    <button class="btn btn-primary" type="button" data-action="refresh">تلاش دوباره ${icon('refresh')}</button>
  </div>`;
}

async function loadView(view, id, preserveScroll) {
  const epoch = ++state.viewEpoch;
  const stale = () => epoch !== state.viewEpoch;
  const scrollY = window.scrollY;
  if (!preserveScroll) {
    renderLoading();
    window.scrollTo({ top: 0, behavior: 'auto' });
  }
  try {
    if (view === 'dashboard') {
      const summary = await api('/api/summary');
      if (stale()) return;
      state.summary = summary;
      state.monitors = state.summary.monitors || [];
      state.agents = state.summary.agents || [];
      state.agentsOfflineAfter = state.summary.offline_after_sec || 120;
      renderDashboard();
      updateBadges();
    } else if (view === 'monitors') {
      const data = await api('/api/monitors');
      if (stale()) return;
      state.monitors = data.monitors || [];
      if (state.summary?.stats) state.summary.stats.total = state.monitors.length;
      if (state.summary) state.summary.monitors = state.monitors;
      renderMonitors();
      updateBadges();
      if (id) await openMonitorDetail(id, { fromRoute: true });
    } else if (view === 'incidents') {
      const data = await api('/api/incidents?limit=200');
      if (stale()) return;
      state.incidents = data.incidents || [];
      if (state.summary?.stats) state.summary.stats.active_incidents = state.incidents.filter((i) => !i.ended_at).length;
      renderIncidents();
      updateBadges();
    } else if (view === 'servers') {
      const data = await api('/api/agents');
      if (stale()) return;
      state.agents = data.agents || [];
      state.agentsOfflineAfter = data.offline_after_sec || 120;
      renderServers();
      if (id) await openAgentDetail(id, { fromRoute: true });
    } else if (view === 'rules') {
      const [rules, monitors] = await Promise.all([api('/api/rules'), api('/api/monitors')]);
      if (stale()) return;
      state.rules = rules.rules || [];
      state.monitors = monitors.monitors || [];
      renderRules();
    } else if (view === 'cloudflare') {
      await loadCloudflareData();
      if (stale()) return;
    } else if (view === 'notifications') {
      await loadNotificationsData();
      if (stale()) return;
    } else if (view === 'logs') {
      await loadLogsData({ reset: true });
      if (stale()) return;
    } else if (view === 'settings') {
      const [settings, audit] = await Promise.all([api('/api/settings'), api('/api/audit?limit=8')]);
      if (stale()) return;
      state.settingsData = settings;
      state.audit = audit.audit || [];
      renderSettings();
    }
    if (stale()) return;
    afterRender();
    if (EN) translateDOM(document.body);
  } catch (e) {
    if (stale()) return;
    if (e?.status !== 401) renderError(e);
  }
  if (preserveScroll) window.scrollTo({ top: scrollY });
}

async function refreshCurrent(preserve = true) {
  if (!state.auth?.authenticated) return;
  const { view, id, sub } = parseHash();
  state.routeSub = sub;
  await loadView(view, preserve ? null : id, preserve);
}

/* ------------------------------------------------------------------ modal helpers / generic UI */
function statusPill(status) {
  return `<span class="status-pill ${esc(status)}"><span class="sdot"></span>${statusLabel(status)}</span>`;
}
function typeChip(type) {
  const t = typeMeta(type);
  return `<span class="type-chip" title="${esc(t.label)}">${icon(t.icon)}${esc(t.short)}</span>`;
}
function gaugeRing(percent, color, label = '', key = '') {
  const p = clamp(Number(percent) || 0, 0, 100);
  const c = 2 * Math.PI * 30;
  const offset = c - (p / 100) * c;
  return `<div class="gauge-unit"${key ? ` data-agent-gauge="${key}"` : ''}><div class="gauge"><svg viewBox="0 0 80 80"><circle class="track" cx="40" cy="40" r="30"/><circle class="val" cx="40" cy="40" r="30" stroke="${color}" stroke-dasharray="${c.toFixed(2)}" stroke-dashoffset="${offset.toFixed(2)}"/></svg><span>${num(p)}٪</span></div><div class="gauge-label">${esc(label)}</div></div>`;
}
function gaugeColor(value) {
  if (value >= 90) return '#fb7185';
  if (value >= 75) return '#fbbf24';
  return '#38bdf8';
}
function statusFromAgent(a) {
  if (typeof a.connected === 'boolean') return a.connected;
  if (!a.last_seen) return false;
  return Date.now() / 1000 - a.last_seen <= (state.agentsOfflineAfter || 120);
}
function geoFlag(code) {
  const cc = String(code || '').trim().toUpperCase();
  if (!/^[A-Z]{2}$/.test(cc)) return '';
  return String.fromCodePoint(...[...cc].map((ch) => 0x1F1E6 + ch.charCodeAt(0) - 65));
}
function geoPlace(info) {
  if (!info || info.error) return '';
  const place = [info.country, info.city].filter(Boolean).join(' · ');
  return [geoFlag(info.country_code), place].filter(Boolean).join(' ');
}
async function hydrateAgentLocations(root = document) {
  const nodes = Array.from(root.querySelectorAll('[data-agent-location]'));
  await Promise.all(nodes.map(async (el) => {
    const ip = el.dataset.ip || '';
    if (!ip) { el.textContent = ''; return; }
    const info = await geoLookup(ip);
    if (!el.isConnected || el.dataset.ip !== ip) return;
    const place = geoPlace(info);
    el.textContent = place ? ` · ${place}` : '';
  }));
}
async function geoLookup(ip) {
  const key = String(ip || '').trim();
  if (!key) return null;
  if (state.geoCache[key]) return state.geoCache[key];
  try {
    const info = await api(`/api/geo?ip=${encodeURIComponent(key)}`, { timeout: 12000 });
    if (info) state.geoCache[key] = info;
    return info;
  } catch {
    state.geoCache[key] = { error: true };
    return null;
  }
}
function agentQuality(a) {
  const online = statusFromAgent(a);
  const age = a.last_seen ? Math.max(0, Date.now() / 1000 - Number(a.last_seen)) : null;
  if (!online) return { online, text: age === null ? 'بدون گزارش' : 'قطع', cls: 'off', age };
  if (age < 10) return { online, text: 'عالی', cls: 'great', age };
  if (age < 30) return { online, text: 'خوب', cls: 'good', age };
  return { online, text: 'متصل', cls: 'ok', age };
}
function monitorCard(m, options = {}) {
  const status = m.status || 'unknown';
  const spark = (m.spark || []).slice(-28);
  const warn = sslBadge(m);
  const active = m.active_incident;
  return `<article class="panel monitor-card ${esc(status)}" data-action="open-monitor" data-id="${m.id}" tabindex="0" role="button">
    <div class="mc-top">${typeChip(m.type)}<div style="display:flex;gap:5px;align-items:center">${warn}${statusPill(status)}</div></div>
    <div class="mc-name" title="${esc(m.name)}">${esc(m.name)}</div>
    <div class="mc-target" title="${esc(m.target)}">${esc(m.target)}</div>
    ${m.probe_agent_id ? `<div class="mc-probe" title="محل اجرای تست">${icon('agent')} تست از ${esc(m.probe_agent_name || 'سرور انتخاب‌شده')}</div>` : ''}
    <div class="mc-spark"><canvas class="spark" data-points="${spark.join(',')}" data-color="${sparkColor(status)}"></canvas></div>
    <div class="mc-foot">
      <span class="mc-stat"><b class="${m.uptime_24h >= 99 ? 'ok' : m.uptime_24h > 0 && m.uptime_24h < 95 ? 'bad' : ''}">${m.uptime_24h ? uptimePct(m.uptime_24h) : '—'}</b> آپتایم</span>
      <span class="mc-stat"><b>${m.last_latency_ms ? num(m.last_latency_ms) + 'ms' : '—'}</b></span>
      <span class="mc-stat">${timeAgo(m.last_checked_at)}</span>
      <span class="mc-actions">
        <button class="icon-btn" style="width:27px;height:27px;border-radius:8px" type="button" title="بررسی فوری" aria-label="بررسی فوری ${esc(m.name)}" data-action="check-monitor" data-id="${m.id}">${icon('refresh')}</button>
        <button class="icon-btn" style="width:27px;height:27px;border-radius:8px" type="button" title="${m.enabled ? 'توقف' : 'فعال‌سازی'}" aria-label="${m.enabled ? 'توقف' : 'فعال‌سازی'} ${esc(m.name)}" data-action="toggle-monitor" data-id="${m.id}">${icon(m.enabled ? 'pause' : 'play')}</button>
      </span>
    </div>
    ${active ? `<div style="font-size:9px;color:var(--danger-text);display:flex;gap:4px;align-items:center"><span style="width:4px;height:4px;border-radius:50%;background:#fb7185;display:inline-block"></span> رخداد باز از ${timeAgo(active.started_at)}</div>` : ''}
  </article>`;
}
function incidentRow(inc, active, actions = false) {
  return `<div class="incident-row">
    <div class="incident-ico ${inc.ended_at ? 'closed' : 'open'}">${icon(inc.ended_at ? 'check' : 'alert')}</div>
    <div class="incident-body">
      <div class="incident-name"><a href="#/monitors/${inc.monitor_id}" data-action="open-monitor" data-id="${inc.monitor_id}" style="color:var(--text)">${esc(inc.monitor_name)}</a>${inc.ended_at ? '<span class="chip green">بازیابی شد</span>' : '<span class="chip red">در جریان</span>'}</div>
      <div class="incident-cause">${esc(inc.cause || 'بدون جزئیات')}</div>
      <div class="incident-meta"><span>شروع: ${timeAgo(inc.started_at)}</span><span>${inc.ended_at ? 'مدت: ' + duration(inc.duration_sec) : 'در حال وقوع'}</span>${active ? '' : `<span>${dateShort(inc.started_at)}</span>`}</div>
    </div>
    ${actions && !inc.ended_at ? `<button class="btn btn-sm btn-ghost" type="button" data-action="resolve-incident" data-id="${inc.id}">بستن دستی</button>` : ''}
  </div>`;
}
function notificationRow(n) {
  const ok = n.ok;
  const channelIcon = n.channel === 'telegram' ? 'send' : n.channel === 'webpush' ? 'smartphone' : n.channel === 'webhook' ? 'webhook' : n.channel === 'cloudflare' ? 'cloud' : 'bell';
  return `<div class="notif-row">
    <div class="notif-ico" style="${ok ? '' : 'background:rgba(251,113,133,.08);color:var(--danger-text)'}">${icon(channelIcon)}</div>
    <div class="notif-body"><div class="notif-title">${esc(n.title || n.rule_name || 'اعلان')}</div><div class="notif-meta">${esc(n.channel)} · ${ok ? 'ارسال شد' : esc(n.error || 'ناموفق')}</div></div>
    <div class="notif-time">${timeAgo(n.at)}</div>
  </div>`;
}

function bindGlobalEvents() {
  document.addEventListener('click', onDocumentClick);
  document.addEventListener('submit', onDocumentSubmit);
  document.addEventListener('change', onDocumentChange);
  document.addEventListener('input', onDocumentInput);
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      if (state.confirm) closeConfirm();
      else if (state.modal) closeModal();
      closeUserMenu();
      closeSidebar();
      return;
    }
    if (e.key === 'Enter' || e.key === ' ') {
      const card = e.target.closest?.('[role="button"][data-action]');
      if (card && e.target === card) {
        e.preventDefault();
        card.click();
        return;
      }
    }
    if (e.key === 'Tab') {
      const scope = state.confirm?.el || $('.modal-backdrop');
      if (!scope) return;
      const focusable = $$('button:not([disabled]), [href], input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])', scope)
        .filter((el) => el.offsetParent !== null);
      if (!focusable.length) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (e.shiftKey && (document.activeElement === first || !scope.contains(document.activeElement))) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && (document.activeElement === last || !scope.contains(document.activeElement))) {
        e.preventDefault();
        first.focus();
      }
    }
  });
  window.addEventListener('hashchange', () => {
    closeConfirm();
    route();
  });
  document.addEventListener('focusout', () => {
    setTimeout(() => {
      if (!state.pendingRefresh || state.modal || !LIVE_VIEWS.has(state.view)) return;
      const active = document.activeElement;
      if (active && ['INPUT', 'TEXTAREA', 'SELECT'].includes(active.tagName)) return;
      state.pendingRefresh = false;
      refreshCurrent(true);
    }, 300);
  });
  window.addEventListener('beforeinstallprompt', (e) => { e.preventDefault(); state.installPrompt = e; });
}

function registerServiceWorker() {
  if (!('serviceWorker' in navigator)) return;
  navigator.serviceWorker.register('/sw.js').catch(() => { /* http or unsupported */ });
}

/* ------------------------------------------------------------------ dashboard */
function renderDashboard() {
  const d = state.summary || {};
  const stats = d.stats || {};
  const monitors = (d.monitors || []).slice().sort((a, b) => {
    const rank = { down: 0, unknown: 1, degraded: 2, up: 3, paused: 4 };
    return (rank[a.status] ?? 5) - (rank[b.status] ?? 5) || a.name.localeCompare(b.name, 'fa');
  });
  const agents = d.agents || [];
  const onlineAgents = agents.filter(statusFromAgent);
  const gaugePct = Number(stats.uptime_24h) || 0;
  const downCount = Number(stats.down) || 0;
  const degradedCount = Number(stats.degraded) || 0;
  const issueCount = downCount + degradedCount;
  const hasDown = downCount > 0;
  const hasIssues = issueCount > 0;
  const trend = (d.trend || []).filter((p) => p.total > 0);
  const latencyPoints = trend.filter((p) => p.avg_latency_ms > 0);
  const latencyAvg = latencyPoints.length ? latencyPoints.reduce((s, p) => s + p.avg_latency_ms, 0) / latencyPoints.length : 0;
  const C = 2 * Math.PI * 74;
  const offset = C - (clamp(gaugePct, 0, 100) / 100) * C;
  const topMonitors = monitors.slice(0, 8);
  const recentIncidents = (d.incidents || []).slice(0, 4);
  const activeIncidents = d.active_incidents || [];
  const notifications = d.notifications || [];
  const noData = monitors.length === 0;

  const heroTitle = hasDown
    ? (EN
      ? `${num(downCount)} ${downCount === 1 ? 'service needs' : 'services need'} <span style="color:var(--danger-text)">attention</span>`
      : `${num(downCount)} سرویس <span style="color:var(--danger-text)">نیاز به توجه</span> دارد`)
    : degradedCount
      ? (EN
        ? `${num(degradedCount)} ${degradedCount === 1 ? 'service has a' : 'services have'} <span style="color:var(--warning-text)">warning</span>`
        : `${num(degradedCount)} سرویس <span style="color:var(--warning-text)">هشدار</span> دارد`)
      : noData
        ? (EN ? 'The first step to <span>peace of mind</span>' : 'اولین قدم برای <span>آرامش خاطر</span>')
        : (EN ? 'Everything under <span>control</span>' : 'همه‌چیز زیر <span>کنترل</span> است');
  const heroDesc = hasDown
    ? (EN
      ? 'One or more services are unreachable. Review the details and enable automatic DNS failover if needed.'
      : 'یک یا چند سرویس از دسترس خارج شده‌اند. جزئیات را ببینید و در صورت نیاز، سوییچ خودکار DNS را فعال کنید.')
    : degradedCount
      ? (EN
        ? 'One or more services are slow or unstable. Review the latest checks and details.'
        : 'عملکرد یک یا چند سرویس کند یا ناپایدار است. آخرین بررسی‌ها و جزئیات را ببینید.')
      : noData
        ? (EN
          ? 'You have not created a monitor yet. Add a domain, server or config and Pulse starts watching immediately.'
          : 'هنوز مانیتوری نساخته‌اید. یک دامنه، سرور یا کانفیگ اضافه کنید تا پالس از همین لحظه مراقبت را آغاز کند.')
        : (EN
          ? `Pulse monitored ${num(stats.total)} ${Number(stats.total) === 1 ? 'service' : 'services'} and ${num(stats.agents_total || 0)} ${Number(stats.agents_total) === 1 ? 'server' : 'servers'} in the last 24 hours.`
          : `پالس در 24 ساعت گذشته ${num(stats.total)} سرویس و ${num(stats.agents_total || 0)} سرور را زیر نظر داشته است.`);

  $('#view').innerHTML = `
    <section class="panel overview-panel">
      <div class="overview-copy">
        <span class="eyebrow" style="${hasDown ? 'color:var(--danger-text);border-color:rgba(251,113,133,.2);background:rgba(251,113,133,.06)' : degradedCount ? 'color:var(--warning-text);border-color:rgba(251,146,60,.2);background:rgba(251,146,60,.06)' : ''}">${icon(hasIssues ? 'alert' : 'activity')} ${hasIssues ? 'نیازمند بررسی' : 'پایش لحظه‌ای'}</span>
        <h2>${heroTitle}</h2>
        <p>${heroDesc}</p>
        <div class="overview-tags">
          <span class="chip cyan">${icon('activity')} ${num(stats.total || 0)} مانیتور</span>
          <span class="chip green">${icon('check')} ${num(stats.up || 0)} سالم</span>
          ${hasDown ? `<span class="chip red">${icon('alert')} ${num(downCount)} قطع</span>` : ''}
          ${degradedCount ? `<span class="chip amber">${icon('alert')} ${num(degradedCount)} هشدار</span>` : ''}
          <span class="chip violet" data-dashboard-agent-chip>${icon('server')} ${num(onlineAgents.length)} سرور آنلاین</span>
        </div>
      </div>
      <div class="overview-gauge">
        <div class="gauge-big">
          <svg viewBox="0 0 176 176">
            <defs><linearGradient id="gaugeGrad" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="${hasDown ? '#fb7185' : hasIssues ? '#fb923c' : '#38bdf8'}"/><stop offset="1" stop-color="${hasDown ? '#f97316' : '#8b5cf6'}"/></linearGradient></defs>
            <circle class="track" cx="88" cy="88" r="74"/>
            <circle class="bar" cx="88" cy="88" r="74" stroke-dasharray="${C.toFixed(1)}" stroke-dashoffset="${offset.toFixed(1)}"/>
          </svg>
          <div class="gauge-center"><b>${trend.length || monitors.length ? num(gaugePct, 1) : '—'}<small>٪</small></b><span>آپتایم 24 ساعته</span></div>
        </div>
        <div class="gauge-caption"><span class="dot" style="background:${hasDown ? '#fb7185' : hasIssues ? '#fb923c' : '#34d399'}"></span>${hasDown ? `${num(stats.active_incidents || 0)} رخداد باز` : degradedCount ? `${num(degradedCount)} سرویس در هشدار` : 'همه سرویس‌ها پایدارند'}</div>
      </div>
    </section>

    <div class="stat-grid">
      <div class="panel stat-card">
        <div class="stat-top"><span class="stat-label">سرویس‌های سالم</span><span class="stat-ico green">${icon('check')}</span></div>
        <div class="stat-value">${num(stats.up || 0)}<small>از ${num(stats.total || 0)}</small></div>
        <div class="stat-foot"><span class="up">${num(stats.unknown || 0)}</span> در انتظار اولین بررسی</div>
      </div>
      <div class="panel stat-card">
        <div class="stat-top"><span class="stat-label">قطعی / هشدار</span><span class="stat-ico red">${icon('alert')}</span></div>
        <div class="stat-value" style="${hasIssues ? 'color:var(--danger-text)' : ''}">${num(issueCount)}<small>سرویس</small></div>
        <div class="stat-foot">${num(stats.active_incidents || 0)} رخداد در جریان · ${num(stats.paused || 0)} متوقف</div>
      </div>
      <div class="panel stat-card">
        <div class="stat-top"><span class="stat-label">آپتایم 24 ساعته</span><span class="stat-ico cyan">${icon('chart')}</span></div>
        <div class="stat-value">${trend.length || monitors.length ? num(gaugePct, 1) : '—'}<small>٪</small></div>
        <div class="stat-foot"><span class="${gaugePct >= 99 ? 'up' : 'down'}">${gaugePct >= 99 ? (EN ? 'Stable' : 'پایدار') : (EN ? 'Needs attention' : 'نیازمند توجه')}</span>${EN ? ' · Average of all monitors' : ' میانگین همه مانیتورها'}</div>
      </div>
      <div class="panel stat-card">
        <div class="stat-top"><span class="stat-label">سرورهای آنلاین</span><span class="stat-ico violet">${icon('server')}</span></div>
        <div class="stat-value" data-dashboard-agents-value>${num(onlineAgents.length)}<small>از ${num(agents.length)}</small></div>
        <div class="stat-foot" data-dashboard-agents-foot>${agents.length ? (onlineAgents.length === agents.length ? 'همه ایجنت‌ها متصل' : 'برخی ایجنت‌ها قطع هستند') : 'هنوز ایجنتی اضافه نشده'}</div>
      </div>
    </div>

    ${noData ? `<div class="banner" style="margin-top:16px">
      <div class="banner-ico">${icon('bolt')}</div>
      <div class="banner-body"><b>با چند کلیک شروع کنید</b><p>یک دامنه، پورت یا کانفیگ اضافه کنید؛ بررسی‌ها و گواهی SSL به‌صورت خودکار پایش می‌شوند.</p></div>
      <button class="btn btn-primary btn-sm" type="button" data-action="quick-monitor">${icon('plus')} مانیتور جدید</button>
    </div>` : ''}

    <div class="dash-mid">
      <div class="panel chart-panel">
        <div class="chart-head">
          <div><div class="chart-title"><span class="live-dot"></span> روند پاسخ‌دهی</div><div class="chart-sub">میانگین تأخیر همه بررسی‌ها در 24 ساعت گذشته</div></div>
          <div class="chart-legend"><span class="chip">${num(latencyAvg)} ms ${EN ? 'average' : 'میانگین'}</span><span class="chip cyan">${EN ? '24 hours' : '24 ساعت'}</span></div>
        </div>
        <div class="chart-canvas-wrap">
          <canvas id="trendChart"></canvas>
          ${trend.length < 2 ? `<div class="chart-empty">${icon('chart')}<span>به اندازه کافی داده ثبت نشده است</span></div>` : ''}
        </div>
      </div>

      <div class="panel server-mini-panel">
        <div class="chart-head" style="margin-bottom:0">
          <div><div class="chart-title">${icon('server')} سلامت سرورها</div><div class="chart-sub">مصرف لحظه‌ای منابع ایجنت‌ها</div></div>
          <a class="btn btn-sm btn-ghost" href="#/servers">همه ${icon('chevron')}</a>
        </div>
        <div class="mini-agent-list">
          ${agents.length ? agents.slice(0, 5).map((a) => {
            const on = statusFromAgent(a);
            return `<div class="mini-agent" data-action="open-agent" data-id="${a.id}" data-mini-agent="${a.id}" role="button" tabindex="0">
              <div class="mini-ico">${icon('server')}</div>
              <div class="mini-info"><div class="mini-name">${esc(a.name)}</div><div class="mini-meta" data-mini-meta title="IP عمومی خروجی؛ ممکن است با IP اصلی سرور متفاوت باشد.">${on ? 'آنلاین' : 'آفلاین'} · ${esc(a.ip || 'بدون IP')}<span data-agent-location data-ip="${esc(a.ip || '')}"></span></div></div>
              <div class="mini-bars">
                <div class="mini-bar"><span class="cpu" data-mini-cpu style="width:${clamp(a.cpu_pct || 0, 0, 100)}%"></span></div>
                <div class="mini-bar"><span class="ram" data-mini-ram style="width:${clamp(a.ram_pct || 0, 0, 100)}%"></span></div>
              </div>
              <span class="agent-online ${on ? '' : 'off'}" data-mini-online style="padding:2px 6px"><span class="dot"></span></span>
            </div>`;
          }).join('') : `<div class="empty" style="padding:28px 10px"><div class="empty-ico" style="width:46px;height:46px">${icon('server')}</div><h3 style="font-size:12px">سروری متصل نیست</h3><p style="font-size:10px">با نصب ایجنت، منابع سرور را زنده ببینید.</p><a class="btn btn-sm btn-soft" href="#/servers">${icon('plus')} افزودن سرور</a></div>`}
        </div>
      </div>
    </div>

    <div class="section-head">
      <div><h2><span class="dot-mark"></span> وضعیت زنده سرویس‌ها</h2><p>${monitors.length ? `${num(monitors.length)} مانیتور پیکربندی شده` : 'هنوز مانیتوری ندارید'}</p></div>
      <div class="head-actions"><a class="btn btn-sm btn-ghost" href="#/monitors">مشاهده همه ${icon('chevron')}</a><button class="btn btn-sm btn-primary" type="button" data-action="quick-monitor">${icon('plus')} مانیتور</button></div>
    </div>
    ${topMonitors.length ? `<div class="monitor-grid">${topMonitors.map((m) => monitorCard(m)).join('')}</div>` : `<div class="panel empty"><div class="empty-ico">${icon('activity')}</div><h3>هنوز چیزی برای پایش نیست</h3><p>با ساخت اولین مانیتور، وضعیت لحظه‌ای و نمودار تأخیر آن اینجا نمایش داده می‌شود.</p><button class="btn btn-primary btn-sm" type="button" data-action="quick-monitor">${icon('plus')} ساخت اولین مانیتور</button></div>`}

    <div class="two-col">
      <div class="panel panel-pad">
        <div class="chart-head" style="margin-bottom:4px">
          <div><div class="chart-title">${icon('alert')} رخدادهای اخیر</div><div class="chart-sub">${activeIncidents.length ? `${num(activeIncidents.length)} رخداد باز` : 'همه چیز آرام است'}</div></div>
          <a class="btn btn-sm btn-ghost" href="#/incidents">تاریخچه ${icon('chevron')}</a>
        </div>
        <div class="incident-list">
          ${recentIncidents.length ? recentIncidents.map((i) => incidentRow(i, i.ended_at === 0)).join('') : `<div class="empty" style="padding:28px 10px"><div class="empty-ico" style="width:44px;height:44px">${icon('shield')}</div><h3 style="font-size:12px">رخدادی ثبت نشده</h3><p style="font-size:10px">وقتی سرویسی از دسترس خارج شود، اینجا گزارش می‌شود.</p></div>`}
        </div>
      </div>
      <div class="panel panel-pad">
        <div class="chart-head" style="margin-bottom:4px">
          <div><div class="chart-title">${icon('bell')} آخرین اعلان‌ها</div><div class="chart-sub">کانال‌های ارتباطی</div></div>
          <a class="btn btn-sm btn-ghost" href="#/notifications">مدیریت ${icon('chevron')}</a>
        </div>
        <div class="notif-list">
          ${notifications.length ? notifications.map(notificationRow).join('') : `<div class="empty" style="padding:24px 10px"><div class="empty-ico" style="width:44px;height:44px">${icon('send')}</div><h3 style="font-size:12px">اعلانی ارسال نشده</h3><p style="font-size:10px">تلگرام یا Web Push را فعال کنید.</p><a class="btn btn-sm btn-soft" href="#/notifications">تنظیم کانال‌ها</a></div>`}
        </div>
      </div>
    </div>`;
  hydrateAgentLocations($('#view'));
}

/* ------------------------------------------------------------------ monitors page */
function filteredMonitors() {
  const q = state.filters.search.trim().toLowerCase();
  return state.monitors.filter((m) => {
    if (state.filters.status !== 'all' && m.status !== state.filters.status) return false;
    if (!q) return true;
    return `${m.name} ${m.target} ${m.group}`.toLowerCase().includes(q);
  });
}
function monitorGridHTML() {
  const list = filteredMonitors();
  if (!list.length) {
    if (!state.monitors.length) {
      return `<div class="panel empty" style="grid-column:1/-1"><div class="empty-ico">${icon('activity')}</div><h3>اولین مانیتور خود را بسازید</h3><p>دامنه، سرور یا کانفیگ پروکسی را اضافه کنید تا پالس هر 60 ثانیه وضعیت آن را بررسی کند.</p><button class="btn btn-primary btn-sm" type="button" data-action="quick-monitor">${icon('plus')} مانیتور جدید</button></div>`;
    }
    return `<div class="panel empty" style="grid-column:1/-1"><div class="empty-ico">${icon('search')}</div><h3>نتیجه‌ای پیدا نشد</h3><p>عبارت جستجو یا فیلتر وضعیت را تغییر دهید.</p><button class="btn btn-ghost btn-sm" type="button" data-action="clear-filters">پاک کردن فیلترها</button></div>`;
  }
  return list.map((m) => monitorCard(m)).join('');
}
function refreshMonitorGrid() {
  const el = $('#monitors-page-grid');
  if (!el) return;
  el.innerHTML = monitorGridHTML();
  $$('.spark', el).forEach(drawSpark);
  const summary = $('[data-monitor-summary]');
  if (summary) summary.innerHTML = monitorSummaryHTML();
  $$('[data-filter]').forEach((b) => b.classList.toggle('active', b.dataset.filter === state.filters.status));
}
function monitorSummaryHTML() {
  const total = state.monitors.length;
  const visible = filteredMonitors();
  const filtered = visible.length;
  const down = visible.filter((m) => m.status === 'down').length;
  const degraded = visible.filter((m) => m.status === 'degraded').length;
  return `<span id="monitor-count-text">${filtered === total ? `${num(total)} مانیتور` : `${num(filtered)} از ${num(total)} مانیتور`}</span>${down ? `<span class="badge-dot bad"><span class="dot"></span>${num(down)} قطع</span>` : ''}${degraded ? `<span class="badge-dot warn"><span class="dot"></span>${num(degraded)} هشدار</span>` : ''}${!down && !degraded && filtered ? '<span class="badge-dot"><span class="dot"></span>همه سالم</span>' : ''}`;
}
function renderMonitors() {
  $('#view').innerHTML = `
    <div class="page-toolbar">
      <div class="search-box">${icon('search')}<input id="monitor-search" placeholder="جستجوی نام، دامنه یا گروه…" value="${esc(state.filters.search)}" autocomplete="off"></div>
      <div class="segmented">
        ${[['all', 'همه'], ['up', 'سالم'], ['degraded', 'هشدار'], ['down', 'قطعی'], ['unknown', 'در انتظار'], ['paused', 'متوقف']].map(([key, label]) => `<button type="button" data-filter="${key}" class="${state.filters.status === key ? 'active' : ''}">${label}</button>`).join('')}
      </div>
      <div class="toolbar-spacer"></div>
      <div class="page-summary" data-monitor-summary>${monitorSummaryHTML()}</div>
      <button class="btn btn-primary" type="button" data-action="quick-monitor">${icon('plus')} مانیتور جدید</button>
    </div>
    <div class="monitor-page-grid" id="monitors-page-grid">${monitorGridHTML()}</div>`;
}

/* ------------------------------------------------------------------ monitor detail */
async function openMonitorDetail(id, opts = {}) {
  const guard = ++state.detailEpoch;
  if (state.modal) closeModal(true);
  const skeleton = openDetailSkeleton('در حال بارگذاری مانیتور…', 'دریافت وضعیت، نمودارها و رخدادها');
  let data;
  try {
    data = await api(`/api/monitors/${id}`);
  } catch (e) {
    if (guard === state.detailEpoch && state.view === 'monitors') {
      if (!skeleton.isClosed()) closeModal(true);
      toast(e.message, 'error');
      if (parseHash().id === id) history.replaceState(null, '', '#/monitors');
    }
    return;
  }
  if (guard !== state.detailEpoch || state.view !== 'monitors') return;
  if (skeleton.isClosed()) return;
  const m = data.monitor;
  const checks = data.checks || [];
  const daily = data.daily || [];
  const incidents = data.incidents || [];
  const serverTime = data.server_time || Date.now() / 1000;
  const last = checks[0] || null;
  const meta = last?.meta || {};
  const latency = checks.slice().reverse().filter((c) => c.ok && c.latency_ms >= 0).map((c) => ({ x: c.at, y: c.latency_ms }));
  const days = 30;
  const dayMap = {};
  daily.forEach((d) => { dayMap[d.day] = d; });
  const bars = [];
  const now = new Date();
  for (let i = days - 1; i >= 0; i--) {
    const dt = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate() - i));
    const key = dt.toISOString().slice(0, 10);
    const row = dayMap[key];
    bars.push({ key, dt, row });
  }
  const downSec = m.down_since ? Math.max(0, serverTime - m.down_since) : 0;
  const sslDays = daysLeft(m.ssl_expires_at);
  const cfg = m.config || {};
  const activeIncident = (m.active_incident || incidents.find((i) => i.ended_at === 0) || null);

  const nodes = Array.isArray(meta.nodes) ? meta.nodes : [];
  const nodeMaxLatency = Math.max(1, ...nodes.map((n) => Number(n.latency_ms) || 0));
  const nodeHealthy = Number(meta.healthy || nodes.filter((n) => n.ok).length);
  const nodeHealthPct = nodes.length ? clamp((nodeHealthy / nodes.length) * 100, 0, 100) : 0;
  const resolved = Array.isArray(meta.resolved) ? meta.resolved : [];
  const configRows = [
    ['نوع', typeMeta(m.type).label],
    ['تست از', m.probe_agent_id ? (m.probe_agent_name || 'سرور انتخاب‌شده') : 'پنل اصلی'],
    ['گروه', m.group || '—'],
    ['فاصله بررسی', `${num(m.interval_sec)} ثانیه`],
    ['مهلت', `${num(m.timeout_ms)} میلی‌ثانیه`],
    ['آستانه خطا', `${num(m.fail_threshold)} بررسی`],
    ['بازیابی', `${num(m.recover_threshold)} بررسی`],
    ['کد وضعیت آخر', m.last_status_code ? num(m.last_status_code) : '—'],
    ['گواهی SSL', m.ssl_expires_at ? `${dateShort(m.ssl_expires_at)} (${sslDays >= 0 ? num(sslDays) + ' روز' : 'منقضی'})` : '—'],
  ];

  const chartEmpty = latency.length < 2;
  const body = `
    <div class="detail-hero">
      <div class="agent-ico" style="background:${typeMeta(m.type).color}18;color:${typeMeta(m.type).color};border-color:${typeMeta(m.type).color}33">${icon(typeMeta(m.type).icon)}</div>
      <div class="detail-title">
        <h2>${esc(m.name)} ${statusPill(m.status)} ${sslBadge(m)}</h2>
        <div class="target">${esc(m.target)}</div>
      </div>
      <div class="detail-actions">
        <button class="btn btn-sm btn-soft" type="button" data-action="check-monitor" data-id="${m.id}">${icon('refresh')} بررسی فوری</button>
        <button class="btn btn-sm" type="button" data-action="toggle-monitor" data-id="${m.id}">${icon(m.enabled ? 'pause' : 'play')} ${m.enabled ? 'توقف' : 'فعال‌سازی'}</button>
      </div>
    </div>
    ${activeIncident ? `<div class="banner" style="margin-top:14px;border-color:rgba(251,113,133,.2);background:rgba(251,113,133,.055)">
      <div class="banner-ico" style="background:rgba(251,113,133,.1);color:var(--danger-text)">${icon('alert')}</div>
      <div class="banner-body"><b>رخداد در جریان از ${timeAgo(activeIncident.started_at)}</b><p>${esc(activeIncident.cause || m.last_error || 'در حال بررسی')} · مدت: ${duration(downSec)}</p></div>
    </div>` : ''}
    <div class="detail-stats">
      <div class="mini-stat"><span>آپتایم 24 ساعته</span><b style="color:${Number(m.uptime_24h) >= 99 ? 'var(--success-text)' : 'var(--danger-text)'}">${m.uptime_24h ? uptimePct(m.uptime_24h) : '—'}</b></div>
      <div class="mini-stat"><span>میانگین تأخیر</span><b>${m.avg_latency_ms ? num(m.avg_latency_ms) + ' ms' : '—'}</b></div>
      <div class="mini-stat"><span>آخرین بررسی</span><b style="font-size:11px">${m.last_checked_at ? timeAgo(m.last_checked_at) : 'هنوز'}</b></div>
      <div class="mini-stat"><span>گواهی SSL</span><b style="font-size:11px;color:${sslDays !== null && sslDays <= 14 ? 'var(--warning-text)' : 'var(--text-soft)'}">${sslDays === null ? '—' : sslDays < 0 ? 'منقضی' : num(sslDays) + ' روز'}</b></div>
    </div>
    ${m.last_error ? `<div class="warning-note" style="margin-top:12px;border-color:rgba(251,113,133,.16);background:rgba(251,113,133,.05);color:var(--danger-text)">${icon('error')} <div><b style="display:block;font-size:10px;margin-bottom:2px">آخرین خطا</b>${esc(m.last_error)}</div></div>` : ''}
    ${last?.meta?.warn ? `<div class="warning-note" style="margin-top:12px">${icon('alert')} <div><b style="display:block;font-size:10px;margin-bottom:2px">هشدار آخرین بررسی</b>${esc(last.meta.warn)}</div></div>` : ''}
    <div class="detail-chart">
      <div class="detail-chart-head"><b>${icon('activity')} تأخیر پاسخ (${num(latency.length)} نمونه)</b><span>${dateTime(checks[checks.length - 1]?.at)} تا ${dateTime(last?.at)}</span></div>
      <div style="position:relative"><canvas id="detailLatency" style="height:150px"></canvas>${chartEmpty ? `<div class="chart-empty" style="border-radius:10px">${icon('chart')}<span>داده کافی برای نمودار وجود ندارد</span></div>` : ''}</div>
    </div>
    <div class="detail-chart">
      <div class="detail-chart-head"><b>${icon('chart')} آپتایم 30 روز</b><span>${daily.length ? uptimePct(daily.reduce((s, d) => s + d.ok, 0) / Math.max(1, daily.reduce((s, d) => s + d.total, 0)) * 100) : '—'}</span></div>
      <div class="uptime-bars">
        ${bars.map((b) => {
          if (!b.row) return `<div class="u-bar" style="height:6px;background:rgba(130,160,220,.1)" data-tip="${dateShort(b.dt.getTime() / 1000)} — بدون داده"></div>`;
          const p = b.row.uptime || 0;
          const cls = p >= 99 ? 'ok' : p >= 90 ? 'warn' : 'bad';
          return `<div class="u-bar ${cls}" style="height:${Math.max(8, (p / 100) * 54)}px" data-tip="${dateShort(b.dt.getTime() / 1000)} — ${nf1.format(p)}٪ آپتایم"></div>`;
        }).join('')}
      </div>
      <div class="uptime-legend"><span>${bars[0] ? dateShort(bars[0].dt.getTime() / 1000) : '—'}</span><span>امروز</span></div>
    </div>
    ${nodes.length ? `<div class="detail-chart">
      <div class="detail-chart-head"><b>${icon('layers')} کانفیگ‌های پروکسی</b><span>${num(nodeHealthy)} از ${num(meta.total || nodes.length)} کانفیگ سالم${meta.xray_version ? ` · Xray ${esc(meta.xray_version)}` : ''}</span></div>
      ${meta.real_test_requested ? `<div style="display:flex;gap:6px;flex-wrap:wrap;margin-bottom:8px;align-items:center"><span class="chip ${meta.real_tested > 0 ? 'green' : 'amber'}">${icon('bolt')} هندشیک واقعی: ${num(meta.real_tested || 0)} تست، ${num(meta.real_passed || 0)} موفق</span>${meta.test_url ? `<span class="chip" title="${esc(meta.test_url)}">تست: ${esc(meta.test_url)}</span>` : ''}</div>` : ''}
      <div class="node-health"><div class="node-health-bar"><span style="width:${nodeHealthPct}%"></span></div><span>${nf1.format(nodeHealthPct)}٪ سلامت کانفیگ‌ها</span></div>
      <div class="table-wrap"><table class="node-table"><thead><tr><th>کانفیگ</th><th>پروتکل</th><th>آدرس</th><th>روش</th><th>تأخیر</th><th>وضعیت</th></tr></thead><tbody>
        ${nodes.map((n) => {
          const method = n.real_tested ? 'Xray' : n.method === 'tls' ? 'TLS' : n.method === 'udp' ? 'UDP' : 'TCP';
          const methodClass = n.real_tested ? 'violet' : n.real_skipped ? 'amber' : 'cyan';
          const methodTitle = n.real_note || (n.real_tested ? 'هندشیک واقعی و درخواست HTTP از داخل تونل' : 'فقط بررسی دسترسی ترابری');
          const latPct = clamp(((Number(n.latency_ms) || 0) / nodeMaxLatency) * 100, 6, 100);
          return `<tr><td class="node-name">${esc(n.name || '—')}</td><td>${esc(n.protocol || '—')}</td><td class="record-name">${esc(n.address || '—')}${n.tls ? ' · TLS' : ''}${n.udp ? ' · UDP' : ''}</td><td><span class="chip ${methodClass}" title="${esc(methodTitle)}">${method}</span></td><td class="node-lat">${n.ok ? `<span class="lat-meter"><i style="width:${latPct}%"></i></span><b>${num(n.latency_ms)} ms</b>` : '—'}</td><td>${n.ok ? '<span class="chip green">سالم</span>' : `<span class="chip red" title="${esc(n.error || '')}">قطع</span>`}</td></tr>`;
        }).join('')}
      </tbody></table></div>
    </div>` : ''}
    ${resolved.length ? `<div class="detail-chart"><div class="detail-chart-head"><b>${icon('network')} رکورد DNS</b><span>${num(resolved.length)} مقدار</span></div><div class="service-chips">${resolved.map((v) => `<span class="chip cyan" style="direction:ltr">${esc(v)}</span>`).join('')}</div></div>` : ''}
    ${meta.provider === 'virtualizor' ? (() => {
      const used = Number(meta.used_gb) || 0;
      const limit = Number(meta.limit_gb) || 0;
      const remaining = Number(meta.remaining_gb) || 0;
      const pct = clamp(Number(meta.percent) || 0, 0, 100);
      const thresholdMode = meta.threshold_mode === 'remaining' ? 'remaining' : 'percent';
      const warnGB = Number(meta.warn_remaining_gb) || 0;
      const criticalGB = Number(meta.critical_remaining_gb) || 0;
      const warnPct = Number(meta.warn_percent) || 80;
      const criticalPct = Number(meta.critical_percent) || 95;
      const critical = thresholdMode === 'remaining' ? remaining <= criticalGB : pct >= criticalPct;
      const warning = thresholdMode === 'remaining' ? remaining <= warnGB : pct >= warnPct;
      const state = meta.traffic_state === 'critical' ? ['red', 'بحرانی'] : meta.traffic_state === 'warning' ? ['amber', 'هشدار'] : ['green', 'عادی'];
      const power = meta.suspended ? 'معلق (Suspend)' : String(meta.status) === '1' ? 'روشن' : 'خاموش';
      const thresholdText = thresholdMode === 'remaining' ? `هشدار: زیر ${nf1.format(warnGB)} GB باقی‌مانده` : `هشدار: از ${nf1.format(warnPct)}٪ مصرف`;
      return `<div class="detail-chart">
        <div class="detail-chart-head"><b>${icon('server')} ترافیک و وضعیت Virtualizor</b><span><span class="chip ${power === 'روشن' ? 'green' : 'red'}">${esc(power)}</span> <span class="chip ${state[0]}">${state[1]}</span> <span class="chip violet">${thresholdMode === 'remaining' ? 'آستانه: باقی‌مانده' : 'آستانه: درصد'}</span></span></div>
        <div class="detail-stats">
          <div class="mini-stat"><span>ترافیک مصرفی</span><b>${nf1.format(used)} GB${limit > 0 ? ` از ${num(limit)} GB` : ''}</b></div>
          <div class="mini-stat"><span>ترافیک باقی‌مانده</span><b style="color:${critical ? 'var(--danger-text)' : warning ? 'var(--warning-text)' : 'var(--success-text)'}">${limit > 0 ? `${nf1.format(remaining)} GB` : 'نامحدود'}</b></div>
          <div class="mini-stat"><span>دریافت / ارسال</span><b style="font-size:11px">${bytes(meta.net_in || 0)} / ${bytes(meta.net_out || 0)}</b></div>
          <div class="mini-stat"><span>VPS ID</span><b>${esc(String(meta.vps_id || '—'))}</b></div>
        </div>
        <div class="node-health"><div class="node-health-bar"><span style="width:${pct}%;background:${critical ? '#fb7185' : warning ? '#fbbf24' : '#34d399'}"></span></div><span>${limit > 0 ? `${nf1.format(pct)}٪ مصرف · ${thresholdText}` : 'سهمیهٔ ترافیک نامحدود است'}</span></div>
      </div>`;
    })() : ''}
    <div class="detail-chart">
      <div class="detail-chart-head"><b>${icon('info')} مشخصات</b><span>${last ? dateTime(last.at) : ''}</span></div>
      <div class="form-grid">
        ${configRows.map(([k, v]) => `<div class="kv"><span>${k}</span><b>${esc(v)}</b></div>`).join('')}
      </div>
      ${Object.keys(cfg).length ? `<details style="margin-top:10px"><summary style="font-size:9px;color:var(--faint);cursor:pointer">نمایش پیکربندی بررسی</summary><pre class="meta-block">${esc(JSON.stringify(cfg, null, 2))}</pre></details>` : ''}
      ${last?.meta && Object.keys(last.meta).length ? `<details style="margin-top:8px"><summary style="font-size:9px;color:var(--faint);cursor:pointer">جزئیات فنی آخرین بررسی</summary><pre class="meta-block">${esc(JSON.stringify(last.meta, null, 2))}</pre></details>` : ''}
    </div>
    <div class="detail-chart">
      <div class="detail-chart-head"><b>${icon('alert')} رخدادها</b><span>${num(incidents.length)} رخداد</span></div>
      <div class="incident-mini">
        ${incidents.length ? incidents.slice(0, 5).map((i) => `<div class="incident-mini-row ${i.ended_at ? 'resolved' : ''}"><span class="idot"></span><span style="flex:1">${esc(i.cause || 'رخداد سرویس')}</span><span>${dateShort(i.started_at)}</span><span>${i.ended_at ? duration(i.duration_sec) : 'در جریان'}</span></div>`).join('') : '<div class="small faint">رخدادی برای این مانیتور ثبت نشده است.</div>'}
      </div>
    </div>`;
  openModal({
    title: m.name,
    subtitle: `${typeMeta(m.type).label} · ${m.target}`,
    body,
    size: 'lg',
    footer: `<div class="left"><button class="btn btn-danger btn-sm" type="button" data-action="delete-monitor" data-id="${m.id}">${icon('trash')} حذف</button></div><button class="btn btn-ghost btn-sm" type="button" data-action="close-detail" data-id="${m.id}">بستن</button><button class="btn btn-primary btn-sm" type="button" data-action="edit-monitor" data-id="${m.id}">${icon('edit')} ویرایش مانیتور</button>`,
    onClose: () => {
      if (location.hash.includes(`/monitors/${m.id}`)) history.replaceState(null, '', '#/monitors');
    },
  });
  if (state.modal) Object.assign(state.modal, { kind: 'monitor', monitorId: Number(m.id) });
  setTimeout(() => {
    const canvas = $('#detailLatency');
    if (chartEmpty || !canvas || state.modal?.kind !== 'monitor' || Number(state.modal?.monitorId) !== Number(m.id)) return;
    lineChart(canvas, {
      series: [{ name: 'تأخیر', color: '#38bdf8', points: latency, fill: true }],
      yFormat: (v) => num(v) + 'ms',
      tipFormat: (v) => num(v) + ' ms',
    });
  }, 40);
}

/* ------------------------------------------------------------------ incidents */
function renderIncidents() {
  const items = state.incidents || [];
  const active = items.filter((i) => !i.ended_at);
  const resolved = items.filter((i) => i.ended_at);
  const dayAgo = Date.now() / 1000 - 86400;
  const today = items.filter((i) => i.started_at >= dayAgo);
  const downtime = today.reduce((s, i) => s + (i.ended_at ? i.duration_sec : Math.max(0, Date.now() / 1000 - i.started_at)), 0);
  const filter = state.incidentFilter;
  const shown = filter === 'active' ? active : filter === 'resolved' ? resolved : items;
  $('#view').innerHTML = `
    <div class="stat-grid" style="margin-top:0">
      <div class="panel stat-card"><div class="stat-top"><span class="stat-label">رخدادهای باز</span><span class="stat-ico red">${icon('alert')}</span></div><div class="stat-value">${num(active.length)}<small>در جریان</small></div><div class="stat-foot">${active.length ? 'نیازمند بررسی' : 'همه سرویس‌ها پایدارند'}</div></div>
      <div class="panel stat-card"><div class="stat-top"><span class="stat-label">رخداد 24 ساعت</span><span class="stat-ico amber">${icon('clock')}</span></div><div class="stat-value">${num(today.length)}<small>رخداد</small></div><div class="stat-foot">شامل ${num(today.filter((i) => !i.ended_at).length)} مورد باز</div></div>
      <div class="panel stat-card"><div class="stat-top"><span class="stat-label">زمان قطعی 24 ساعت</span><span class="stat-ico violet">${icon('activity')}</span></div><div class="stat-value" style="font-size:20px">${duration(downtime)}</div><div class="stat-foot">مجموع مدت رخدادها</div></div>
      <div class="panel stat-card"><div class="stat-top"><span class="stat-label">تاریخچه کل</span><span class="stat-ico green">${icon('check')}</span></div><div class="stat-value">${num(resolved.length)}<small>بازیابی</small></div><div class="stat-foot">از ابتدای پایش</div></div>
    </div>
    <div class="section-head">
      <div><h2><span class="dot-mark"></span> تاریخچه رخدادها</h2><p>${active.length ? `${num(active.length)} رخداد هنوز باز است` : 'رخداد بازی وجود ندارد'}</p></div>
      <div class="head-actions"><div class="segmented">
        ${[['all', 'همه'], ['active', 'باز'], ['resolved', 'بسته']].map(([k, l]) => `<button type="button" data-incident-filter="${k}" class="${filter === k ? 'active' : ''}">${l}</button>`).join('')}
      </div></div>
    </div>
    <div class="panel panel-pad">
      ${shown.length ? `<div class="incident-list">${shown.map((i) => incidentRow(i, i.ended_at === 0, true)).join('')}</div>` : `<div class="empty"><div class="empty-ico">${icon('shield')}</div><h3>رخدادی در این دسته نیست</h3><p>وقتی وضعیت سرویس‌ها تغییر کند، اینجا با جزئیات ثبت می‌شود.</p></div>`}
    </div>`;
}

/* ------------------------------------------------------------------ logs */
const LOG_CATEGORY_LABELS = {
  all: 'All', panel: 'Panel', scheduler: 'Scheduler', monitor: 'Monitor',
  xray: 'Xray', cloudflare: 'Cloudflare', notify: 'Notifications', agent: 'Agent',
  security: 'Security', api: 'API',
};
const LOG_LEVEL_LABELS = { all: 'All', debug: 'Debug', info: 'Info', warn: 'Warning', error: 'Error' };

function logCategoryLabel(category) { return LOG_CATEGORY_LABELS[category] || category || '—'; }
function logLevelLabel(level) { return LOG_LEVEL_LABELS[level] || level || '—'; }
function stopLogsPoll() {
  if (state.logTimer) { clearInterval(state.logTimer); state.logTimer = null; }
}
function startLogsPoll() {
  stopLogsPoll();
  if (!state.logAutoRefresh) return;
  state.logTimer = setInterval(() => {
    if (state.view === 'logs' && !state.modal) refreshLogsList().catch(() => {});
  }, 4000);
}
async function loadLogsData({ reset = false } = {}) {
  if (state.view !== 'logs') return;
  if (reset) {
    state.logCategory = state.routeSub && LOG_CATEGORY_LABELS[state.routeSub] ? state.routeSub : 'all';
    state.logLevel = 'all';
    state.logQuery = '';
  }
  renderLogs();
  bindLogFilters();
  await refreshLogsList();
  startLogsPoll();
}
async function refreshLogsList() {
  if (state.view !== 'logs') return;
  const params = new URLSearchParams({ limit: '200' });
  if (state.logCategory && state.logCategory !== 'all') params.set('category', state.logCategory);
  if (state.logLevel && state.logLevel !== 'all') params.set('level', state.logLevel);
  if (state.logQuery.trim()) params.set('q', state.logQuery.trim());
  const data = await api(`/api/logs?${params.toString()}`);
  state.logs = data.logs || [];
  const list = $('#log-list');
  if (list) {
    const open = new Set([...list.querySelectorAll('.log-detail[open]')].map((d) => d.closest('.log-row')?.dataset.logId).filter(Boolean));
    list.innerHTML = logsListHTML();
    for (const id of open) {
      const detail = list.querySelector(`.log-row[data-log-id="${id}"] .log-detail`);
      if (detail) detail.open = true;
    }
  }
  const count = $('[data-log-count]');
  if (count) count.textContent = `${num(state.logs.length)} events`;
}
function logsListHTML() {
  if (!state.logs.length) {
    return `<div class="empty" style="padding:36px 16px"><div class="empty-ico" style="width:46px;height:46px">${icon('terminal')}</div><h3 style="font-size:12px">No logs in this category</h3><p style="font-size:10px">Change the filters or wait for new events to appear here.</p></div>`;
  }
  return state.logs.map((entry) => {
    let detail = entry.detail || '';
    if (detail) {
      try { detail = JSON.stringify(JSON.parse(detail), null, 2); } catch { /* keep text */ }
    }
    return `<article class="log-row level-${esc(entry.level)}" data-log-id="${entry.id}">
      <div class="log-row-head">
        <span class="log-level">${esc(logLevelLabel(entry.level))}</span>
        <span class="log-category">${esc(logCategoryLabel(entry.category))}</span>
        <span class="log-time">${dateTime(entry.at)}</span>
      </div>
      <div class="log-message">${esc(entry.message)}</div>
      ${detail ? `<details class="log-detail"><summary>Technical details</summary><pre>${esc(detail)}</pre></details>` : ''}
    </article>`;
  }).join('');
}
function renderLogs() {
  const categories = Object.keys(LOG_CATEGORY_LABELS).map((key) => `<option value="${key}" ${state.logCategory === key ? 'selected' : ''}>${LOG_CATEGORY_LABELS[key]}</option>`).join('');
  const levels = Object.keys(LOG_LEVEL_LABELS).map((key) => `<option value="${key}" ${state.logLevel === key ? 'selected' : ''}>${LOG_LEVEL_LABELS[key]}</option>`).join('');
  $('#view').innerHTML = `
    <div class="page-toolbar log-toolbar">
      <div class="search-box">${icon('search')}<input id="log-search" placeholder="Search messages or details…" value="${esc(state.logQuery)}" autocomplete="off"></div>
      <select class="select" id="log-category" aria-label="Log category">${categories}</select>
      <select class="select" id="log-level" aria-label="Log level">${levels}</select>
      <div class="toolbar-spacer"></div>
      <div class="page-summary"><span class="badge-dot"><span class="dot"></span><span data-log-count>${num(state.logs.length)} events</span></span></div>
      <label class="log-auto-refresh"><span>Auto refresh</span><span class="switch"><input id="log-auto-refresh" type="checkbox" ${state.logAutoRefresh ? 'checked' : ''}><span></span></span></label>
      <button class="btn btn-sm btn-soft" type="button" data-action="refresh-logs">${icon('refresh')} Refresh</button>
    </div>
    <div class="banner">
      <div class="banner-ico">${icon('terminal')}</div>
      <div class="banner-body"><b>Pulse structured logs</b><p>Panel, scheduler, monitor, Xray, Cloudflare, notifications, agent, security and API events are kept in separate categories. Logs can be refreshed manually or automatically every 4 seconds.</p></div>
    </div>
    <div class="log-list" id="log-list">${logsListHTML()}</div>`;
}
function bindLogFilters() {
  const search = $('#log-search');
  if (search) {
    let timer = null;
    search.addEventListener('input', () => {
      clearTimeout(timer);
      timer = setTimeout(() => { state.logQuery = search.value; refreshLogsList().catch(() => {}); }, 350);
    });
  }
  $('#log-category')?.addEventListener('change', (e) => {
    state.logCategory = e.target.value;
    history.replaceState(null, '', state.logCategory === 'all' ? '#/logs' : `#/logs/${state.logCategory}`);
    state.routeSub = state.logCategory;
    refreshLogsList().catch(() => {});
  });
  $('#log-level')?.addEventListener('change', (e) => {
    state.logLevel = e.target.value;
    refreshLogsList().catch(() => {});
  });
  $('#log-auto-refresh')?.addEventListener('change', (e) => {
    state.logAutoRefresh = e.target.checked;
    try { localStorage.setItem('pulse-logs-auto', state.logAutoRefresh ? '1' : '0'); } catch { /* private mode */ }
    startLogsPoll();
    if (state.logAutoRefresh) refreshLogsList().catch(() => {});
  });
}

/* ------------------------------------------------------------------ servers */
function agentTransport(a) {
  if (a.transport === 'gRPC') return 'gRPC';
  if (a.transport === 'WebSocket') return 'WebSocket';
  if (a.connected || statusFromAgent(a)) return 'REST';
  return '';
}
function transportChipClass(a) {
  return a.transport === 'gRPC' ? 'violet' : a.transport === 'WebSocket' ? 'cyan' : '';
}
function agentCard(a) {
  const q = agentQuality(a);
  const transport = agentTransport(a);
  const services = Array.isArray(a.services) ? a.services : [];
  const wg = Array.isArray(a.wireguard) ? a.wireguard : [];
  const serviceOnline = services.filter((s) => s.active).length;
  return `<article class="panel agent-card" data-action="open-agent" data-id="${a.id}" role="button" tabindex="0">
    <div class="agent-head">
      <div class="agent-ico agent-ico-pulse">${icon('agent')}</div>
      <div class="agent-title"><div class="agent-name">${esc(a.name)}</div><div class="agent-host" data-agent-host title="IP عمومی خروجی؛ ممکن است با IP اصلی سرور متفاوت باشد."><span data-agent-ip-host>${esc(a.ip || 'بدون IP')}</span><span data-agent-location data-ip="${esc(a.ip || '')}"></span></div></div>
      <span class="agent-online ${q.online ? '' : 'off'}" data-agent-online><span class="dot"></span>${q.online ? 'آنلاین' : 'آفلاین'}</span>
    </div>
    <div class="agent-connection">
      <span class="agent-quality ${q.cls}" data-agent-quality>${icon('signal')} ${q.text}</span>
      <span class="chip ${transportChipClass(a)}" data-agent-transport style="${transport ? '' : 'display:none'}">${transport || 'REST'}</span>
      <span class="agent-ip" data-agent-ip>${esc(a.ip || 'بدون IP')}</span>
    </div>
    <div class="gauges">
      ${gaugeRing(a.cpu_pct || 0, gaugeColor(a.cpu_pct || 0), 'CPU', 'cpu')}
      ${gaugeRing(a.ram_pct || 0, gaugeColor(a.ram_pct || 0), 'RAM', 'ram')}
      ${gaugeRing(a.disk_pct || 0, gaugeColor(a.disk_pct || 0), 'دیسک', 'disk')}
    </div>
    <div class="agent-telemetry">
      <div class="telemetry-item" data-agent-load><b>${nf1.format(a.load1 || 0)}</b><span>Load 1m</span></div>
      <div class="telemetry-item" data-agent-ram><b class="ltr" style="unicode-bidi:embed">${a.ram_total ? bytes(a.ram_total) : '—'}</b><span>حافظه</span></div>
      <div class="telemetry-item" data-agent-net><b style="direction:ltr">${a.net_rx_bps ? bytes(a.net_rx_bps) + '/s' : '—'}</b><span>دریافت</span></div>
    </div>
    ${services.length ? `<div class="service-chips">${services.slice(0, 5).map((s) => `<span class="service-chip ${s.active ? '' : 'off'}"><span class="dot"></span>${esc(s.name)}</span>`).join('')}${services.length > 5 ? `<span class="service-chip">+${num(services.length - 5)}</span>` : ''}</div>` : ''}
    <div class="agent-foot"><span data-agent-foot>${wg.length ? `${icon('link')} ${num(wg.length)} اینترفیس WireGuard` : services.length ? `${icon('sliders')} ${num(serviceOnline)} از ${num(services.length)} سرویس فعال` : q.online ? `ارتباط ${transport || 'زنده'} برقرار است` : 'آخرین گزارش ' + timeAgo(a.last_seen)}</span><span data-agent-version>${a.version ? 'v' + fa(a.version) : ''} · ${timeAgo(a.last_seen)}</span></div>
  </article>`;
}

function updateAgentCard(a) {
  const card = document.querySelector(`.agent-card[data-id="${Number(a.id)}"]`);
  if (!card) return false;
  const q = agentQuality(a);
  const onlineEl = card.querySelector('[data-agent-online]');
  if (onlineEl) {
    onlineEl.classList.toggle('off', !q.online);
    onlineEl.innerHTML = `<span class="dot"></span>${q.online ? 'آنلاین' : 'آفلاین'}`;
  }
  const qualityEl = card.querySelector('[data-agent-quality]');
  if (qualityEl) {
    qualityEl.className = `agent-quality ${q.cls}`;
    qualityEl.innerHTML = `${icon('signal')} ${q.text}`;
  }
  const ipEl = card.querySelector('[data-agent-ip]');
  if (ipEl) ipEl.textContent = a.ip || 'بدون IP';
  const transport = agentTransport(a);
  const transportEl = card.querySelector('[data-agent-transport]');
  if (transportEl) {
    transportEl.textContent = transport || 'REST';
    transportEl.className = `chip ${transportChipClass(a)}`;
    transportEl.style.display = transport ? '' : 'none';
  }
  const hostEl = card.querySelector('[data-agent-ip-host]');
  if (hostEl) hostEl.textContent = a.ip || 'بدون IP';
  const locationEl = card.querySelector('[data-agent-location]');
  if (locationEl) {
    const nextIP = a.ip || '';
    if (locationEl.dataset.ip !== nextIP) {
      locationEl.dataset.ip = nextIP;
      locationEl.textContent = '';
    }
    hydrateAgentLocations(locationEl.parentElement || card);
  }
  [['cpu', a.cpu_pct], ['ram', a.ram_pct], ['disk', a.disk_pct]].forEach(([key, raw]) => {
    updateGauge(card, key, raw);
  });
  const load = card.querySelector('[data-agent-load] b');
  if (load) load.textContent = nf1.format(a.load1 || 0);
  const ram = card.querySelector('[data-agent-ram] b');
  if (ram) ram.textContent = a.ram_total ? bytes(a.ram_total) : '—';
  const net = card.querySelector('[data-agent-net] b');
  if (net) net.textContent = a.net_rx_bps ? bytes(a.net_rx_bps) + '/s' : '—';
  const services = Array.isArray(a.services) ? a.services : [];
  const wg = Array.isArray(a.wireguard) ? a.wireguard : [];
  const foot = card.querySelector('[data-agent-foot]');
  if (foot) {
    foot.innerHTML = wg.length
      ? `${icon('link')} ${num(wg.length)} اینترفیس WireGuard`
      : services.length
        ? `${icon('sliders')} ${num(services.filter((s) => s.active).length)} از ${num(services.length)} سرویس فعال`
        : q.online ? `ارتباط ${transport || 'زنده'} برقرار است` : 'آخرین گزارش ' + timeAgo(a.last_seen);
  }
  const version = card.querySelector('[data-agent-version]');
  if (version) version.textContent = `${a.version ? 'v' + fa(a.version) : ''} · ${timeAgo(a.last_seen)}`;
  return true;
}

function updateGauge(root, key, raw) {
  const unit = root.querySelector(`[data-agent-gauge="${key}"]`);
  if (!unit) return;
  const p = clamp(Number(raw) || 0, 0, 100);
  const c = 2 * Math.PI * 30;
  const val = unit.querySelector('circle.val');
  if (val) {
    val.setAttribute('stroke', gaugeColor(p));
    val.setAttribute('stroke-dashoffset', (c - (p / 100) * c).toFixed(2));
  }
  const txt = unit.querySelector('.gauge > span');
  if (txt) txt.textContent = `${num(p)}٪`;
}

function updateAgentDetailLive(a) {
  const modal = $('.modal');
  if (!modal) return false;
  const q = agentQuality(a);
  const online = modal.querySelector('[data-agent-detail-online]');
  if (online) {
    online.classList.toggle('off', !q.online);
    online.innerHTML = `<span class="dot"></span>${q.online ? 'آنلاین' : 'آفلاین'}`;
  }
  const quality = modal.querySelector('[data-agent-detail-quality]');
  if (quality) {
    quality.className = `agent-quality ${q.cls}`;
    quality.innerHTML = `${icon('signal')} ${q.online ? `${agentTransport(a) || 'زنده'} متصل` : 'قطع ارتباط'}`;
  }
  const transportEl = modal.querySelector('[data-agent-detail-transport]');
  if (transportEl) {
    const transport = agentTransport(a);
    transportEl.textContent = transport || 'REST';
    transportEl.className = `chip ${transportChipClass(a)}`;
  }
  const xrayEl = modal.querySelector('[data-agent-detail-xray]');
  if (xrayEl) {
    xrayEl.textContent = a.can_check
      ? (a.xray_available ? `Xray ${a.xray_version || ''}`.trim() : 'بدون Xray')
      : q.online ? 'نیازمند اتصال زنده' : 'آفلاین';
  }
  const seenInline = modal.querySelector('[data-agent-detail-seen-inline]');
  if (seenInline) seenInline.textContent = `آخرین گزارش: ${timeAgo(a.last_seen)}`;
  [['cpu', a.cpu_pct], ['ram', a.ram_pct], ['disk', a.disk_pct]].forEach(([key, raw]) => updateGauge(modal, key, raw));
  const values = {
    load: nf1.format(a.load1 || 0),
    uptime: duration(a.uptime_sec || 0),
    ram: a.ram_total ? bytes(a.ram_total) : '—',
    disk: a.disk_total ? bytes(a.disk_total) : '—',
    rx: `${bytes(a.net_rx_bps || 0)}/s`,
    tx: `${bytes(a.net_tx_bps || 0)}/s`,
    seen: timeAgo(a.last_seen),
    version: a.version ? 'v' + fa(a.version) : '—',
  };
  Object.entries(values).forEach(([key, value]) => {
    const el = modal.querySelector(`[data-agent-detail-${key}]`);
    if (el) el.textContent = value;
  });
  return true;
}
function renderServers() {
  const agents = state.agents || [];
  const online = agents.filter(statusFromAgent).length;
  $('#view').innerHTML = `
    <div class="page-toolbar">
      <div class="page-summary" data-agent-summary>${agentSummaryHTML(agents)}</div>
      <div class="toolbar-spacer"></div>
      <button class="btn btn-primary" type="button" data-action="new-agent">${icon('plus')} افزودن سرور</button>
    </div>
    ${agents.length ? `<div class="agent-grid">${agents.slice().sort((a, b) => Number(statusFromAgent(b)) - Number(statusFromAgent(a)) || a.name.localeCompare(b.name, 'fa')).map(agentCard).join('')}</div>` : `<div class="panel empty" style="margin-top:8px">
      <div class="empty-ico">${icon('agent')}</div><h3>منابع سرورها را زنده ببینید</h3><p>یک ایجنت سبک روی سرور نصب کنید تا CPU، RAM، دیسک، وضعیت سرویس‌ها و WireGuard هر 1 ثانیه از طریق اتصال زنده به پالس گزارش شود.</p>
      <div style="display:flex;gap:8px;flex-wrap:wrap;justify-content:center;margin-top:6px"><button class="btn btn-primary btn-sm" type="button" data-action="new-agent">${icon('plus')} ساخت ایجنت</button><button class="btn btn-sm" type="button" data-action="agent-guide">${icon('terminal')} نصب یک‌خطی</button></div>
    </div>`}
    <div class="banner" style="margin-top:16px">
      <div class="banner-ico">${icon('signal')}</div>
      <div class="banner-body"><b>اتصال زندهٔ ایجنت</b><p>ایجنت ابتدا gRPC و در صورت نبود HTTP/2 از WebSocket استفاده می‌کند؛ در نهایت هم REST به‌عنوان fallback فعال می‌ماند. نصب با یک دستور و به‌روزرسانی با <code style="font-family:var(--mono);font-size:9px;color:var(--link-text)">pulse-a update</code>.</p></div>
    </div>`;
  hydrateAgentLocations($('#view'));
}

/* ------------------------------------------------------------------ rules page */
function ruleChannelChips(channels) {
  const map = { all: 'همه کانال‌ها', telegram: 'تلگرام', webpush: 'پوش مرورگر', webhook: 'وب‌هوک' };
  return (channels || []).map((c) => `<span class="chip ${c === 'all' ? 'cyan' : c === 'telegram' ? 'violet' : 'green'}">${icon(c === 'webhook' ? 'webhook' : c === 'webpush' ? 'smartphone' : 'send')} ${map[c] || c}</span>`).join('');
}
function renderRules() {
  const rules = state.rules || [];
  const monitorName = (id) => state.monitors.find((m) => m.id === id)?.name;
  $('#view').innerHTML = `
    <div class="banner">
      <div class="banner-ico">${icon('sliders')}</div>
      <div class="banner-body"><b>موتور قوانین، قلب هوشمند پالس</b><p>هر قانون تعیین می‌کند چه رویدادی، چه زمانی و از طریق کدام کانال اطلاع داده شود. کانال‌ها را در صفحه <a href="#/notifications" style="color:var(--link-text)">اعلان‌ها</a> پیکربندی کنید.</p></div>
      <button class="btn btn-primary btn-sm" type="button" data-action="new-rule">${icon('plus')} قانون جدید</button>
    </div>
    <div class="section-head"><div><h2><span class="dot-mark"></span> قوانین فعال</h2><p>${num(rules.filter((r) => r.enabled).length)} قانون فعال از ${num(rules.length)}</p></div><div class="head-actions"><span class="chip">${icon('clock')} زمان‌بندی هوشمند</span></div></div>
    ${rules.length ? `<div class="rule-grid">${rules.slice().sort((a, b) => b.enabled - a.enabled || a.id - b.id).map((r) => {
      const t = TRIGGERS[r.trigger] || { label: r.trigger, icon: 'sliders', tone: '' };
      const scope = r.monitor_id ? (monitorName(r.monitor_id) || 'مانیتور حذف‌شده') : 'همه مانیتورها';
      return `<article class="panel rule-card ${r.enabled ? '' : 'disabled'}">
        <div class="rule-head">
          <div class="rule-ico ${t.tone}">${icon(t.icon)}</div>
          <div class="rule-title"><b>${esc(cleanRuleName(r.name))}</b><span>${t.label} · ${esc(scope)}</span></div>
          <label class="switch" title="${r.enabled ? 'غیرفعال کردن' : 'فعال کردن'}"><input type="checkbox" data-rule-toggle="${r.id}" aria-label="${r.enabled ? 'غیرفعال کردن' : 'فعال کردن'} ${esc(cleanRuleName(r.name))}" ${r.enabled ? 'checked' : ''}><span></span></label>
        </div>
        <div class="rule-desc">${t.desc}</div>
        <div class="rule-meta">
          <span class="chip">${icon('target')} ${esc(scope)}</span>
          ${r.threshold > 0 ? `<span class="chip amber">${icon('chart')} آستانه ${num(r.threshold)}${r.trigger === 'cert_expiring' ? ' روز' : '٪'}</span>` : ''}
          <span class="chip">${icon('clock')} ${r.cooldown_sec ? 'هر ' + duration(r.cooldown_sec) : 'بدون محدودیت'}</span>
        </div>
        <div class="service-chips">${ruleChannelChips(r.channels)}</div>
        <div class="rule-foot"><span class="tiny-label">${r.last_fired_at ? 'آخرین اجرا: ' + timeAgo(r.last_fired_at) : 'هنوز اجرا نشده'}</span><div class="rule-actions"><button class="btn btn-sm btn-ghost" type="button" data-action="edit-rule" data-id="${r.id}">${icon('edit')} ویرایش</button><button class="btn btn-sm btn-ghost" type="button" data-action="delete-rule" data-id="${r.id}" style="color:var(--danger-text)" aria-label="حذف قانون ${esc(r.name)}">${icon('trash')}</button></div></div>
      </article>`;
    }).join('')}</div>` : `<div class="panel empty"><div class="empty-ico">${icon('sliders')}</div><h3>هنوز قانونی نساخته‌اید</h3><p>برای دریافت هشدار قطعی، انقضای SSL یا مصرف بالای منابع، یک قانون بسازید.</p><button class="btn btn-primary btn-sm" type="button" data-action="new-rule">${icon('plus')} ساخت قانون</button></div>`}`;
}

/* ------------------------------------------------------------------ after render */
function drawSpark(canvas) {
  const raw = canvas.dataset.points || '';
  const data = raw ? raw.split(',').map(Number).filter((v) => Number.isFinite(v)) : [];
  sparkline(canvas, data, canvas.dataset.color || '#38bdf8');
}
let labelSeq = 0;
function wireLabels(root = document) {
  root.querySelectorAll('.field > label').forEach((label) => {
    if (label.htmlFor) return;
    const field = label.parentElement;
    const input = field?.querySelector('input:not([type="hidden"]):not([type="radio"]):not([type="checkbox"]), select, textarea');
    if (!input) return;
    if (!input.id) input.id = `pulse-field-${++labelSeq}`;
    label.htmlFor = input.id;
  });
}
function afterRender() {
  wireLabels(document);
  $$('.spark').forEach(drawSpark);
  if ($('#trendChart') && state.summary) {
    const trend = (state.summary.trend || []).filter((p) => p.total > 0);
    if (trend.length >= 2) {
      lineChart($('#trendChart'), {
        series: [{ name: 'تأخیر', color: '#38bdf8', points: trend.map((p) => ({ x: p.at, y: p.avg_latency_ms || 0 })), fill: true }],
        yFormat: (v) => num(v) + 'ms',
        tipFormat: (v) => num(v) + ' ms',
      });
    }
  }
}

/* ------------------------------------------------------------------ cloudflare */
function cfAccountQuery(accountId = state.cfAccount) {
  const id = Number(accountId) || 0;
  return id > 0 ? `?account_id=${encodeURIComponent(id)}` : '';
}
async function loadCloudflareData() {
  const seq = ++state.cfZoneSeq;
  const [status, monitors] = await Promise.all([
    api(`/api/cloudflare/status${cfAccountQuery()}`),
    api('/api/monitors'),
  ]);
  if (seq !== state.cfZoneSeq) return;
  state.cfStatus = status;
  state.cfAccounts = status.accounts || [];
  if (!state.cfAccount || !state.cfAccounts.some((a) => Number(a.id) === Number(state.cfAccount))) {
    const selected = Number(status.account_id) || 0;
    state.cfAccount = state.cfAccounts.some((a) => Number(a.id) === selected)
      ? selected
      : (Number(state.cfAccounts[0]?.id) || 0);
  }
  state.cfMonitors = monitors.monitors || [];
  state.cfZones = [];
  state.cfRecords = [];
  state.cfError = '';
  state.cfLoading = !!status.connected;
  // Paint the account panel immediately; zones and records are network-bound.
  renderCloudflare();
  if (status.connected) {
    try {
      const zones = await api(`/api/cloudflare/zones${cfAccountQuery()}`, { timeout: 35000 });
      if (seq !== state.cfZoneSeq) return;
      state.cfZones = zones.zones || [];
      if (state.cfZones.length) {
        if (!state.cfZones.some((z) => z.id === state.cfZone)) state.cfZone = state.cfZones[0].id;
        const records = await api(`/api/cloudflare/records${cfAccountQuery()}${state.cfAccount ? '&' : '?'}zone_id=${encodeURIComponent(state.cfZone)}`, { timeout: 35000 });
        if (seq !== state.cfZoneSeq) return;
        state.cfRecords = records.records || [];
      }
    } catch (e) {
      state.cfError = e.message;
    }
  }
  state.cfLoading = false;
  renderCloudflare();
}
async function fetchCFRecords(zoneId, accountId = state.cfAccount) {
  if (!zoneId) return [];
  const q = Number(accountId) > 0 ? `account_id=${encodeURIComponent(accountId)}&` : '';
  const data = await api(`/api/cloudflare/records?${q}zone_id=${encodeURIComponent(zoneId)}`, { timeout: 35000 });
  return data.records || [];
}
function cfRecordsFiltered() {
  const q = state.cfSearch.trim().toLowerCase();
  return state.cfRecords.filter((r) => {
    if (state.cfRecordType && r.type !== state.cfRecordType) return false;
    if (!q) return true;
    return `${r.name} ${r.content} ${r.type}`.toLowerCase().includes(q);
  });
}
function cfRecordRows() {
  const list = cfRecordsFiltered();
  if (!list.length) return `<tr><td colspan="5" style="text-align:center;padding:24px;color:var(--faint);font-size:10px">رکوردی پیدا نشد</td></tr>`;
  return list.slice(0, 300).map((r) => `<tr>
    <td><span class="type-badge">${esc(r.type)}</span></td>
    <td class="record-name">${esc(r.name)}</td>
    <td class="record-content">${esc(r.content)}</td>
    <td>${r.proxied ? '<span class="chip amber">Proxied</span>' : '<span class="chip">DNS Only</span>'}${r.priority ? ` <span class="chip">${num(r.priority)}</span>` : ''}</td>
    <td style="text-align:left;white-space:nowrap"><button class="btn btn-sm btn-ghost" type="button" data-action="cf-edit-record" data-id="${esc(r.id)}">${icon('edit')} تغییر</button><button class="btn btn-sm btn-ghost" type="button" data-action="cf-delete-record" data-id="${esc(r.id)}" data-name="${esc(r.name)}" style="color:var(--danger-text)">${icon('trash')}</button></td>
  </tr>`).join('');
}
function renderCloudflare() {
  const status = state.cfStatus || {};
  const accounts = state.cfAccounts || [];
  const configured = accounts.length > 0;
  const connected = !!status.connected;
  const enabled = !!status.enabled;
  const zones = state.cfZones || [];
  const activeAccount = accounts.find((a) => Number(a.id) === Number(state.cfAccount)) || accounts[0] || null;
  const autoMonitors = state.cfMonitors.filter((m) => m.cf_enabled);
  const stateLabel = connected && enabled ? 'متصل و فعال' : connected ? 'متصل · اتوماسیون خاموش' : configured ? 'اتصال ناموفق' : 'تنظیم نشده';
  const accountRows = accounts.map((a) => `<tr class="${Number(a.id) === Number(state.cfAccount) ? 'cf-account-active' : ''}">
    <td><button class="btn btn-sm btn-ghost" type="button" data-action="cf-select-account" data-id="${a.id}" style="width:100%;justify-content:flex-start">${icon('cloud')} <b>${esc(a.name)}</b></button></td>
    <td><span class="chip ${a.enabled ? 'green' : 'red'}">${a.enabled ? 'فعال' : 'غیرفعال'}</span></td>
    <td style="text-align:left;white-space:nowrap"><button class="btn btn-sm btn-ghost" type="button" data-action="cf-edit-account" data-id="${a.id}">${icon('edit')} ویرایش</button><button class="btn btn-sm btn-ghost" type="button" data-action="cf-delete-account" data-id="${a.id}" data-name="${esc(a.name)}" style="color:var(--danger-text)">${icon('trash')}</button></td>
  </tr>`).join('');
  const accountPanel = configured ? `<div class="panel panel-pad" style="margin-top:16px">
    <div class="chart-head"><div><div class="chart-title">${icon('key')} حساب‌های Cloudflare</div><div class="chart-sub">هر حساب توکن و دامنه‌های مستقل خودش را دارد.</div></div><button class="btn btn-sm btn-primary" type="button" data-action="cf-add-account">${icon('plus')} حساب جدید</button></div>
    <div class="table-wrap"><table class="record-table"><thead><tr><th>حساب</th><th>وضعیت</th><th></th></tr></thead><tbody>${accountRows}</tbody></table></div>
    ${!connected && status.error ? `<div class="warning-note" style="margin-top:10px">${icon('alert')} <div><b>${esc(activeAccount?.name || 'حساب انتخاب‌شده')}:</b> ${esc(status.error)}</div></div>` : ''}
  </div>` : `<div class="panel panel-pad" style="margin-top:16px">
    <div class="chart-head"><div><div class="chart-title">${icon('key')} اتصال امن با API Token</div><div class="chart-sub">برای شروع، یک حساب Cloudflare اضافه کنید. بعداً می‌توانید چند حساب بسازید.</div></div></div>
    <div class="warning-note">${icon('info')} <div>توکن را از داشبورد Cloudflare → My Profile → API Tokens بسازید. دسترسی‌های لازم: <b>Zone:Read</b> و <b>DNS:Edit</b>.</div></div>
    <button class="btn btn-primary btn-sm" type="button" data-action="cf-add-account" style="margin-top:12px">${icon('plus')} افزودن حساب Cloudflare</button>
  </div>`;

  $('#view').innerHTML = `
    <section class="panel cf-hero">
      <div class="cf-logo">${icon('cloud')}</div>
      <div class="cf-copy"><h2>${connected ? (enabled ? 'کلادفلر متصل است' : 'کلادفلر متصل است، اتوماسیون خاموش') : configured ? 'اتصال کلادفلر برقرار نیست' : 'سوییچ خودکار DNS'}</h2>
        <p>${connected ? (enabled ? 'پالس می‌تواند رکوردهای A/AAAA را در زمان قطعی سرویس، به IP پشتیبان منتقل کند و پس از بازیابی برگرداند.' : 'اتصال توکن سالم است اما تغییر خودکار DNS غیرفعال شده است. از بخش مدیریت، دوباره فعال کنید.') : configured ? 'حساب انتخاب‌شده در دسترس نیست. توکن یا وضعیت حساب را بررسی کنید.' : 'با اتصال API، ترمیم خودکار DNS را فعال کنید تا قطعی‌ها بدون دخالت شما مدیریت شوند.'}</p>
      </div>
      <div class="cf-status">${connected ? `<span class="status-pill ${enabled ? 'up' : 'paused'}"><span class="sdot"></span>${stateLabel}</span><span class="chip">${num(zones.length)} دامنه</span>` : configured ? `<span class="status-pill down"><span class="sdot"></span>اتصال ناموفق</span><button class="btn btn-sm" type="button" data-action="cf-refresh">${icon('refresh')} بررسی مجدد</button>` : `<span class="status-pill paused"><span class="sdot"></span>تنظیم نشده</span>`}</div>
    </section>
    ${state.cfError ? `<div class="warning-note">${icon('alert')} <div>${esc(state.cfError)}</div></div>` : ''}
    ${accountPanel}
    ${connected ? `
      <div class="cf-grid" style="margin-top:16px">
        <div class="panel panel-pad">
          <div class="chart-head"><div><div class="chart-title">${icon('network')} رکوردهای DNS ${activeAccount ? `· ${esc(activeAccount.name)}` : ''}</div><div class="chart-sub">ساخت، ویرایش یا حذف رکورد در حساب انتخاب‌شده</div></div><div style="display:flex;gap:6px;flex-wrap:wrap"><button class="btn btn-sm btn-primary" type="button" data-action="cf-add-record">${icon('plus')} رکورد جدید</button><button class="btn btn-sm" type="button" data-action="cf-refresh">${icon('refresh')} بروزرسانی</button></div></div>
          <div class="cf-zone-bar">
            <div class="field"><label>دامنه / Zone</label><select class="select" id="cf-zone">${zones.map((z) => `<option value="${esc(z.id)}" ${z.id === state.cfZone ? 'selected' : ''}>${esc(z.name)}</option>`).join('') || '<option value="">دامنه‌ای پیدا نشد</option>'}</select></div>
            <div class="field"><label>نوع</label><select class="select" id="cf-record-type"><option value="">همه</option>${['A', 'AAAA', 'CNAME', 'TXT', 'MX'].map((t) => `<option value="${t}" ${state.cfRecordType === t ? 'selected' : ''}>${t}</option>`).join('')}</select></div>
            <div class="field"><label>جستجو</label><input class="input" id="cf-search" value="${esc(state.cfSearch)}" placeholder="نام یا IP…"></div>
          </div>
          ${state.cfLoading ? `<div class="loading-grid">${Array.from({ length: 2 }).map(() => '<div class="panel loading-card skeleton"></div>').join('')}</div>` : `<div class="table-wrap"><table class="record-table"><thead><tr><th>نوع</th><th>نام</th><th>مقدار</th><th>حالت</th><th></th></tr></thead><tbody id="cf-records-body">${cfRecordRows()}</tbody></table></div>
          ${zones.length ? '' : `<div class="empty" style="padding:20px"><p>هیچ Zone در این حساب Cloudflare پیدا نشد.</p></div>`}`}
        </div>
        <div class="cf-side">
          <div class="panel auto-switch-card">
            <div class="chart-head" style="margin-bottom:0"><div><div class="chart-title">${icon('bolt')} ${enabled ? 'سوییچ خودکار فعال' : 'سوییچ خودکار خاموش'}</div><div class="chart-sub">مانیتورهایی با IP پشتیبان</div></div><span class="chip ${enabled && autoMonitors.length ? 'green' : enabled ? '' : 'amber'}">${num(autoMonitors.length)}</span></div>
            <div class="auto-switch-list">
              ${autoMonitors.length ? autoMonitors.slice(0, 6).map((m) => `<div class="auto-switch-row"><span class="status-pill ${m.status}" style="height:22px;padding:0 7px"><span class="sdot"></span></span><div class="mini-info"><b>${esc(m.name)}</b><span>${esc(m.cf_record_name || m.target)} · ${esc(m.cf_primary || '—')} ← ${esc(m.cf_failover || '—')}</span>${m.cf_last_action ? `<span style="color:var(--faint);font-size:8px">${esc(m.cf_last_action)}</span>` : ''}</div><a class="btn btn-sm btn-ghost" href="#/monitors/${m.id}">${icon('chevron')}</a></div>`).join('') : `<div class="help" style="padding:10px 0">هنوز هیچ مانیتوری برای سوییچ خودکار پیکربندی نشده است. هنگام ویرایش مانیتور، بخش «سوییچ خودکار DNS» را فعال کنید.</div>`}
            </div>
          </div>
          <div class="panel panel-pad">
            <div class="chart-title" style="margin-bottom:8px">${icon('shield')} تغییر امن</div>
            <div class="kv"><span>رکوردها</span><b>${num(state.cfRecords.length)}</b></div>
            <div class="kv"><span>آخرین بررسی اتصال</span><b>${connected ? 'همین حالا' : '—'}</b></div>
            <div class="warning-note">${icon('alert')} <div>پالس قبل از سوییچ، مقدار رکورد را با IP اصلی مقایسه می‌کند تا تغییرات دستی شما را بازنویسی نکند.</div></div>
          </div>
        </div>
      </div>
      <div class="panel panel-pad" style="margin-top:16px;display:flex;gap:10px;align-items:center;flex-wrap:wrap"><span class="chart-title">${icon('key')} مدیریت اتصال</span><span class="help" style="flex:1">${enabled ? 'اتوماسیون DNS فعال است.' : 'اتوماسیون DNS خاموش است؛ می‌توانید بدون تغییر توکن دوباره فعالش کنید.'}</span>${enabled ? `<button class="btn btn-sm btn-danger" type="button" data-action="cf-disable">${icon('pause')} غیرفعال‌سازی اتوماسیون</button>` : `<button class="btn btn-sm btn-primary" type="button" data-action="cf-enable">${icon('play')} فعال‌سازی اتوماسیون</button>`}<button class="btn btn-sm" type="button" data-action="cf-edit-account" data-id="${activeAccount?.id || ''}">${icon('key')} تغییر توکن</button></div>
    ` : ''}`;
  wireLabels($('#view'));
}

/* ------------------------------------------------------------------ notifications */
async function loadNotificationsData() {
  const [settingsData, pushConfig, notif] = await Promise.all([
    api('/api/settings'),
    api('/api/push/config'),
    api('/api/notifications?limit=50'),
  ]);
  state.settingsData = settingsData;
  state.pushConfig = pushConfig;
  state.notifications = notif.notifications || [];
  state.pushSubscribed = false;
  try {
    if ('serviceWorker' in navigator) {
      const reg = await navigator.serviceWorker.getRegistration();
      const sub = reg ? await reg.pushManager.getSubscription() : null;
      state.pushSubscribed = !!sub;
    }
  } catch { /* ignore */ }
  renderNotifications();
}

function captureNotificationDrafts() {
  const drafts = {};
  $$('#view form[data-form]').forEach((form) => {
    const fields = {};
    Array.from(form.elements).forEach((el) => {
      if (!el.name) return;
      fields[el.name] = { value: el.value, checked: !!el.checked };
    });
    drafts[form.dataset.form] = fields;
  });
  return drafts;
}

function restoreNotificationDrafts(drafts, except = '') {
  Object.entries(drafts || {}).forEach(([formName, fields]) => {
    if (formName === except) return;
    const form = $(`#view form[data-form="${formName}"]`);
    if (!form) return;
    Object.entries(fields).forEach(([name, field]) => {
      const el = form.elements.namedItem(name);
      if (!el) return;
      try { el.value = field.value; el.checked = field.checked; } catch { /* ignore */ }
    });
  });
}

async function refreshNotificationsPreserve(except = '') {
  const drafts = captureNotificationDrafts();
  await loadNotificationsData();
  restoreNotificationDrafts(drafts, except);
}

async function reloadSettingsPreserve(except = '') {
  const drafts = captureNotificationDrafts();
  await loadView('settings', null, true);
  restoreNotificationDrafts(drafts, except);
}
function renderNotifications() {
  const data = state.settingsData || {};
  const s = data.settings || {};
  const sys = data.system || {};
  const push = state.pushConfig || {};
  const isIOS = /iPad|iPhone|iPod/.test(navigator.userAgent);
  const standalone = window.navigator.standalone === true || window.matchMedia('(display-mode: standalone)').matches;
  const telegramOn = !!s.telegram_enabled;
  const telegramConfigured = !!data.telegram_configured;
  const webhookConfigured = !!data.webhook_configured;
  const pushCount = push.subscriptions || 0;
  $('#view').innerHTML = `
    <div class="banner"><div class="banner-ico">${icon('bell')}</div><div class="banner-body"><b>هوشمند خبردار شوید</b><p>اعلان‌ها بر اساس <a href="#/rules" style="color:var(--link-text)">قوانین</a> ارسال می‌شوند؛ قطعی‌ها، بازیابی، انقضای SSL، منابع بالا و اختلاف DNS. ساعات سکوت را هم می‌توانید تنظیم کنید.</p></div></div>
    <div class="notif-grid">
      <form class="panel channel-card" data-form="telegram">
        <div class="channel-head"><div class="channel-ico tg">${icon('send')}</div><div class="channel-title"><b>تلگرام</b><span>پیام مستقیم از ربات</span></div><span class="channel-status ${telegramOn && telegramConfigured ? 'on' : ''}"></span></div>
        <div class="channel-body">
          <div class="switch-row"><div class="switch-text"><b>ارسال به تلگرام</b><span>${telegramConfigured ? 'توکن ذخیره شده است' : 'توکن ربات را وارد کنید'}</span></div><label class="switch"><input type="checkbox" name="enabled" aria-label="فعال‌سازی ارسال به تلگرام" ${telegramOn ? 'checked' : ''}><span></span></label></div>
          <div class="field"><label>توکن ربات</label><input class="input ltr" type="password" name="token" autocomplete="off" placeholder="${telegramConfigured ? '•••••••• (برای تغییر وارد کنید)' : '123456789:AA...' }"></div>
          <div class="field"><label>Chat ID</label><input class="input ltr" type="text" name="chat_id" value="${esc(s.telegram_chat_id || '')}" placeholder="مثلاً 123456789 یا @channelname"></div>
          <div class="help">ربات را از <b>@BotFather</b> بسازید و برای گرفتن Chat ID، به ربات پیام دهید و از <b>@userinfobot</b> استفاده کنید. برای کانال، ربات را ادمین کنید.</div>
          <div class="form-error"></div>
        </div>
        <div class="channel-foot"><button class="btn btn-sm btn-soft" type="submit">${icon('check')} ذخیره</button><div style="display:flex;gap:4px"><button class="btn btn-sm btn-ghost" type="button" data-action="telegram-test">${icon('send')} تست</button>${telegramConfigured ? `<button class="btn btn-sm btn-ghost" type="button" data-action="clear-telegram" style="color:var(--danger-text)" aria-label="قطع اتصال تلگرام">${icon('trash')}</button>` : ''}</div></div>
      </form>

      <form class="panel channel-card" data-form="push-settings">
        <div class="channel-head"><div class="channel-ico push">${icon('smartphone')}</div><div class="channel-title"><b>Web Push</b><span>اعلان مرورگر و PWA</span></div><span class="channel-status ${s.webpush_enabled && state.pushSubscribed ? 'on' : ''}"></span></div>
        <div class="channel-body">
          <div class="switch-row"><div class="switch-text"><b>اعلان مرورگر</b><span>${num(pushCount)} اشتراک ثبت شده</span></div><label class="switch"><input type="checkbox" name="enabled" aria-label="فعال‌سازی اعلان مرورگر" ${s.webpush_enabled ? 'checked' : ''}><span></span></label></div>
          <div class="kv"><span>وضعیت این مرورگر</span><b>${state.pushSubscribed ? 'ثبت شده' : 'ثبت نشده'}</b></div>
          <div class="kv"><span>پشتیبانی</span><b>${'Notification' in window ? 'بله' : 'خیر'}</b></div>
          <div class="kv"><span>کلید عمومی VAPID</span><b class="ltr" style="font-size:8px;max-width:120px;overflow:hidden;text-overflow:ellipsis">${esc((push.public_key || '').slice(0, 18))}…</b></div>
          ${isIOS && !standalone ? `<div class="warning-note">${icon('info')} <div>در iPhone ابتدا پالس را از Safari به صفحه اصلی اضافه کنید (Share → Add to Home Screen)، سپس از داخل اپ اعلان را فعال کنید.</div></div>` : ''}
          <div class="form-error"></div>
        </div>
        <div class="channel-foot"><button class="btn btn-sm btn-soft" type="submit">${icon('check')} ذخیره</button><div style="display:flex;gap:4px">${state.pushSubscribed ? `<button class="btn btn-sm btn-ghost" type="button" data-action="push-disable">لغو این مرورگر</button>` : `<button class="btn btn-sm btn-soft" type="button" data-action="push-enable">${icon('bell')} فعال‌سازی</button>`}<button class="btn btn-sm btn-ghost" type="button" data-action="push-test">تست</button></div></div>
      </form>

      <form class="panel channel-card" data-form="webhook">
        <div class="channel-head"><div class="channel-ico wh">${icon('webhook')}</div><div class="channel-title"><b>وب‌هوک</b><span>اتصال به سیستم‌های دیگر</span></div><span class="channel-status ${webhookConfigured ? 'on' : ''}"></span></div>
        <div class="channel-body">
          <div class="field"><label>URL مقصد</label><input class="input ltr" type="url" name="url" value="${esc(s.webhook_url || '')}" placeholder="https://example.com/hooks/pulse"></div>
          <div class="field"><label>کلید امضا (اختیاری)</label><input class="input ltr" type="password" name="secret" autocomplete="off" placeholder="${data.webhook_configured ? 'ذخیره شده — برای تغییر وارد کنید' : 'Secret برای HMAC-SHA256'}"></div>
          <div class="help">در هر رویداد، یک JSON به آدرس شما POST می‌شود. هدر <b>X-Pulse-Signature</b> با HMAC-SHA256 امضا می‌شود.</div>
          <div class="form-error"></div>
        </div>
        <div class="channel-foot"><button class="btn btn-sm btn-soft" type="submit">${icon('check')} ذخیره</button><div style="display:flex;gap:4px"><button class="btn btn-sm btn-ghost" type="button" data-action="webhook-test">${icon('send')} تست</button>${webhookConfigured ? `<button class="btn btn-sm btn-ghost" type="button" data-action="clear-webhook" style="color:var(--danger-text)" aria-label="حذف وب‌هوک">${icon('trash')}</button>` : ''}</div></div>
      </form>
    </div>

    <form class="panel quiet-card" data-form="quiet-hours">
      <div class="chart-head"><div><div class="chart-title">${icon('clock')} ساعات سکوت و تکرار هشدار</div><div class="chart-sub">اعلان‌های غیرحیاتی در این بازه ارسال نمی‌شوند.</div></div><label class="switch"><input type="checkbox" name="enabled" aria-label="فعال‌سازی ساعات سکوت" ${s.quiet_hours_enabled ? 'checked' : ''}><span></span></label></div>
      <div class="quiet-grid">
        <div class="field"><label>شروع سکوت</label><input class="input ltr" type="text" inputmode="numeric" pattern="[0-9]{2}:[0-9]{2}" name="start" value="${esc(s.quiet_start || '00:00')}" placeholder="HH:MM"></div>
        <div class="field"><label>پایان سکوت</label><input class="input ltr" type="text" inputmode="numeric" pattern="[0-9]{2}:[0-9]{2}" name="end" value="${esc(s.quiet_end || '07:00')}" placeholder="HH:MM"></div>
        <div class="field"><label>تکرار هشدار قطعی (دقیقه)</label><input class="input" type="number" min="0" max="10080" name="repeat" value="${Number(s.notify_repeat_min) || 0}"><div class="help">0 = بدون تکرار</div></div>
        <div class="field full"><label class="check-row"><input type="checkbox" name="critical" ${s.quiet_allow_critical ? 'checked' : ''}> هشدارهای حیاتی (قطعی) در ساعات سکوت هم ارسال شوند</label></div>
      </div>
      <div style="display:flex;justify-content:flex-end;margin-top:10px"><button class="btn btn-primary btn-sm" type="submit">${icon('check')} ذخیره تنظیمات</button></div>
      <div class="form-error"></div>
    </form>

    <form class="panel settings-card" data-form="notify-template" style="margin-top:16px">
      <h2>${icon('send')} قالب اعلان سفارشی <span class="chip violet" style="margin-inline-start:auto">Template</span></h2>
      <div class="settings-desc">همین قالب برای تلگرام، Web Push و بدنهٔ وب‌هوک استفاده می‌شود. جای‌نگهدارها با اطلاعات واقعی رخداد جایگزین می‌شوند.</div>
      <div class="form-grid">
        <div class="field full"><label>عنوان اعلان</label><input class="input ltr" name="title_template" value="${esc(s.notify_title_template || DEFAULT_NOTIFY_TITLE)}" placeholder="{emoji} {title}"></div>
        <div class="field full"><label>متن اعلان</label><textarea class="textarea ltr" name="body_template" rows="8" spellcheck="false">${esc(s.notify_body_template || DEFAULT_NOTIFY_BODY)}</textarea></div>
      </div>
      <div class="service-chips" style="margin-top:2px">${NOTIFY_PLACEHOLDERS.map((p) => `<button class="chip" type="button" data-action="notify-placeholder" data-value="${esc(p)}" style="cursor:pointer">${esc(p)}</button>`).join('')}</div>
      <div class="tiny-label" style="margin-top:6px">پیش‌نمایش</div>
      <pre class="code-block" id="notify-template-preview" style="max-height:160px"></pre>
      <div class="form-error"></div>
      <div style="display:flex;justify-content:flex-end;gap:6px"><button class="btn btn-sm btn-ghost" type="button" data-action="notify-template-reset">بازگردانی پیش‌فرض</button><button class="btn btn-sm btn-primary" type="submit">${icon('check')} ذخیره قالب</button></div>
    </form>

    <div class="section-head"><div><h2><span class="dot-mark"></span> سابقه ارسال</h2><p>آخرین ${num(state.notifications.length)} اعلان ثبت‌شده</p></div><a class="btn btn-sm btn-ghost" href="#/rules">مدیریت قوانین ${icon('chevron')}</a></div>
    <div class="panel" style="padding:8px 14px">
      ${state.notifications.length ? `<div class="table-wrap"><table class="log-table"><thead><tr><th>وضعیت</th><th>عنوان</th><th>کانال</th><th>قانون</th><th>زمان</th></tr></thead><tbody>
        ${state.notifications.map((n) => `<tr><td><span class="${n.ok ? 'ok-dot' : 'fail-dot'}" style="width:6px;height:6px;border-radius:50%;display:inline-block;margin-inline-end:5px;background:${n.ok ? '#34d399' : '#fb7185'}"></span>${n.ok ? 'موفق' : 'ناموفق'}</td><td class="log-title">${esc(n.title || '—')}${!n.ok && n.error ? `<div style="font-size:8.5px;color:var(--danger-text);margin-top:2px">${esc(n.error)}</div>` : ''}</td><td>${esc(n.channel)}</td><td>${esc(n.rule_name || '—')}</td><td style="white-space:nowrap">${timeAgo(n.at)}</td></tr>`).join('')}
      </tbody></table></div>` : `<div class="empty" style="padding:24px"><div class="empty-ico" style="width:46px;height:46px">${icon('bell')}</div><h3>هنوز اعلانی ارسال نشده</h3><p>با فعال‌سازی کانال‌ها و وقوع رویداد، سابقه اینجا نمایش داده می‌شود.</p></div>`}
    </div>`;
  wireLabels($('#view'));
  bindNotifyTemplateEditor();
}

function notifyTemplatePreview(form) {
  if (!form) return;
  const titleEl = form.elements.namedItem('title_template');
  const bodyEl = form.elements.namedItem('body_template');
  const preview = $('#notify-template-preview');
  if (!preview) return;
  const sample = {
    '{emoji}': '🟡', '{title}': 'هشدار سرویس: مثال', '{event}': 'degraded',
    '{monitor}': 'ساب اصلی', '{agent}': 'ساب اصلی', '{hostname}': 'vps-1',
    '{type}': 'پروکسی', '{target}': 'sub.example.com', '{status}': 'هشدار',
    '{reason}': '2 کانفیگ از 13 کانفیگ در دسترس نیست', '{details}': 'Germany - Tun: handshake timeout',
    '{failed_configs}': 'Germany - Tun: handshake timeout', '{message}': 'پیام کامل و اصلی رخداد', '{value}': '91.2', '{time}': '2026/11/27 - 13:30',
  };
  const render = (value) => String(value || '').replace(/\{[a-z_]+\}/g, (m) => sample[m] ?? m);
  const title = render(titleEl?.value || DEFAULT_NOTIFY_TITLE);
  const body = render(bodyEl?.value || DEFAULT_NOTIFY_BODY).replace(/<[^>]+>/g, '');
  preview.textContent = `${title}\n\n${body}`;
}

function bindNotifyTemplateEditor() {
  const form = $('#view form[data-form="notify-template"]');
  if (!form) return;
  const update = () => notifyTemplatePreview(form);
  form.addEventListener('input', update);
  update();
}

/* ------------------------------------------------------------------ settings */
function renderSettings() {
  const data = state.settingsData || {};
  const s = data.settings || {};
  const sys = data.system || {};
  const xr = data.xray || {};
  const listenAddr = String(sys.listen_addr || '');
  const currentPort = listenAddr.split(':').pop() || '';
  const initials = userNameInitial();
  const zones = ['Asia/Tehran', 'Asia/Dubai', 'Europe/London', 'Europe/Berlin', 'America/New_York', 'UTC'];
  const tab = state.settingsTab || 'general';
  const tabs = [['general', 'عمومی', 'sliders'], ['xray', 'Xray Core', 'layers'], ['system', 'سیستم', 'activity']];
  $('#view').innerHTML = `
    <div class="settings-tabs" role="tablist">
      ${tabs.map(([key, label, ico]) => `<button type="button" class="settings-tab ${tab === key ? 'active' : ''}" data-action="settings-tab" data-tab="${key}" role="tab" aria-selected="${tab === key}">${icon(ico)} ${label}</button>`).join('')}
    </div>
    <div class="settings-pane ${tab === 'general' ? '' : 'hidden'}" data-settings-pane="general">
      <div class="settings-grid">
        <div style="display:flex;flex-direction:column;gap:16px">
          <div class="panel settings-card">
            <h2>${icon('user')} حساب کاربری</h2>
            <div class="account-row"><div class="account-avatar">${initials}</div><div class="account-info"><b>${esc(state.auth?.username || 'admin')}</b><span>مدیر سامانه · نشست فعال</span></div><div style="margin-inline-start:auto;display:flex;gap:6px"><button class="btn btn-sm" type="button" data-action="change-password">${icon('key')} تغییر رمز</button><button class="btn btn-sm btn-danger" type="button" data-action="logout">${icon('logout')} خروج</button></div></div>
          </div>
          <form class="panel settings-card" data-form="settings-general">
            <h2>${icon('sliders')} تنظیمات عمومی</h2>
            <div class="settings-desc">پیکربندی پایه سامانه، دامنه عمومی و نگه‌داری داده‌ها.</div>
            <div class="form-grid">
              <div class="field"><label>عنوان سامانه</label><input class="input" name="site_title" value="${esc(s.site_title || 'پالس')}" required></div>
              <div class="field"><label>منطقه زمانی</label><input class="input ltr" name="timezone" list="tz-list" value="${esc(s.timezone || 'Asia/Tehran')}"><datalist id="tz-list">${zones.map((z) => `<option value="${z}">`).join('')}</datalist></div>
              <div class="field full"><label>آدرس عمومی سامانه</label><input class="input ltr" name="public_base_url" value="${esc(s.public_base_url || '')}" placeholder="https://status.example.com"><div class="help">برای Web Push و ساخت فرمان نصب ایجنت، HTTPS توصیه می‌شود.</div></div>
              <div class="field"><label>DNS Resolver</label><input class="input ltr" name="dns_resolver" value="${esc(s.dns_resolver || '1.1.1.1:53')}" placeholder="1.1.1.1:53"></div>
              <div class="field"><label>آستانه آفلاین ایجنت (ثانیه)</label><input class="input" type="number" min="60" max="3600" name="agent_offline_after_sec" value="${Number(s.agent_offline_after_sec) || 120}"></div>
              <div class="field"><label>نگه‌داری تاریخچه بررسی (روز)</label><input class="input" type="number" min="1" max="365" name="check_retention_days" value="${Number(s.check_retention_days) || 14}"></div>
              <div class="field"><label>نگه‌داری نمودار سرورها (روز)</label><input class="input" type="number" min="1" max="365" name="metrics_retention_days" value="${Number(s.metrics_retention_days) || 30}"></div>
              <div class="field full"><label>لینک دانلود ایجنت</label><input class="input ltr" name="agent_download_url" value="${esc(s.agent_download_url || '')}" placeholder="https://github.com/AmiwrM/Pulse/releases/latest/download/pulse-linux-amd64"><div class="help">پیش‌فرض از ریلیز رسمی همین ریپو برای معماری پنل استفاده می‌شود. برای ایجنت‌های ARM یا میرور، آدرس را تغییر دهید.</div></div>
            </div>
            <div class="form-error"></div>
            <div style="display:flex;justify-content:flex-end"><button class="btn btn-primary btn-sm" type="submit">${icon('check')} ذخیره تنظیمات</button></div>
          </form>
          <div class="panel settings-card">
            <h2>${icon('database')} نسخه پشتیبان</h2>
            <div class="settings-desc">خروجی JSON از مانیتورها، قوانین، ایجنت‌ها و تنظیمات (بدون توکن‌ها و رمزها).</div>
            <div class="backup-box">${icon('database')}<div><b>پشتیبان کامل پیکربندی</b><span>ذخیره روی سیستم شما</span></div><button class="btn btn-sm" type="button" data-action="download-backup">${icon('download')} دانلود</button></div>
            ${state.installPrompt ? `<div class="backup-box">${icon('smartphone')}<div><b>نصب پالس روی دستگاه</b><span>دسترسی سریع و اعلان پایدار</span></div><button class="btn btn-sm btn-soft" type="button" data-action="install-pwa">${icon('download')} نصب</button></div>` : ''}
          </div>
        </div>
        <div style="display:flex;flex-direction:column;gap:16px">
          <div class="panel settings-card">
            <h2>${icon('shield')} امنیت و سلامت</h2>
            <div class="kv"><span>HTTPS</span><b>${requestSecure() ? 'فعال' : (location.hostname === 'localhost' || location.hostname === '127.0.0.1' ? 'لوکال (HTTP)' : 'غیرفعال (HTTP)')}</b></div>
            <div class="kv"><span>Web Push</span><b>${data.push_subscriptions ? num(data.push_subscriptions) + ' مشترک' : 'ثبت نشده'}</b></div>
            <div class="kv"><span>کلادفلر</span><b>${data.cloudflare_configured ? 'پیکربندی شده' : 'متصل نیست'}</b></div>
            <div class="kv"><span>تلگرام</span><b>${data.telegram_configured ? 'پیکربندی شده' : 'متصل نیست'}</b></div>
            <div class="warning-note">${icon('shield')} <div>برای دسترسی از خارج، پنل را پشت HTTPS (Caddy/Nginx) قرار دهید و رمز قوی استفاده کنید.</div></div>
          </div>
        </div>
      </div>
    </div>
    <div class="settings-pane ${tab === 'xray' ? '' : 'hidden'}" data-settings-pane="xray">
      <div class="settings-grid" style="grid-template-columns:1fr">
        <div class="panel settings-card">
          <h2>${icon('layers')} هستهٔ Xray <span class="chip ${xr.available ? 'green' : 'amber'}" style="margin-inline-start:auto">${xr.available ? 'آماده' : 'شناسایی نشده'}</span></h2>
          <div class="settings-desc">برای تست واقعی کانفیگ‌ها، پالس یک هستهٔ موقت Xray با SOCKS محلی اجرا می‌کند و از داخل تونل یک درخواست HTTP واقعی می‌فرستد.</div>
          <div class="kv"><span>نسخه</span><b>${xr.version ? 'Xray ' + fa(xr.version) : '—'}</b></div>
          <div class="kv"><span>مسیر شناسایی‌شده</span><b style="font-size:8.5px;max-width:300px;overflow:hidden;text-overflow:ellipsis;direction:ltr">${esc(xr.path || '—')}</b></div>
          <div class="kv"><span>آخرین بررسی</span><b style="font-size:9px">${xr.checked_at ? timeAgo(xr.checked_at) : '—'}</b></div>
          ${xr.available ? '' : `<div class="warning-note">${icon('alert')} <div>${esc(xr.error || 'هستهٔ Xray پیدا نشد')} — نصب: <code>sudo bash install.sh --force-xray</code> یا مسیر باینری را وارد کنید.</div></div>`}
          <div class="service-chips"><span class="chip violet">VMess</span><span class="chip violet">VLESS</span><span class="chip violet">Trojan</span><span class="chip violet">Shadowsocks</span><span class="chip violet">WireGuard</span></div>
          <form data-form="xray-settings">
            <div class="form-error"></div>
            <div class="form-grid" style="margin-top:8px">
              <div class="field"><label>مسیر باینری Xray</label><input class="input ltr" name="binary" value="${esc(s.xray_binary || 'xray')}" placeholder="xray یا /usr/local/bin/xray"></div>
              <div class="field"><label>آدرس پیش‌فرض تست واقعی</label><input class="input ltr" name="test_url" value="${esc(s.xray_test_url || '')}" placeholder="https://www.gstatic.com/generate_204"></div>
            </div>
            <div style="display:flex;gap:6px;justify-content:flex-end;margin-top:9px;flex-wrap:wrap"><button class="btn btn-sm" type="button" data-action="xray-detect">${icon('refresh')} بررسی مجدد</button><button class="btn btn-sm btn-primary" type="submit">${icon('check')} ذخیره</button></div>
          </form>
          <div class="help">نسخهٔ تست‌شده: Xray v26.11.9. نصب‌کننده یا Docker آن را نصب می‌کند؛ برای به‌روزرسانی دوباره install.sh را با <code>--force-xray</code> اجرا کنید.</div>
        </div>
      </div>
    </div>
    <div class="settings-pane ${tab === 'system' ? '' : 'hidden'}" data-settings-pane="system">
      <div class="panel settings-card" style="margin-bottom:16px">
        <h2>${icon('terminal')} پورت پنل و حالت پشت پروکسی <span class="chip violet" style="margin-inline-start:auto">Advanced</span></h2>
        <div class="settings-desc">در حالت عادی، Caddy خودکار پنل را روی 443 منتشر می‌کند. اگر خودتان Nginx/Caddy/Cloudflare دارید، نصب با <code>--behind-proxy</code> پنل را فقط روی <code>127.0.0.1</code> بالا می‌آورد و به پورت 443 و Xray دست نمی‌زند. تغییر پورت زیر همان تنظیمات Caddy را هم به‌روزرسانی می‌کند.</div>
        <form data-form="system-port">
          <div class="form-grid">
            <div class="field"><label>پورت فعلی</label><input class="input ltr" value="${esc(currentPort || '—')}" disabled></div>
            <div class="field"><label>پورت جدید</label><input class="input ltr" id="panel-port-input" type="number" min="1" max="65535" name="port" value="${esc(currentPort || '8080')}" required></div>
          </div>
          <div class="warning-note">${icon('alert')} <div>با تغییر پورت، پنل چند لحظه قطع می‌شود. اگر پشت پروکسی خارجی هستید، پورت مقصد همان پروکسی را هم به‌روزرسانی کنید. در Docker پورت را از <code>docker-compose.yml</code> تغییر دهید. در نصب‌های قدیمی بدون واحد <code>pulse-port.path</code>، یک‌بار <code>install.sh</code> را اجرا کنید یا از دستور زیر استفاده کنید.</div></div>
          <div class="help" style="display:flex;gap:8px;align-items:center;flex-wrap:wrap"><code class="ltr" id="panel-port-command" style="direction:ltr">sudo pulse port ${esc(currentPort || '8080')}</code><button class="btn btn-sm btn-ghost" type="button" data-action="copy" data-copy-target="panel-port-command">${icon('copy')} کپی دستور</button><button class="btn btn-sm btn-primary" type="submit">${icon('check')} اعمال پورت</button></div>
          <div class="form-error"></div>
        </form>
      </div>
      <div class="settings-grid">
        <div class="panel settings-card">
          <h2>${icon('activity')} وضعیت سامانه</h2>
          <div class="sys-grid">
            <div class="sys-item"><span>نسخه پالس</span><b>${esc(sys.version || '1.9.0')}</b></div>
            <div class="sys-item"><span>زمان فعال بودن</span><b>${duration(sys.uptime_sec || 0)}</b></div>
            <div class="sys-item"><span>حجم پایگاه داده</span><b>${bytes(sys.db_bytes || 0)}</b></div>
            <div class="sys-item"><span>مانیتورها</span><b>${num(sys.monitors || 0)}</b></div>
            <div class="sys-item"><span>بررسی‌های ذخیره‌شده</span><b>${num(sys.checks || 0)}</b></div>
            <div class="sys-item"><span>ایگنت‌ها</span><b>${num(sys.agents || 0)}</b></div>
            <div class="sys-item"><span>گو</span><b>${esc(sys.go_version || '—')}</b></div>
            <div class="sys-item"><span>پلتفرم</span><b>${esc((sys.os || '') + '/' + (sys.arch || ''))}</b></div>
          </div>
          <div class="divider"></div>
          <div class="tiny-label">آخرین فعالیت‌ها</div>
          <div style="display:flex;flex-direction:column;gap:2px">
            ${(state.audit || []).length ? state.audit.map((a) => `<div class="kv"><span style="font-size:9px">${esc(a.action)} · ${esc(a.actor)}</span><b style="font-size:8.5px;color:var(--faint)">${timeAgo(a.at)}</b></div>`).join('') : '<div class="help">فعالیتی ثبت نشده است.</div>'}
          </div>
        </div>
      </div>
    </div>`;
}
function requestSecure() { return location.protocol === 'https:'; }

/* ------------------------------------------------------------------ monitor form */
function formVal(form, name) {
  const el = form.elements.namedItem(name);
  return el ? String(el.value ?? '').trim() : '';
}
function formChecked(form, name) {
  const el = form.elements.namedItem(name);
  return !!(el && el.checked);
}
function monitorTypeFields(type, cfg = {}) {
  const val = (k, d = '') => cfg[k] === undefined || cfg[k] === null ? d : cfg[k];
  const checked = (k) => !!cfg[k];
  if (type === 'http') {
    return `<div class="field"><label>متد درخواست</label><select class="select" name="method">${['GET', 'HEAD', 'POST', 'PUT'].map((m) => `<option value="${m}" ${val('method', 'GET') === m ? 'selected' : ''}>${m}</option>`).join('')}</select></div>
      <div class="field"><label>کد وضعیت مورد انتظار</label><input class="input ltr" name="expected_status" value="${esc(val('expected_status', '2xx'))}" placeholder="2xx یا 200"><div class="help">مثال: 200، 2xx، 200-299</div></div>
      <div class="field"><label>کلیدواژه در پاسخ (اختیاری)</label><input class="input" name="keyword" value="${esc(val('keyword'))}" placeholder="مثلاً: welcome"></div>
      <div class="field"><label>رفتار کلیدواژه</label><select class="select" name="keyword_mode"><option value="contains" ${val('keyword_mode', 'contains') !== 'not_contains' ? 'selected' : ''}>باید وجود داشته باشد</option><option value="not_contains" ${val('keyword_mode') === 'not_contains' ? 'selected' : ''}>نباید وجود داشته باشد</option></select></div>
      <div class="field"><label>محل جستجوی کلیدواژه</label><select class="select" name="keyword_scope"><option value="body" ${val('keyword_scope', 'body') === 'body' ? 'selected' : ''}>بدنه پاسخ</option><option value="headers" ${val('keyword_scope') === 'headers' ? 'selected' : ''}>هدرهای پاسخ</option><option value="both" ${val('keyword_scope') === 'both' ? 'selected' : ''}>بدنه + هدرها</option></select><div class="help">برای ساب‌سکریپشن‌هایی که عنوان را در هدر برمی‌گردانند، «هدرهای پاسخ» را انتخاب کنید.</div></div>
      <div class="field"><label>آستانه کندی (میلی‌ثانیه)</label><input class="input" type="number" name="slow_ms" min="0" value="${Number(val('slow_ms', 0)) || ''}" placeholder="اختیاری"></div>
      <div class="field"><label>Host Header (اختیاری)</label><input class="input ltr" name="host_header" value="${esc(val('host_header'))}" placeholder="example.com"></div>
      <div class="field full"><label>هدرهای سفارشی (JSON)</label><textarea class="textarea ltr" name="headers" rows="2" placeholder='{"Authorization": "Bearer ..."}'>${Object.keys(cfg.headers || {}).length ? esc(JSON.stringify(cfg.headers, null, 2)) : ''}</textarea></div>
      <div class="field full"><label>بدنه درخواست (اختیاری)</label><textarea class="textarea ltr" name="body" rows="2">${esc(val('body'))}</textarea></div>
      <div class="field full"><label class="check-row"><input type="checkbox" name="follow_redirects" ${checked('follow_redirects') ? 'checked' : ''}> دنبال کردن ریدایرکت‌ها</label><label class="check-row"><input type="checkbox" name="skip_tls_verify" ${checked('skip_tls_verify') ? 'checked' : ''}> نادیده گرفتن خطای گواهی TLS</label></div>`;
  }
  if (type === 'dns') {
    return `<div class="field"><label>نوع رکورد</label><select class="select" name="record">${['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS'].map((r) => `<option value="${r}" ${val('record', 'A') === r ? 'selected' : ''}>${r}</option>`).join('')}</select></div>
      <div class="field"><label>مقدار مورد انتظار (اختیاری)</label><input class="input ltr" name="expect" value="${esc(val('expect'))}" placeholder="1.2.3.4 یا mail.example.com"><div class="help">با کاما می‌توانید چند مقدار وارد کنید.</div></div>
      <div class="field full"><label>DNS Server (اختیاری)</label><input class="input ltr" name="server" value="${esc(val('server'))}" placeholder="1.1.1.1:53"><div class="help">خالی بگذارید تا Resolver پیش‌فرض سامانه استفاده شود.</div></div>
      <div class="field full"><div class="warning-note" style="margin:0">${icon('info')} <div>برای بررسی اختلاف DNS، مقدار مورد انتظار را وارد کنید. در صورت تفاوت، قانون «اختلاف رکورد DNS» اجرا می‌شود.</div></div></div>`;
  }
  if (type === 'ssl') {
    return `<div class="field"><label>Server Name / SNI (اختیاری)</label><input class="input ltr" name="server_name" value="${esc(val('server_name'))}" placeholder="example.com"></div>
      <div class="field"><label>هشدار چند روز قبل از انقضا</label><input class="input" type="number" min="1" max="365" name="warn_days" value="${Number(val('warn_days', 14)) || 14}"></div>
      <div class="field full"><label class="check-row"><input type="checkbox" name="skip_verify" ${checked('skip_verify') ? 'checked' : ''}> نادیده گرفتن اعتبار گواهی (فقط بررسی انقضا)</label></div>
      <div class="field full"><div class="help">اگر آدرس شامل پورت نیست، پورت 443 استفاده می‌شود. اعتبار گواهی، نام صادرکننده و تاریخ انقضا ذخیره می‌شود.</div></div>`;
  }
  if (type === 'proxy') {
    return `<div class="field full"><label>کانفیگ یا لینک ساب‌سکریپشن <span class="req">*</span></label><textarea class="textarea ltr" name="input" rows="5" placeholder="vmess://...&#10;vless://...&#10;https://example.com/subscription&#10;یا متن کامل WireGuard / Clash">${esc(val('input'))}</textarea><div class="help">پشتیبانی از vmess، vless، trojan، shadowsocks، hysteria2/tuic، کانفیگ WireGuard و ساب‌سکریپشن Base64. برای کانفیگ گروهی، تعداد کانفیگ‌های سالم بررسی می‌شود.</div></div>
      <div class="field"><label>حداقل کانفیگ سالم</label><input class="input" type="number" min="1" name="min_healthy" value="${Number(val('min_healthy', 1)) || 1}"></div>
      <div class="field"><label class="check-row" style="margin-top:25px"><input type="checkbox" name="skip_tls_verify" ${checked('skip_tls_verify') ? 'checked' : ''}> نادیده گرفتن گواهی در دریافت ساب</label></div>
      <div class="field full"><label class="check-row"><input type="checkbox" name="real_test" ${cfg.real_test === false ? '' : 'checked'}> هندشیک واقعی با هستهٔ Xray <span class="chip violet" style="height:18px;padding:0 6px">Real test</span></label><div class="help" id="xray-hint">در حال بررسی هستهٔ Xray…</div></div>
      <div class="field full"><label>آدرس تست اتصال (HTTP/HTTPS)</label><input class="input ltr" name="test_url" value="${esc(val('test_url'))}" placeholder="https://www.gstatic.com/generate_204"><div class="help">درخواست از داخل تونل به این آدرس زده می‌شود. باید از سمت سرور پروکسی قابل دسترس باشد و کد 2xx/3xx برگرداند.</div></div>
      <div class="field"><label>حداکثر کانفیگ برای تست واقعی</label><input class="input" type="number" min="1" max="30" name="max_real_nodes" value="${Number(val('max_real_nodes', 10)) || 10}"><div class="help">برای سابسکریپشن‌های بزرگ، فقط این تعداد کانفیگ با Xray هندشیک واقعی می‌شوند.</div></div>
      <div class="field full"><div class="service-chips"><span class="chip violet">VMess</span><span class="chip violet">VLESS</span><span class="chip violet">Trojan</span><span class="chip violet">Shadowsocks</span><span class="chip violet">WireGuard</span><span class="chip amber">Hysteria2/TUIC: فقط دسترسی</span></div></div>`;
  }
  if (type === 'vps') {
    const mode = val('threshold_mode', 'percent') === 'remaining' ? 'remaining' : 'percent';
    return `<div class="field full"><div class="warning-note" style="margin:0">${icon('server')} <div>این مانیتور با API ادمین <b>Virtualizor</b> وضعیت VPS، ترافیک ماهانه و suspend را می‌خواند. API Key و Password را از <b>Admin Panel → API Credentials → List API Pairs</b> بردار.</div></div></div>
      <div class="field full"><label>آدرس پنل Virtualizor</label><input class="input ltr" name="panel_url" value="${esc(val('panel_url'))}" placeholder="https://panel.example.com:4085" required></div>
      <div class="field"><label>API Key</label><input class="input ltr" name="api_key" value="${esc(val('api_key'))}" autocomplete="off" required></div>
      <div class="field"><label>API Password</label><input class="input ltr" type="password" name="api_pass" value="${esc(val('api_pass'))}" autocomplete="off" required></div>
      <div class="field"><label>VPS ID</label><input class="input" type="number" min="1" name="vps_id" value="${Number(val('vps_id', 0)) || ''}" required></div>
      <div class="field"><label>حالت آستانهٔ ترافیک</label><select class="select" name="threshold_mode" id="m-vps-threshold-mode"><option value="percent" ${mode === 'percent' ? 'selected' : ''}>درصد مصرف‌شده</option><option value="remaining" ${mode === 'remaining' ? 'selected' : ''}>گیگ باقی‌مانده</option></select></div>
      <div class="field vps-threshold-percent ${mode === 'percent' ? '' : 'hidden'}"><label>هشدار ترافیک (%)</label><input class="input" type="number" min="1" max="99" name="warn_percent" value="${Number(val('warn_percent', 80)) || 80}"><div class="help">وقتی درصد مصرف از این مقدار بگذرد.</div></div>
      <div class="field vps-threshold-percent ${mode === 'percent' ? '' : 'hidden'}"><label>بحرانی (%)</label><input class="input" type="number" min="2" max="100" name="critical_percent" value="${Number(val('critical_percent', 95)) || 95}"></div>
      <div class="field vps-threshold-remaining ${mode === 'remaining' ? '' : 'hidden'}"><label>هشدار: باقی‌مانده (GB)</label><input class="input" type="number" min="0.1" step="0.1" name="warn_remaining_gb" value="${Number(val('warn_remaining_gb', 10)) || 10}"><div class="help">وقتی ترافیک باقی‌مانده کمتر از این مقدار شود.</div></div>
      <div class="field vps-threshold-remaining ${mode === 'remaining' ? '' : 'hidden'}"><label>بحرانی: باقی‌مانده (GB)</label><input class="input" type="number" min="0.1" step="0.1" name="critical_remaining_gb" value="${Number(val('critical_remaining_gb', 2)) || 2}"></div>
      <div class="field full"><label class="check-row"><input type="checkbox" name="skip_tls_verify" ${checked('skip_tls_verify') ? 'checked' : ''}> نادیده گرفتن گواهی TLS پنل (self-signed)</label></div>`;
  }
  if (type === 'tcp') {
    return `<div class="field full"><div class="warning-note" style="margin:0">${icon('info')} <div>آدرس را به شکل <b class="ltr">host:port</b> وارد کنید. اتصال TCP برقرار می‌شود و تأخیر آن اندازه‌گیری می‌شود.</div></div></div>`;
  }
  return `<div class="field full"><div class="warning-note" style="margin:0">${icon('info')} <div>برای بررسی ICMP، سرور پالس باید دسترسی لازم را داشته باشد. در Docker گزینه <b class="ltr">cap_add: NET_RAW</b> لازم است. اگر ping نصب نباشد، بررسی خطا می‌دهد.</div></div></div>`;
}
function updateVPSThresholdFields(form) {
  if (!form) return;
  const mode = form.querySelector('#m-vps-threshold-mode')?.value || 'percent';
  $$('.vps-threshold-percent', form).forEach((el) => el.classList.toggle('hidden', mode !== 'percent'));
  $$('.vps-threshold-remaining', form).forEach((el) => el.classList.toggle('hidden', mode !== 'remaining'));
}
function targetMeta(type) {
  switch (type) {
    case 'http': return { label: 'آدرس وب‌سایت', placeholder: 'https://example.com/health', hint: 'آدرس کامل با http:// یا https://' };
    case 'tcp': return { label: 'آدرس پورت', placeholder: 'example.com:443', hint: 'به شکل host:port' };
    case 'ping': return { label: 'دامنه یا IP', placeholder: '1.1.1.1 یا example.com', hint: 'بدون پورت و پروتکل' };
    case 'dns': return { label: 'دامنه', placeholder: 'example.com', hint: 'نام دامنه برای استعلام DNS' };
    case 'ssl': return { label: 'دامنه / آدرس', placeholder: 'example.com', hint: 'پورت پیش‌فرض 443 است' };
    case 'proxy': return { label: 'نام نمایشی کانفیگ', placeholder: 'ساب اصلی — فرانکفورت', hint: 'فقط برای نمایش در داشبورد و اعلان‌ها' };
    case 'vps': return { label: 'نام نمایشی VPS', placeholder: 'مثلاً VPS آلمان', hint: 'اختیاری — اگر خالی بماند از شناسه VPS استفاده می‌شود.', optional: true };
  }
  return { label: 'هدف', placeholder: '', hint: '' };
}
async function ensureCFAccounts() {
  if (state.cfAccounts?.length) return state.cfAccounts;
  const data = await api('/api/cloudflare/accounts');
  state.cfAccounts = data.accounts || [];
  return state.cfAccounts;
}
async function ensureCFZones(accountId = state.cfAccount) {
  const id = Number(accountId) || 0;
  const data = await api(`/api/cloudflare/zones${id > 0 ? `?account_id=${encodeURIComponent(id)}` : ''}`, { timeout: 35000 });
  const zones = data.zones || [];
  if (!id || id === Number(state.cfAccount)) state.cfZones = zones;
  return zones;
}
async function populateCFRecordSelect(form, zoneId, selectedId = '', fallbackName = '', fallbackPrimary = '', accountId = 0) {
  const sel = form.querySelector('#m-cf-record');
  if (!sel) return;
  if (!zoneId) { sel.innerHTML = '<option value="">ابتدا Zone را انتخاب کنید</option>'; return; }
  sel.innerHTML = '<option value="">در حال دریافت رکوردها…</option>';
  const q = Number(accountId) > 0 ? `account_id=${encodeURIComponent(accountId)}&` : '';
  try {
    const data = await api(`/api/cloudflare/records?${q}zone_id=${encodeURIComponent(zoneId)}`);
    const records = (data.records || []).filter((r) => ['A', 'AAAA'].includes(r.type));
    let html = '<option value="">انتخاب رکورد…</option>';
    let found = false;
    for (const r of records) {
      if (r.id === selectedId) found = true;
      html += `<option value="${esc(r.id)}" data-name="${esc(r.name)}" data-content="${esc(r.content)}" ${r.id === selectedId ? 'selected' : ''}>${esc(r.name)} → ${esc(r.content)} (${esc(r.type)})</option>`;
    }
    if (selectedId && !found) {
      html += `<option value="${esc(selectedId)}" data-name="${esc(fallbackName)}" data-content="${esc(fallbackPrimary)}" selected>${esc(fallbackName || selectedId)} (رکورد فعلی)</option>`;
    }
    sel.innerHTML = html;
  } catch (e) {
    sel.innerHTML = `<option value="">${esc('خطا: ' + e.message)}</option>`;
  }
}
async function hydrateCFSelects(form, monitor) {
  const accountSel = form.querySelector('#m-cf-account');
  const zoneSel = form.querySelector('#m-cf-zone');
  if (!zoneSel) return;
  const note = form.querySelector('#m-cf-note');
  try {
    const accounts = await ensureCFAccounts();
    if (accountSel) {
      accountSel.innerHTML = accounts.length
        ? accounts.map((a) => `<option value="${a.id}" ${a.enabled ? '' : 'data-disabled="1"'}>${esc(a.name)}${a.enabled ? '' : ' · غیرفعال'}</option>`).join('')
        : '<option value="">حساب Cloudflare ثبت نشده</option>';
      const preferred = Number(monitor?.cf_account_id) || Number(state.cfAccount) || Number(accounts[0]?.id) || 0;
      if (preferred && accounts.some((a) => Number(a.id) === preferred)) accountSel.value = String(preferred);
    }
    const accountID = Number(accountSel?.value) || 0;
    const zones = accountID ? await ensureCFZones(accountID) : [];
    if (!zones.length) {
      zoneSel.innerHTML = '<option value="">دامنه‌ای پیدا نشد</option>';
      if (note) note.innerHTML = `برای استفاده از سوییچ خودکار ابتدا <a href="#/cloudflare" style="color:var(--link-text)">یک حساب کلادفلر و دامنه اضافه کنید</a>.`;
      return;
    }
    zoneSel.innerHTML = '<option value="">انتخاب Zone…</option>' + zones.map((z) => `<option value="${esc(z.id)}" ${z.id === monitor?.cf_zone_id ? 'selected' : ''}>${esc(z.name)}</option>`).join('');
    if (monitor?.cf_zone_id && zones.some((z) => z.id === monitor.cf_zone_id)) zoneSel.value = monitor.cf_zone_id;
    if (!zoneSel.value && zones[0]) zoneSel.value = zones[0].id;
    if (zoneSel.value) await populateCFRecordSelect(form, zoneSel.value, monitor?.cf_record_id || '', monitor?.cf_record_name || '', monitor?.cf_primary || '', accountID);
    if (note) note.innerHTML = `رکوردهای A/AAAA حساب «${esc(accounts.find((a) => Number(a.id) === accountID)?.name || '')}».`;
  } catch (e) {
    zoneSel.innerHTML = `<option value="">${esc('کلادفلر متصل نیست')}</option>`;
    if (note) note.innerHTML = `برای استفاده از سوییچ خودکار ابتدا <a href="#/cloudflare" style="color:var(--link-text)">کلادفلر را متصل کنید</a>.`;
  }
}
function probeAgentHelp(agent, type) {
  if (!agent) return 'تست‌ها از خود پنل انجام می‌شوند. برای دید از شبکهٔ یک سرور دیگر، ایجنت آن را انتخاب کنید.';
  const online = statusFromAgent(agent);
  const parts = [`تست از سرور «${agent.name}»`];
  if (!online) parts.push('ایجنت آفلاین است؛ تا اتصال مجدد، بررسی از این سرور انجام نمی‌شود');
  else if (agent.can_check === false && agent.transport === 'REST') parts.push('اتصال زندهٔ ایجنت برقرار نیست؛ برای تست از راه دور، gRPC/WebSocket لازم است');
  else if (agent.can_check === false) parts.push('این ایجنت از تست از راه دور پشتیبانی نمی‌کند؛ نسخهٔ ایجنت را به 1.5 به‌روزرسانی کنید');
  if (type === 'proxy') {
    if (agent.xray_available) parts.push(`Xray ${agent.xray_version || ''}`.trim());
    else parts.push('Xray روی این ایجنت شناسایی نشده؛ فقط بررسی سریع TCP/TLS انجام می‌شود');
  }
  return parts.join(' · ');
}

function updateProbeAgentHelp(form, type) {
  if (!form) return;
  const select = form.querySelector('#m-probe-agent');
  const help = form.querySelector('#m-probe-help');
  if (!select || !help) return;
  const agent = (state.agents || []).find((a) => Number(a.id) === Number(select.value));
  help.textContent = probeAgentHelp(agent, type);
  help.classList.toggle('bad', !!agent && !statusFromAgent(agent));
}

async function openMonitorForm(monitor = null) {
  let m = monitor;
  if (typeof m === 'number') {
    try { m = (await api(`/api/monitors/${m}`)).monitor; } catch (e) { toast(e.message, 'error'); return; }
  }
  const cfg = m?.config && typeof m.config === 'object' ? m.config : {};
  const type = m?.type || 'http';
  const target = targetMeta(type);
  const hasCF = !!m?.cf_enabled;
  const probeID = Number(m?.probe_agent_id) || 0;
  const probeAgent = (state.agents || []).find((a) => Number(a.id) === probeID) || null;
  const probeOptions = [
    `<option value="0" ${probeID ? '' : 'selected'}>پنل اصلی (همین سرور)</option>`,
    ...(state.agents || []).map((a) => `<option value="${a.id}" ${Number(a.id) === probeID ? 'selected' : ''}>${esc(a.name)}${statusFromAgent(a) ? '' : ' · آفلاین'}</option>`),
  ].join('');
  const body = `<form id="monitor-form" data-form="monitor" data-id="${m?.id || ''}" novalidate>
    <div class="form-error"></div>
    <div class="form-section" style="border-top:0;margin-top:0;padding-top:0">
      <div class="form-section-title">${icon('layers')} نوع بررسی</div>
      <div class="type-picker">
        ${Object.entries(TYPES).map(([key, t]) => `<button type="button" class="type-option ${key === type ? 'selected' : ''}" data-action="select-monitor-type" data-type="${key}" aria-pressed="${key === type}">${icon(t.icon)}<b>${t.label}</b><span>${key === 'http' ? 'کد وضعیت، کلیدواژه و SSL' : key === 'tcp' ? 'دسترسی پورت' : key === 'ping' ? 'پاسخ ICMP' : key === 'dns' ? 'مقدار رکورد' : key === 'ssl' ? 'انقضای گواهی' : key === 'vps' ? 'ترافیک، وضعیت و Suspend' : 'VMess/VLESS/WireGuard'}</span></button>`).join('')}
      </div>
      <input type="hidden" name="type" id="m-type" value="${type}">
    </div>
    <div class="form-grid">
      <div class="field"><label>نام مانیتور <span class="req">*</span></label><input class="input" name="name" id="m-name" value="${esc(m?.name || '')}" placeholder="مثلاً سایت اصلی" required></div>
      <div class="field"><label>گروه (اختیاری)</label><input class="input" name="group" id="m-group" value="${esc(m?.group || '')}" placeholder="مثلاً پروژه آلفا"></div>
      <div class="field full"><label id="m-target-label">${target.label}${target.optional ? '' : ' <span class="req">*</span>'}</label><input class="input ${type === 'proxy' || type === 'vps' ? '' : 'ltr'}" name="target" id="m-target" value="${esc(m?.target || '')}" placeholder="${esc(target.placeholder)}" ${target.optional ? '' : 'required'}><div class="help" id="m-target-hint">${target.hint}</div></div>
      <div class="field full">
        <label>اجرای تست از</label>
        <select class="select" name="probe_agent_id" id="m-probe-agent">${probeOptions}</select>
        <div class="help ${probeAgent && !statusFromAgent(probeAgent) ? 'bad' : ''}" id="m-probe-help">${probeAgentHelp(probeAgent, type)}</div>
      </div>
    </div>
    <div class="form-grid" id="type-fields" style="margin-top:12px">${monitorTypeFields(type, cfg)}</div>
    <div class="form-grid" style="margin-top:12px">
      <div class="field"><label>فاصله بررسی (ثانیه)</label><input class="input" type="number" min="10" max="86400" name="interval_sec" value="${Number(m?.interval_sec) || 60}"></div>
      <div class="field"><label>مهلت بررسی (میلی‌ثانیه)</label><input class="input" type="number" min="1000" max="180000" name="timeout_ms" value="${Number(m?.timeout_ms) || 12000}"></div>
      <div class="field"><label>آستانه خطا (تعداد بررسی)</label><input class="input" type="number" min="1" max="20" name="fail_threshold" value="${Number(m?.fail_threshold) || 2}"></div>
      <div class="field"><label>آستانه بازیابی (تعداد بررسی)</label><input class="input" type="number" min="1" max="20" name="recover_threshold" value="${Number(m?.recover_threshold) || 1}"></div>
      <div class="field full"><label class="check-row"><input type="checkbox" name="enabled" ${m ? (m.enabled ? 'checked' : '') : 'checked'}> فعال باشد</label></div>
    </div>
    <div class="form-section">
      <div class="form-section-title">${icon('cloud')} سوییچ خودکار DNS <span class="chip" style="margin-inline-start:auto">اختیاری</span></div>
      <label class="check-row" style="margin-bottom:8px"><input type="checkbox" name="cf_enabled" id="m-cf-enabled" ${hasCF ? 'checked' : ''}> در صورت قطعی، رکورد Cloudflare به IP پشتیبان تغییر کند</label>
      <div class="cf-extra ${hasCF ? '' : 'hidden'}" id="m-cf-extra">
        <div class="form-grid">
          <div class="field"><label>حساب Cloudflare</label><select class="select" id="m-cf-account" name="cf_account_id"><option value="">در حال دریافت…</option></select></div>
          <div class="field"><label>Zone</label><select class="select" id="m-cf-zone"><option value="">در حال دریافت…</option></select><div class="help" id="m-cf-note">برای تغییر، ابتدا کلادفلر را متصل کنید.</div></div>
          <div class="field"><label>رکورد A/AAAA</label><select class="select" id="m-cf-record"><option value="">ابتدا Zone را انتخاب کنید</option></select></div>
          <div class="field"><label>IP اصلی</label><input class="input ltr" name="cf_primary" id="m-cf-primary" value="${esc(m?.cf_primary || '')}" placeholder="1.2.3.4"></div>
          <div class="field"><label>IP پشتیبان</label><input class="input ltr" name="cf_failover" id="m-cf-failover" value="${esc(m?.cf_failover || '')}" placeholder="5.6.7.8"></div>
        </div>
        <div class="help" style="margin-top:7px">پالس فقط وقتی رکورد را تغییر می‌دهد که مقدار فعلی با IP اصلی (هنگام قطعی) یا IP پشتیبان (هنگام بازیابی) برابر باشد.</div>
      </div>
    </div>
  </form>`;
  openModal({
    title: m ? 'ویرایش مانیتور' : 'مانیتور جدید',
    subtitle: m ? m.name : 'یک بررسی تازه برای زیرساخت خود بسازید',
    body,
    size: 'lg',
    footer: `<button class="btn btn-ghost btn-sm" type="button" data-action="close-modal">انصراف</button><button class="btn btn-primary btn-sm" type="submit" form="monitor-form">${icon('check')} ${m ? 'ذخیره تغییرات' : 'ساخت مانیتور'}</button>`,
  });
  const form = $('#monitor-form');
  if (!form) return;
  form.querySelector('#m-probe-agent')?.addEventListener('change', () => {
    const formType = form.querySelector('#m-type')?.value || 'http';
    updateProbeAgentHelp(form, formType);
    if (formType === 'proxy') refreshXrayHint();
  });
  form.querySelector('#m-cf-enabled')?.addEventListener('change', (e) => {
    $('#m-cf-extra')?.classList.toggle('hidden', !e.target.checked);
  });
  updateVPSThresholdFields(form);
  if (hasCF || !m) hydrateCFSelects(form, m);
  if (type === 'proxy') refreshXrayHint();
  // The modal must not wait for the agent list. Refresh it in the background
  // and keep the current selection if the user already changed it.
  if (!(state.agents || []).length) {
    api('/api/agents').then((data) => {
      state.agents = data.agents || [];
      state.agentsOfflineAfter = data.offline_after_sec || state.agentsOfflineAfter;
      const sel = form.querySelector('#m-probe-agent');
      if (!sel) return;
      const current = Number(sel.value) || 0;
      sel.innerHTML = [
        `<option value="0" ${current ? '' : 'selected'}>پنل اصلی (همین سرور)</option>`,
        ...(state.agents || []).map((a) => `<option value="${a.id}" ${Number(a.id) === current ? 'selected' : ''}>${esc(a.name)}${statusFromAgent(a) ? '' : ' · آفلاین'}</option>`),
      ].join('');
      sel.value = String(current);
      updateProbeAgentHelp(form, form.querySelector('#m-type')?.value || 'http');
    }).catch(() => {});
  }
}
async function refreshXrayHint() {
  const el = $('#xray-hint');
  if (!el) return;
  const probeID = Number($('#m-probe-agent')?.value) || 0;
  const agent = (state.agents || []).find((a) => Number(a.id) === probeID);
  if (agent) {
    if (agent.xray_available) {
      el.innerHTML = `<span style="color:var(--success-text)">Xray ${esc(agent.xray_version || '')} روی ایجنت «${esc(agent.name)}» آماده است</span> — هندشیک واقعی از همان سرور انجام می‌شود.`;
    } else {
      el.innerHTML = `Xray روی ایجنت «${esc(agent.name)}» شناسایی نشده؛ فقط بررسی سریع TCP/TLS انجام می‌شود. برای هندشیک واقعی، Xray را روی همان ایجنت نصب کنید.`;
    }
    return;
  }
  el.textContent = 'در حال بررسی هستهٔ Xray…';
  try {
    const data = await api('/api/xray/status');
    const st = data.status || {};
    if (st.available) {
      el.innerHTML = `<span style="color:var(--success-text)">Xray ${esc(st.version)} آماده است</span> — برای هر کانفیگ یک هستهٔ موقت اجرا و یک درخواست HTTP واقعی از داخل تونل ارسال می‌شود.`;
    } else {
      el.innerHTML = `هستهٔ Xray شناسایی نشد: ${esc(st.error || 'باینری پیدا نشد')} — <a href="#/settings" style="color:var(--link-text)">نصب یا تنظیم مسیر</a>`;
    }
  } catch (e) {
    el.innerHTML = `وضعیت Xray در دسترس نیست: ${esc(e.message)}`;
  }
}
function collectMonitorConfig(form, type) {
  const cfg = {};
  if (type === 'http') {
    cfg.method = formVal(form, 'method') || 'GET';
    cfg.expected_status = formVal(form, 'expected_status');
    cfg.keyword = formVal(form, 'keyword');
    cfg.keyword_mode = formVal(form, 'keyword_mode');
    cfg.keyword_scope = formVal(form, 'keyword_scope') || 'body';
    cfg.follow_redirects = formChecked(form, 'follow_redirects');
    cfg.skip_tls_verify = formChecked(form, 'skip_tls_verify');
    cfg.slow_ms = Number(formVal(form, 'slow_ms')) || 0;
    cfg.host_header = formVal(form, 'host_header');
    cfg.body = formVal(form, 'body');
    const headersRaw = formVal(form, 'headers');
    if (headersRaw) {
      try {
        const parsed = JSON.parse(headersRaw);
        if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) cfg.headers = parsed;
        else throw new Error('invalid');
      } catch { throw new Error('هدرهای سفارشی باید یک آبجکت JSON معتبر باشند'); }
    }
  } else if (type === 'dns') {
    cfg.record = formVal(form, 'record') || 'A';
    cfg.expect = formVal(form, 'expect');
    cfg.server = formVal(form, 'server');
  } else if (type === 'ssl') {
    cfg.server_name = formVal(form, 'server_name');
    cfg.warn_days = Number(formVal(form, 'warn_days')) || 14;
    cfg.skip_verify = formChecked(form, 'skip_verify');
  } else if (type === 'proxy') {
    cfg.input = formVal(form, 'input');
    cfg.min_healthy = Number(formVal(form, 'min_healthy')) || 1;
    cfg.skip_tls_verify = formChecked(form, 'skip_tls_verify');
    cfg.real_test = formChecked(form, 'real_test');
    cfg.test_url = formVal(form, 'test_url');
    cfg.max_real_nodes = Number(formVal(form, 'max_real_nodes')) || 10;
    if (!cfg.input) throw new Error('متن کانفیگ یا لینک سابسکریپشن الزامی است');
  } else if (type === 'vps') {
    cfg.panel_url = formVal(form, 'panel_url');
    cfg.api_key = formVal(form, 'api_key');
    cfg.api_pass = formVal(form, 'api_pass');
    cfg.vps_id = Number(formVal(form, 'vps_id')) || 0;
    cfg.threshold_mode = formVal(form, 'threshold_mode') || 'percent';
    cfg.warn_percent = Number(formVal(form, 'warn_percent')) || 80;
    cfg.critical_percent = Number(formVal(form, 'critical_percent')) || 95;
    cfg.warn_remaining_gb = Number(formVal(form, 'warn_remaining_gb')) || 10;
    cfg.critical_remaining_gb = Number(formVal(form, 'critical_remaining_gb')) || 2;
    cfg.skip_tls_verify = formChecked(form, 'skip_tls_verify');
    if (!cfg.panel_url || !cfg.api_key || !cfg.api_pass || !cfg.vps_id) {
      throw new Error('آدرس پنل، API Key، API Password و VPS ID الزامی است');
    }
  }
  return cfg;
}

/* ------------------------------------------------------------------ rule form */
function openRuleForm(rule = null) {
  const r = rule;
  const triggers = Object.entries(TRIGGERS);
  const body = `<form id="rule-form" data-form="rule" data-id="${r?.id || ''}" novalidate>
    <div class="form-error"></div>
    <div class="form-grid">
      <div class="field full"><label>نام قانون <span class="req">*</span></label><input class="input" name="name" value="${esc(cleanRuleName(r?.name || ''))}" placeholder="مثلاً هشدار قطعی سایت اصلی" required></div>
      <div class="field"><label>رویداد</label><select class="select" name="trigger" id="rule-trigger">${triggers.map(([k, t]) => `<option value="${k}" ${r?.trigger === k ? 'selected' : ''}>${t.label}</option>`).join('')}</select></div>
      <div class="field" id="rule-monitor-field"><label>محدوده</label><select class="select" name="monitor_id"><option value="0">همه مانیتورها</option>${state.monitors.map((m) => `<option value="${m.id}" ${r?.monitor_id === m.id ? 'selected' : ''}>${esc(m.name)}</option>`).join('')}</select></div>
      <div class="field" id="rule-threshold-field"><label id="rule-threshold-label">آستانه</label><input class="input" type="number" name="threshold" id="rule-threshold" min="0" value="${r?.threshold || ''}" placeholder="14"><div class="help" id="rule-threshold-help">در صورت رسیدن به این مقدار، قانون اجرا می‌شود.</div></div>
      <div class="field"><label>حداقل فاصله بین اعلان‌ها (دقیقه)</label><input class="input" type="number" min="0" max="10080" name="cooldown" value="${r ? Math.round((r.cooldown_sec || 0) / 60) : 15}"><div class="help">برای قطعی/بازیابی، هر تغییر وضعیت ارسال می‌شود؛ این مقدار برای هشدارهای تکرارشونده مثل منابع و انقضاست.</div></div>
      <div class="field full"><label>کانال‌های ارسال</label>
        <div style="display:flex;gap:12px;flex-wrap:wrap;padding:4px 0">
          <label class="check-row"><input type="checkbox" name="channel" value="all" ${!r || (r.channels || []).includes('all') ? 'checked' : ''}> همه کانال‌های فعال</label>
          <label class="check-row"><input type="checkbox" name="channel" value="telegram" ${(r?.channels || []).includes('telegram') ? 'checked' : ''}> تلگرام</label>
          <label class="check-row"><input type="checkbox" name="channel" value="webpush" ${(r?.channels || []).includes('webpush') ? 'checked' : ''}> Web Push</label>
          <label class="check-row"><input type="checkbox" name="channel" value="webhook" ${(r?.channels || []).includes('webhook') ? 'checked' : ''}> وب‌هوک</label>
        </div>
        <div class="help">گزینه «همه کانال‌های فعال» از هر کانالی که در تنظیمات اعلان‌ها فعال باشد استفاده می‌کند.</div>
      </div>
      <div class="field full"><label class="check-row"><input type="checkbox" name="enabled" ${!r || r.enabled ? 'checked' : ''}> قانون فعال باشد</label></div>
    </div>
  </form>`;
  openModal({
    title: r ? 'ویرایش قانون' : 'قانون جدید',
    subtitle: 'تعیین کنید چه رویدادی و از چه کانالی اطلاع داده شود',
    body,
    footer: `<button class="btn btn-ghost btn-sm" type="button" data-action="close-modal">انصراف</button><button class="btn btn-primary btn-sm" type="submit" form="rule-form">${icon('check')} ذخیره قانون</button>`,
  });
  const form = $('#rule-form');
  const update = () => {
    const trigger = formVal(form, 'trigger');
    const isResource = ['cpu_high', 'ram_high', 'disk_high'].includes(trigger);
    const isCert = trigger === 'cert_expiring';
    const scopeable = ['down', 'up', 'degraded', 'cert_expiring', 'dns_mismatch'].includes(trigger);
    $('#rule-monitor-field')?.classList.toggle('hidden', !scopeable);
    $('#rule-threshold-field')?.classList.toggle('hidden', !(isResource || isCert));
    if (isResource) {
      $('#rule-threshold-label').textContent = 'آستانه مصرف (درصد)';
      $('#rule-threshold').placeholder = '90';
      $('#rule-threshold-help').textContent = 'وقتی مصرف از این درصد عبور کند، اعلان ارسال می‌شود.';
    } else if (isCert) {
      $('#rule-threshold-label').textContent = 'چند روز قبل از انقضا';
      $('#rule-threshold').placeholder = '14';
      $('#rule-threshold-help').textContent = 'وقتی کمتر از این تعداد روز تا انقضای گواهی باقی بماند.';
    }
  };
  form.elements.namedItem('trigger')?.addEventListener('change', update);
  update();
}

/* ------------------------------------------------------------------ agent dialogs */
function openAgentGuide() {
  openModal({
    title: 'راهنمای نصب ایجنت',
    subtitle: 'پایش منابع سرور در کمتر از دو دقیقه',
    size: 'lg',
    body: `<div class="steps">
      <div class="step"><span class="step-num">1</span><p><b>یک ایجنت بساز</b> تا توکن اختصاصی و دستور نصب یک‌خطی برایت ساخته شود.</p></div>
      <div class="step"><span class="step-num">2</span><p>همان دستور را روی VPS مقصد با <code>sudo</code> اجرا کن. اینستالر باینری را از Release رسمی می‌گیرد، سرویس systemd را می‌سازد و توکن را در <code>/etc/pulse/agent.env</code> امن نگه می‌دارد.</p></div>
      <div class="step"><span class="step-num">3</span><p>وضعیت اتصال زنده را با <code>pulse-a status</code> ببین. آمار هر 1 ثانیه می‌رسد و در قطعی موقت، ایجنت با backoff خودکار دوباره وصل می‌شود.</p></div>
    </div>
    <div class="agent-rest-note">${icon('signal')} اتصال gRPC با fallback به WebSocket و REST؛ keep-alive، تلاش مجدد خودکار و پایش آخرین گزارش.</div>
    <div class="warning-note">${icon('shield')} <div>ایجنت با دسترسی root برای خواندن آمار WireGuard و وضعیت systemd اجرا می‌شود. توکن را در جای امن نگه دار.</div></div>`,
    footer: `<button class="btn btn-ghost btn-sm" type="button" data-action="close-modal">بستن</button><button class="btn btn-primary btn-sm" type="button" data-action="new-agent">${icon('plus')} ساخت ایجنت</button>`,
  });
}
function openAgentForm() {
  openModal({
    title: 'افزودن سرور جدید',
    subtitle: 'یک ایجنت سبک روی سرور نصب کنید',
    body: `<form id="agent-form" data-form="agent"><div class="form-error"></div><div class="field"><label>نام سرور</label><input class="input" name="name" placeholder="مثلاً vps-frankfurt-1" required autocomplete="off"><div class="help">نامی که در داشبورد نمایش داده می‌شود.</div></div></form>`,
    footer: `<button class="btn btn-ghost btn-sm" type="button" data-action="close-modal">انصراف</button><button class="btn btn-primary btn-sm" type="submit" form="agent-form">${icon('plus')} ساخت ایجنت</button>`,
  });
}
function openAgentToken(data, title = 'ایجنت آماده نصب است') {
  const unit = data.unit || '';
  const install = data.install_command || data.command || '';
  openModal({
    title,
    subtitle: 'نصب اتصال زنده با یک دستور روی VPS مقصد',
    size: 'lg',
    body: `<div class="warning-note" style="margin:0 0 12px">${icon('alert')} <div>این توکن فقط یک‌بار نمایش داده می‌شود. اگر گم شد، از داخل پنل توکن جدید بسازید.</div></div>
      <div class="install-card">
        <div class="install-card-head"><span class="install-badge">${icon('terminal')} نصب یک‌خطی</span><span class="chip green">gRPC</span></div>
        <div class="code-block" id="agent-install">${esc(install)}<button class="copy-btn" type="button" data-action="copy" data-copy-target="agent-install">کپی</button></div>
        <div class="install-help">روی VPS مقصد، با کاربر root اجرا کن. اسکریپت باینری را از GitHub Releases دانلود می‌کند، Xray رسمی را برای تست کانفیگ نصب می‌کند و سرویس <code>pulse-a</code> را با اتصال زنده بالا می‌آورد.</div>
      </div>
      <div class="token-box" style="margin-top:14px"><b>AGENT TOKEN</b><code id="agent-token">${esc(data.token || '')}</code><button class="btn btn-sm btn-ghost" style="margin-top:6px" type="button" data-action="copy" data-copy-target="agent-token">${icon('copy')} کپی توکن</button></div>
      <div class="agent-rest-note">${icon('signal')} پروتکل ایجنت: gRPC + WebSocket + fallback REST · گزارش زندهٔ 1 ثانیه‌ای</div>
      <details class="agent-manual">
        <summary>${icon('sliders')} نصب دستی / عیب‌یابی پیشرفته</summary>
        ${data.download_command ? `<div style="margin-top:10px"><div class="form-section-title">${icon('download')} دریافت باینری</div><div class="code-block" id="agent-download">${esc(data.download_command)}<button class="copy-btn" type="button" data-action="copy" data-copy-target="agent-download">کپی</button></div></div>` : ''}
        <div style="margin-top:10px"><div class="form-section-title">${icon('server')} اجرای مستقیم</div><div class="code-block" id="agent-cmd">${esc(data.command || '')}<button class="copy-btn" type="button" data-action="copy" data-copy-target="agent-cmd">کپی</button></div></div>
        <div style="margin-top:10px"><div class="form-section-title">${icon('server')} سرویس systemd</div><button class="copy-btn" style="position:static;margin-bottom:5px" type="button" data-action="copy" data-copy-target="agent-unit">کپی unit</button><div class="code-block" id="agent-unit">${esc(unit)}</div></div>
      </details>`,
    footer: `<button class="btn btn-primary btn-sm" type="button" data-action="close-modal">متوجه شدم</button>`,
  });
}
async function openAgentDetail(id, opts = {}) {
  const guard = ++state.detailEpoch;
  if (state.modal) closeModal(true);
  const skeleton = openDetailSkeleton('در حال بارگذاری سرور…', 'دریافت منابع، سرویس‌ها و تاریخچه');
  let data;
  try { data = await api(`/api/agents/${id}/metrics?hours=${opts.hours || 24}`); } catch (e) {
    if (guard === state.detailEpoch && state.view === 'servers') {
      if (!skeleton.isClosed()) closeModal(true);
      toast(e.message, 'error');
      if (parseHash().id === id) history.replaceState(null, '', '#/servers');
    }
    return;
  }
  if (guard !== state.detailEpoch || state.view !== 'servers') return;
  if (skeleton.isClosed()) return;
  const a = data.agent;
  const metrics = data.metrics || [];
  state.agentHours = opts.hours || 24;
  const online = statusFromAgent(a);
  const services = Array.isArray(a.services) ? a.services : [];
  const wg = Array.isArray(a.wireguard) ? a.wireguard : [];
  const peers = wg.reduce((s, w) => s + (w.peers?.length || 0), 0);
  const body = `<div class="detail-hero">
    <div class="agent-ico agent-ico-pulse" style="width:46px;height:46px">${icon('agent')}</div>
    <div class="detail-title"><h2>${esc(a.name)} <span class="agent-online ${online ? '' : 'off'}" data-agent-detail-online><span class="dot"></span>${online ? 'آنلاین' : 'آفلاین'}</span></h2><div class="target" style="line-height:1.9"><span class="ltr">${esc(a.ip || 'بدون IP عمومی')}</span>${a.peer_ip && a.peer_ip !== a.ip ? ` <span class="chip" style="height:20px;padding:0 7px" title="آدرسی که اتصال ایجنت از آن دیده می‌شود">IP اتصال: <b class="ltr">${esc(a.peer_ip)}</b></span>` : ''}<div style="font-size:9.5px;color:var(--faint);margin-top:2px">${esc(a.hostname || '—')}${a.os ? ' · ' + esc(a.os) : ''}</div></div>${a.ip ? `<div class="agent-geo" data-agent-geo>در حال دریافت موقعیت و مالک IP…</div><div class="help" style="margin-top:2px">IP عمومی از سرویس‌های خارجی خوانده می‌شود؛ اگر سرور پشت NAT یا پروکسی باشد ممکن است با IP اصلی سرور متفاوت باشد.</div>` : ''}</div>
    <div class="detail-actions"><button class="btn btn-sm" type="button" data-action="restart-agent" data-id="${a.id}">${icon('refresh')} ری‌استارت</button><button class="btn btn-sm" type="button" data-action="update-agent" data-id="${a.id}">${icon('download')} آپدیت ایجنت</button><button class="btn btn-sm" type="button" data-action="edit-agent" data-id="${a.id}">${icon('edit')} ویرایش</button><button class="btn btn-sm btn-soft" type="button" data-action="rotate-agent" data-id="${a.id}">${icon('key')} توکن جدید</button></div>
  </div>
  <div class="agent-connection detail-connection">
    <span class="agent-quality ${online ? 'great' : 'off'}" data-agent-detail-quality>${icon('signal')} ${online ? `${agentTransport(a) || 'زنده'} متصل` : 'قطع ارتباط'}</span>
    <span class="chip ${transportChipClass(a)}" data-agent-detail-transport>${agentTransport(a) || 'REST'}</span>
    <span data-agent-detail-seen-inline>آخرین گزارش: ${timeAgo(a.last_seen)}</span>
    <span class="agent-update-hint">${icon('terminal')} <code>pulse-a update</code></span>
    <span class="agent-update-hint">${icon('terminal')} <code>pulse-a logs</code></span>
  </div>
  ${a.can_check && online && !a.xray_available ? `<div class="warning-note" style="margin-top:10px">${icon('alert')} <div>برای تست واقعی کانفیگ از این ایجنت، Xray روی آن شناسایی نشده است. نصب‌کنندهٔ جدید ایجنت Xray رسمی را نصب می‌کند؛ برای ایجنت فعلی، Xray را نصب یا ایجنت را با نصب‌کنندهٔ جدید دوباره نصب کنید و سرویس ایجنت را ری‌استارت کنید.</div></div>` : ''}
  <div class="gauges" style="padding:16px 0 8px">
    ${gaugeRing(a.cpu_pct || 0, gaugeColor(a.cpu_pct || 0), 'CPU', 'cpu')}
    ${gaugeRing(a.ram_pct || 0, gaugeColor(a.ram_pct || 0), 'RAM', 'ram')}
    ${gaugeRing(a.disk_pct || 0, gaugeColor(a.disk_pct || 0), 'دیسک', 'disk')}
  </div>
  <div class="detail-stats">
    <div class="mini-stat"><span>Load 1m</span><b data-agent-detail-load>${nf1.format(a.load1 || 0)}</b></div>
    <div class="mini-stat"><span>آپتایم سرور</span><b style="font-size:11px" data-agent-detail-uptime>${duration(a.uptime_sec || 0)}</b></div>
    <div class="mini-stat"><span>حافظه</span><b style="font-size:11px" data-agent-detail-ram>${a.ram_total ? bytes(a.ram_total) : '—'}</b></div>
    <div class="mini-stat"><span>دیسک</span><b style="font-size:11px" data-agent-detail-disk>${a.disk_total ? bytes(a.disk_total) : '—'}</b></div>
    <div class="mini-stat"><span>دریافت</span><b style="font-size:11px" data-agent-detail-rx>${bytes(a.net_rx_bps || 0)}/s</b></div>
    <div class="mini-stat"><span>ارسال</span><b style="font-size:11px" data-agent-detail-tx>${bytes(a.net_tx_bps || 0)}/s</b></div>
    <div class="mini-stat"><span>آخرین گزارش</span><b style="font-size:11px" data-agent-detail-seen>${timeAgo(a.last_seen)}</b></div>
    <div class="mini-stat"><span>نسخه ایجنت</span><b data-agent-detail-version>${a.version ? 'v' + fa(a.version) : '—'}</b></div>
    <div class="mini-stat"><span>تست از این ایجنت</span><b style="font-size:11px" data-agent-detail-xray>${a.can_check ? (a.xray_available ? `Xray ${esc(a.xray_version || '')}` : 'بدون Xray') : online ? 'نیازمند اتصال زنده' : 'آفلاین'}</b></div>
  </div>
  <div class="detail-chart">
    <div class="detail-chart-head"><b>${icon('chart')} مصرف منابع</b><div class="range-tabs">${[6, 24, 72].map((h) => `<button type="button" data-action="agent-range" data-id="${id}" data-hours="${h}" class="${(opts.hours || 24) == h ? 'active' : ''}">${h === 24 ? '24 ساعت' : h === 6 ? '6 ساعت' : '3 روز'}</button>`).join('')}</div></div>
    <div style="position:relative"><canvas id="agentChart"></canvas>${metrics.length < 2 ? `<div class="chart-empty">${icon('chart')}<span>داده کافی برای نمودار وجود ندارد</span></div>` : ''}</div>
    <div class="agent-chart-legend"><span class="legend-item"><i class="legend-dot" style="background:#38bdf8"></i> CPU</span><span class="legend-item"><i class="legend-dot" style="background:#8b5cf6"></i> RAM</span><span class="legend-item"><i class="legend-dot" style="background:#f472b6"></i> دیسک</span></div>
  </div>
  <div class="detail-chart">
    <div class="detail-chart-head"><b>${icon('sliders')} سرویس‌ها</b><span>${num(services.length)} سرویس</span></div>
    ${services.length ? `<div class="table-wrap"><table class="node-table"><thead><tr><th>سرویس</th><th>وضعیت</th><th>جزئیات</th></tr></thead><tbody>${services.map((s) => `<tr><td class="node-name">${esc(s.name)}</td><td>${s.active ? '<span class="chip green">فعال</span>' : '<span class="chip red">غیرفعال</span>'}</td><td style="font-family:var(--mono);direction:ltr;text-align:right;font-size:8.5px">${esc(s.detail || '—')}</td></tr>`).join('')}</tbody></table></div>` : '<div class="help">سرویسی برای پایش تعریف نشده است. با فلگ <code>--services</code> هنگام نصب ایجنت اضافه کنید.</div>'}
  </div>
  <div class="detail-chart">
    <div class="detail-chart-head"><b>${icon('link')} WireGuard</b><span>${num(peers)} همتا</span></div>
    ${wg.length ? wg.map((iface) => `<div style="margin-bottom:10px"><div class="form-section-title">${icon('shield')} ${esc(iface.name)}</div><div class="table-wrap"><table class="node-table"><thead><tr><th>Public Key</th><th>Endpoint</th><th>Last Handshake</th><th>RX</th><th>TX</th></tr></thead><tbody>${(iface.peers || []).map((p) => `<tr><td class="record-name" style="max-width:120px;overflow:hidden;text-overflow:ellipsis">${esc(p.public_key || '—')}</td><td class="record-name">${esc(p.endpoint || '—')}</td><td>${p.latest_handshake ? timeAgo(p.latest_handshake) : 'هرگز'}</td><td>${bytes(p.rx || 0)}</td><td>${bytes(p.tx || 0)}</td></tr>`).join('')}</tbody></table></div></div>`).join('') : '<div class="help">داده WireGuard موجود نیست. ماژول wg را نصب و دسترسی root را برای ایجنت فعال کنید.</div>'}
  </div>`;
  openModal({
    title: a.name,
    subtitle: 'جزئیات ایجنت سرور',
    size: 'lg',
    body,
    footer: `<div class="left"><button class="btn btn-danger btn-sm" type="button" data-action="delete-agent" data-id="${a.id}">${icon('trash')} حذف ایجنت</button></div><button class="btn btn-ghost btn-sm" type="button" data-action="close-detail" data-id="${a.id}">بستن</button>`,
    onClose: () => { if (location.hash.includes(`/servers/${a.id}`)) history.replaceState(null, '', '#/servers'); },
  });
  if (state.modal) Object.assign(state.modal, { kind: 'agent', agentId: Number(a.id) });
  if (a.ip) {
    geoLookup(a.ip).then((info) => {
      if (!state.modal || state.modal.kind !== 'agent' || Number(state.modal.agentId) !== Number(a.id)) return;
      const el = $('[data-agent-geo]');
      if (!el) return;
      if (!info || info.error) {
        el.textContent = 'IP owner lookup unavailable';
        return;
      }
      const flag = info.country_code ? String.fromCodePoint(...[...info.country_code.toUpperCase()].map((c) => 127397 + c.charCodeAt(0))) : '';
      el.textContent = [flag, info.country, info.isp || info.org, info.asn].filter(Boolean).join(' · ');
    });
  }
  if (metrics.length >= 2) {
    setTimeout(() => {
      const canvas = $('#agentChart');
      if (!canvas || state.modal?.kind !== 'agent' || Number(state.modal?.agentId) !== Number(a.id)) return;
      lineChart(canvas, {
        series: [
          { name: 'CPU', color: '#38bdf8', points: metrics.map((m) => ({ x: m.at, y: m.cpu })), fill: true },
          { name: 'RAM', color: '#8b5cf6', points: metrics.map((m) => ({ x: m.at, y: m.ram })) },
          { name: 'دیسک', color: '#f472b6', points: metrics.map((m) => ({ x: m.at, y: m.disk })) },
        ],
        yMax: 100,
        yFormat: (v) => num(v) + '٪',
        tipFormat: (v) => nf1.format(v) + '٪',
      });
    }, 40);
  }
  $$('.gauge .val').forEach((el) => {
    const dash = el.getAttribute('stroke-dasharray');
    const final = el.getAttribute('stroke-dashoffset');
    if (dash && final !== null) { el.style.strokeDashoffset = dash; requestAnimationFrame(() => setTimeout(() => { el.style.strokeDashoffset = final; }, 50)); }
  });
}
function openEditAgent(a) {
  openModal({
    title: 'ویرایش ایجنت',
    subtitle: a.name,
    body: `<form id="agent-edit-form" data-form="agent-edit" data-id="${a.id}"><div class="form-error"></div><div class="form-grid"><div class="field full"><label>نام سرور</label><input class="input" name="name" value="${esc(a.name)}" required></div><div class="field full"><label>یادداشت</label><textarea class="textarea" name="notes" rows="3" placeholder="مثلاً: سرور اصلی پروژه آلفا">${esc(a.notes || '')}</textarea></div></div></form>`,
    footer: `<button class="btn btn-ghost btn-sm" type="button" data-action="close-modal">انصراف</button><button class="btn btn-primary btn-sm" type="submit" form="agent-edit-form">ذخیره</button>`,
  });
}
function openPasswordForm() {
  openModal({
    title: 'تغییر رمز عبور',
    subtitle: 'رمز فعلی و رمز جدید را وارد کنید',
    body: `<form id="password-form" data-form="password"><div class="form-error"></div><div class="form-grid"><div class="field full"><label>رمز فعلی</label><input class="input ltr" type="password" name="current" autocomplete="current-password" required></div><div class="field full"><label>رمز جدید (حداقل 8 کاراکتر)</label><input class="input ltr" type="password" name="new" autocomplete="new-password" required></div><div class="field full"><label>تکرار رمز جدید</label><input class="input ltr" type="password" name="confirm" autocomplete="new-password" required></div></div></form>`,
    footer: `<button class="btn btn-ghost btn-sm" type="button" data-action="close-modal">انصراف</button><button class="btn btn-primary btn-sm" type="submit" form="password-form">${icon('key')} تغییر رمز</button>`,
  });
}
function openCFAccountForm(account = null) {
  let a = account;
  if (typeof a === 'number') a = state.cfAccounts.find((x) => Number(x.id) === a) || null;
  const editing = !!a?.id;
  openModal({
    title: editing ? 'ویرایش حساب Cloudflare' : 'حساب Cloudflare جدید',
    subtitle: editing ? a.name : 'توکن را با دسترسی Zone:Read و DNS:Edit بسازید',
    body: `<form id="cf-account-form" data-form="cf-account" data-id="${a?.id || ''}" novalidate>
      <div class="form-error"></div>
      <div class="form-grid">
        <div class="field full"><label>نام حساب <span class="req">*</span></label><input class="input" name="name" value="${esc(a?.name || '')}" placeholder="مثلاً حساب اصلی" required autocomplete="off"></div>
        <div class="field full"><label>API Token ${editing ? '' : '<span class="req">*</span>'}</label><input class="input ltr" type="password" name="token" placeholder="${editing ? 'برای تغییر توکن، مقدار جدید را وارد کنید' : 'cf_xxxxxxxxxxxxxxxx'}" ${editing ? '' : 'required'} autocomplete="off"><div class="help">${editing ? 'اگر خالی بماند، توکن فعلی حفظ می‌شود.' : 'توکن از Cloudflare → My Profile → API Tokens ساخته می‌شود.'}</div></div>
        <div class="field full"><label class="check-row"><input type="checkbox" name="enabled" ${!a || a.enabled ? 'checked' : ''}> این حساب برای اتوماسیون DNS فعال باشد</label></div>
      </div>
    </form>`,
    footer: `<button class="btn btn-ghost btn-sm" type="button" data-action="close-modal">انصراف</button><button class="btn btn-primary btn-sm" type="submit" form="cf-account-form">${icon('check')} ${editing ? 'ذخیره تغییرات' : 'تأیید و افزودن'}</button>`,
  });
}
function openCFTokenForm() { openCFAccountForm(Number(state.cfAccount) || null); }

function openCFRecordForm(record = null) {
  const r = record || {};
  const zoneId = r.zone_id || state.cfZone;
  if (!zoneId) { toast('ابتدا یک دامنه / Zone انتخاب کنید', 'error'); return; }
  const editing = !!r.id;
  const type = r.type || 'A';
  openModal({
    title: editing ? 'ویرایش رکورد DNS' : 'رکورد DNS جدید',
    subtitle: editing ? `${r.name} (${r.type})` : 'تغییرات فوراً روی Cloudflare اعمال می‌شود',
    body: `<form id="cf-record-form" data-form="cf-record" data-id="${esc(r.id || '')}" novalidate>
      <div class="form-error"></div>
      <input type="hidden" name="account_id" value="${Number(state.cfAccount) || ''}">
      <input type="hidden" name="zone_id" value="${esc(zoneId)}">
      <div class="form-grid">
        <div class="field"><label>نوع رکورد</label><select class="select" name="type" id="cf-record-form-type">${['A', 'AAAA', 'CNAME', 'TXT', 'MX', 'NS', 'SRV', 'CAA'].map((t) => `<option value="${t}" ${type === t ? 'selected' : ''}>${t}</option>`).join('')}</select></div>
        <div class="field"><label>TTL</label><input class="input" type="number" min="1" name="ttl" value="${Number(r.ttl) || 1}"><div class="help">1 = خودکار</div></div>
        <div class="field full"><label>نام <span class="req">*</span></label><input class="input ltr" name="name" value="${esc(r.name || '')}" placeholder="example.com یا sub" required></div>
        <div class="field full"><label>مقدار <span class="req">*</span></label><input class="input ltr" name="content" value="${esc(r.content || '')}" placeholder="1.2.3.4 یا target.example.com" required></div>
        <div class="field" id="cf-record-priority-field"><label>Priority</label><input class="input" type="number" min="0" name="priority" value="${Number(r.priority) || (type === 'MX' ? 10 : 0)}" placeholder="10"></div>
        <div class="field" id="cf-record-proxied-field"><label class="check-row" style="margin-top:25px"><input type="checkbox" name="proxied" ${r.proxied ? 'checked' : ''}> عبور از Proxy کلادفلر</label></div>
      </div>
    </form>`,
    footer: `<button class="btn btn-ghost btn-sm" type="button" data-action="close-modal">انصراف</button><button class="btn btn-primary btn-sm" type="submit" form="cf-record-form">${icon('check')} ${editing ? 'ذخیره رکورد' : 'ساخت رکورد'}</button>`,
  });
  const form = $('#cf-record-form');
  const typeEl = form?.querySelector('#cf-record-form-type');
  const update = () => {
    const t = typeEl?.value || 'A';
    form?.querySelector('#cf-record-priority-field')?.classList.toggle('hidden', !['MX', 'SRV'].includes(t));
    form?.querySelector('#cf-record-proxied-field')?.classList.toggle('hidden', !['A', 'AAAA', 'CNAME'].includes(t));
  };
  typeEl?.addEventListener('change', update);
  update();
}

/* ------------------------------------------------------------------ UI helpers */
function applySidebarCollapse(collapsed) {
  const root = document.documentElement;
  root.classList.toggle('sidebar-collapsed', !!collapsed);
  try { localStorage.setItem('pulse-sidebar-collapsed', collapsed ? '1' : '0'); } catch { /* private mode */ }
  const btn = $('#sidebar-toggle');
  if (btn) {
    btn.setAttribute('aria-pressed', collapsed ? 'true' : 'false');
    btn.setAttribute('aria-label', collapsed ? 'باز کردن منو' : 'جمع کردن منو');
    btn.title = collapsed ? 'باز کردن منو' : 'جمع کردن منو';
  }
  $$('.nav-item').forEach((item) => {
    if (!collapsed) { item.removeAttribute('title'); return; }
    const label = Array.from(item.children).find((child) => child.tagName === 'SPAN'
      && !child.classList.contains('ni') && !child.classList.contains('nav-glow') && !child.classList.contains('nav-badge'));
    if (label?.textContent.trim()) item.title = label.textContent.trim();
  });
}
function toggleSidebarCollapse() {
  applySidebarCollapse(!document.documentElement.classList.contains('sidebar-collapsed'));
}
function openSidebar() {
  $('#sidebar')?.classList.add('open');
  $('#sidebar-backdrop')?.classList.add('show');
}
function closeSidebar() {
  $('#sidebar')?.classList.remove('open');
  $('#sidebar-backdrop')?.classList.remove('show');
}
function toggleUserMenu() {
  if (state.userMenu) { closeUserMenu(); return; }
  const el = document.createElement('div');
  el.className = 'user-pop';
  el.innerHTML = `<div class="user-pop-head"><b>${esc(state.auth?.username || 'admin')}</b><span>مدیر سامانه</span></div>
    <button type="button" data-action="go-settings">${icon('settings')} تنظیمات</button>
    <button type="button" data-action="go-notifications">${icon('bell')} اعلان‌ها</button>
    <button type="button" data-action="change-password">${icon('key')} تغییر رمز عبور</button>
    <div class="divider"></div>
    <button type="button" class="danger" data-action="logout">${icon('logout')} خروج از حساب</button>`;
  document.body.appendChild(el);
  state.userMenu = el;
}
function closeUserMenu() {
  if (state.userMenu) { state.userMenu.remove(); state.userMenu = null; }
}
async function copyText(text) {
  try {
    if (navigator.clipboard && window.isSecureContext) await navigator.clipboard.writeText(text);
    else {
      const ta = document.createElement('textarea');
      ta.value = text; ta.style.position = 'fixed'; ta.style.opacity = '0';
      document.body.appendChild(ta); ta.select(); document.execCommand('copy'); ta.remove();
    }
    toast('در حافظه کپی شد', 'success');
  } catch { toast('کپی انجام نشد', 'error'); }
}
function textWithoutButtons(node) {
  if (!node) return '';
  const clone = node.cloneNode(true);
  clone.querySelectorAll('button').forEach((b) => b.remove());
  return (clone.textContent || '').trim();
}
function goTo(hash) {
  closeUserMenu();
  if (location.hash === hash) route();
  else location.hash = hash;
}

/* ------------------------------------------------------------------ click router */
async function onDocumentClick(e) {
  if (state.userMenu && !e.target.closest('.user-pop') && !e.target.closest('#avatar-btn')) closeUserMenu();
  const filterBtn = e.target.closest('[data-filter]');
  if (filterBtn) {
    state.filters.status = filterBtn.dataset.filter;
    refreshMonitorGrid();
    return;
  }
  const incFilter = e.target.closest('[data-incident-filter]');
  if (incFilter) {
    state.incidentFilter = incFilter.dataset.incidentFilter;
    renderIncidents();
    return;
  }
  const actionEl = e.target.closest('[data-action]');
  if (!actionEl) {
    if (!e.target.closest('.user-pop') && !e.target.closest('#avatar-btn')) closeUserMenu();
    return;
  }
  const action = actionEl.dataset.action;
  const id = Number(actionEl.dataset.id) || 0;
  if (action !== 'copy') e.preventDefault();
  try {
    switch (action) {
      case 'open-sidebar': openSidebar(); break;
      case 'close-sidebar': closeSidebar(); break;
      case 'toggle-sidebar': toggleSidebarCollapse(); break;
      case 'close-modal': closeModal(); break;
      case 'close-detail': closeModal(); break;
      case 'user-menu': toggleUserMenu(); break;
      case 'go-settings': goTo('#/settings'); closeUserMenu(); break;
      case 'go-notifications': goTo('#/notifications'); closeUserMenu(); break;
      case 'logout': closeUserMenu(); await logout(); break;
      case 'refresh': closeUserMenu(); await loadView(state.view, null, false); break;
      case 'refresh-logs': await refreshLogsList(); break;
      case 'toggle-lang': setLang(getLang() === 'en' ? 'fa' : 'en'); location.reload(); break;
      case 'quick-monitor': await openMonitorForm(); break;
      case 'select-monitor-type': {
        const form = $('#monitor-form');
        if (!form) break;
        const type = actionEl.dataset.type;
        const cfg = {};
        form.querySelector('#m-type').value = type;
        $$('.type-option', form).forEach((el) => {
          const selected = el.dataset.type === type;
          el.classList.toggle('selected', selected);
          el.setAttribute('aria-pressed', selected ? 'true' : 'false');
        });
        form.querySelector('#type-fields').innerHTML = monitorTypeFields(type, cfg);
        updateVPSThresholdFields(form);
        const meta = targetMeta(type);
        form.querySelector('#m-target-label').innerHTML = `${meta.label}${meta.optional ? '' : ' <span class="req">*</span>'}`;
        const inp = form.querySelector('#m-target');
        inp.placeholder = meta.placeholder;
        inp.required = !meta.optional;
        inp.classList.toggle('ltr', type !== 'proxy' && type !== 'vps');
        form.querySelector('#m-target-hint').textContent = meta.hint;
        updateProbeAgentHelp(form, type);
        wireLabels(form);
        if (type === 'proxy') {
          if (!form.dataset.id) {
            const timeout = form.elements.namedItem('timeout_ms');
            if (timeout && Number(timeout.value) < 30000) timeout.value = 45000;
          }
          refreshXrayHint();
        }
        break;
      }
      case 'toggle-password': {
        const input = actionEl.closest('.password-wrap')?.querySelector('input');
        if (input) input.type = input.type === 'password' ? 'text' : 'password';
        break;
      }
      case 'open-monitor': {
        const mid = Number(actionEl.dataset.id) || id;
        if (mid) goTo(`#/monitors/${mid}`);
        break;
      }
      case 'open-agent': {
        const aid = Number(actionEl.dataset.id) || id;
        if (aid) goTo(`#/servers/${aid}`);
        break;
      }
      case 'toggle-theme': {
        applyTheme(document.documentElement.dataset.theme === 'light' ? 'dark' : 'light');
        break;
      }
      case 'check-monitor': {
        const original = actionEl.innerHTML;
        const inDetail = state.modal?.kind === 'monitor' && Number(state.modal?.monitorId) === id;
        const epochAtStart = state.viewEpoch;
        actionEl.disabled = true;
        actionEl.classList.add('is-busy');
        actionEl.innerHTML = `<span class="spin"></span> در حال تست…`;
        try {
          const res = await api(`/api/monitors/${id}/check?wait=1`, { method: 'POST', body: {}, timeout: 120000 });
          const status = res.check?.status || res.monitor?.status;
          const labels = { up: 'سالم', degraded: 'هشدار', down: 'قطع', unknown: 'نامشخص' };
          toast(`بررسی انجام شد${status ? ' — ' + (labels[status] || status) : ''}`, status === 'down' ? 'error' : 'success');
          if (inDetail) {
            if (state.modal?.kind === 'monitor' && Number(state.modal?.monitorId) === id) await openMonitorDetail(id);
          } else if (state.view === 'monitors' && epochAtStart === state.viewEpoch && !state.modal) {
            await loadView('monitors', null, true);
          }
        } catch (e) {
          toast(e.message || 'بررسی مانیتور ناموفق بود', 'error');
        } finally {
          if (actionEl.isConnected) {
            actionEl.disabled = false;
            actionEl.classList.remove('is-busy');
            actionEl.innerHTML = original;
          }
        }
        break;
      }
      case 'toggle-monitor': {
        const res = await api(`/api/monitors/${id}/toggle`, { method: 'POST', body: {} });
        toast(res.enabled ? 'مانیتور فعال شد' : 'مانیتور متوقف شد', 'success');
        closeModal(true);
        await route();
        break;
      }
      case 'edit-monitor': {
        if (location.hash.includes('/monitors/')) history.replaceState(null, '', '#/monitors');
        closeModal(true);
        const cached = (state.monitors || []).find((m) => Number(m.id) === id)
          || (state.summary?.monitors || []).find((m) => Number(m.id) === id);
        if (cached) openMonitorForm(cached);
        else await openMonitorForm(id);
        break;
      }
      case 'delete-monitor': {
        const ok = await confirmAction({ title: 'حذف مانیتور', message: 'با حذف این مانیتور، تاریخچه بررسی‌ها و رخدادهای آن هم پاک می‌شود. مطمئن هستید؟', confirmText: 'حذف کن', danger: true });
        if (!ok) break;
        await api(`/api/monitors/${id}`, { method: 'DELETE' });
        closeModal(true);
        history.replaceState(null, '', state.view === 'monitors' ? '#/monitors' : '#/dashboard');
        toast('مانیتور حذف شد', 'success');
        await route();
        break;
      }
      case 'resolve-incident': {
        await api(`/api/incidents/${id}/resolve`, { method: 'POST', body: {} });
        toast('رخداد به‌صورت دستی بسته شد', 'success');
        await loadView('incidents', null, true);
        if (state.summary?.stats) state.summary.stats.active_incidents = state.incidents.filter((i) => !i.ended_at).length;
        updateBadges();
        break;
      }
      case 'clear-filters': state.filters = { status: 'all', search: '' }; renderMonitors(); afterRender(); break;
      case 'new-agent': openAgentForm(); break;
      case 'restart-agent': {
        const ok = await confirmAction({ title: 'ری‌استارت ایجنت', message: 'سرویس ایجنت روی سرور ری‌استارت می‌شود و اتصال برای چند ثانیه قطع خواهد شد. ادامه می‌دهید؟', confirmText: 'ری‌استارت' });
        if (!ok) break;
        await api(`/api/agents/${id}/command`, { method: 'POST', body: { action: 'restart' } });
        toast('فرمان ری‌استارت ارسال شد؛ اتصال به‌زودی برمی‌گردد.', 'success');
        break;
      }
      case 'update-agent': {
        const ok = await confirmAction({ title: 'آپدیت ایجنت', message: 'آخرین نسخهٔ Pulse روی سرور دانلود و جایگزین می‌شود؛ به‌روزرسانی در یک واحد موقت systemd اجرا می‌شود و اتصال لحظه‌ای قطع خواهد شد. ادامه می‌دهید؟', confirmText: 'آپدیت' });
        if (!ok) break;
        await api(`/api/agents/${id}/command`, { method: 'POST', body: { action: 'update' } });
        toast('آپدیت ایجنت شروع شد؛ چند لحظه بعد نسخه را بررسی کنید.', 'success');
        break;
      }
      case 'edit-agent': {
        const a = state.agents.find((x) => x.id === id);
        if (a) { closeModal(true); openEditAgent(a); }
        break;
      }
      case 'rotate-agent': {
        const ok = await confirmAction({ title: 'توکن جدید ایجنت', message: 'توکن قبلی بلافاصله باطل می‌شود و باید روی سرور به‌روزرسانی شود. ادامه می‌دهید؟', confirmText: 'ساخت توکن جدید' });
        if (!ok) break;
        const data = await api(`/api/agents/${id}/rotate`, { method: 'POST', body: {} });
        closeModal(true);
        if (location.hash.includes('/servers/')) history.replaceState(null, '', '#/servers');
        openAgentToken(data, 'توکن جدید ساخته شد');
        break;
      }
      case 'delete-agent': {
        const a = state.agents.find((x) => x.id === id);
        const ok = await confirmAction({ title: 'حذف ایجنت', message: `سرور «${a?.name || ''}» و تمام تاریخچه منابع آن پاک می‌شود. ادامه می‌دهید؟`, confirmText: 'حذف کن', danger: true });
        if (!ok) break;
        await api(`/api/agents/${id}`, { method: 'DELETE' });
        closeModal(true);
        history.replaceState(null, '', '#/servers');
        toast('سرور حذف شد', 'success');
        await route();
        break;
      }
      case 'agent-range': {
        const hours = Number(actionEl.dataset.hours) || 24;
        closeModal(true);
        await openAgentDetail(id, { hours });
        break;
      }
      case 'new-rule': openRuleForm(); break;
      case 'edit-rule': {
        const r = state.rules.find((x) => x.id === id);
        if (r) { closeModal(true); openRuleForm(r); }
        break;
      }
      case 'delete-rule': {
        const r = state.rules.find((x) => x.id === id);
        const ok = await confirmAction({ title: 'حذف قانون', message: `قانون «${r?.name || ''}» حذف شود؟`, confirmText: 'حذف', danger: true });
        if (!ok) break;
        await api(`/api/rules/${id}`, { method: 'DELETE' });
        toast('قانون حذف شد', 'success');
        await loadView('rules', null, true);
        break;
      }
      case 'cf-refresh': await loadCloudflareData(); toast('وضعیت کلادفلر بروزرسانی شد', 'info'); break;
      case 'cf-show-token-form': openCFTokenForm(); break;
      case 'cf-add-account': openCFAccountForm(); break;
      case 'cf-edit-account': openCFAccountForm(Number(actionEl.dataset.id) || null); break;
      case 'cf-select-account':
        state.cfAccount = Number(actionEl.dataset.id) || 0;
        state.cfZone = '';
        state.cfRecords = [];
        state.cfError = '';
        await loadCloudflareData();
        break;
      case 'cf-delete-account': {
        const accName = actionEl.dataset.name || 'این حساب';
        const ok = await confirmAction({ title: 'حذف حساب Cloudflare', message: `حساب «${accName}» حذف شود؟ مانیتورهای متصل به آن غیرفعال می‌شوند.`, confirmText: 'حذف حساب', danger: true });
        if (!ok) break;
        await api(`/api/cloudflare/accounts/${id}`, { method: 'DELETE' });
        state.cfAccount = 0;
        toast('حساب Cloudflare حذف شد', 'success');
        await loadCloudflareData();
        break;
      }
      case 'cf-add-record': openCFRecordForm(); break;
      case 'cf-delete-record': {
        const recName = actionEl.dataset.name || 'این رکورد';
        const ok = await confirmAction({ title: 'حذف رکورد DNS', message: `رکورد «${recName}» از Cloudflare حذف شود؟`, confirmText: 'حذف رکورد', danger: true });
        if (!ok) break;
        await api(`/api/cloudflare/records/${encodeURIComponent(actionEl.dataset.id)}?${Number(state.cfAccount) > 0 ? `account_id=${state.cfAccount}&` : ''}zone_id=${encodeURIComponent(state.cfZone)}`, { method: 'DELETE' });
        toast('رکورد DNS حذف شد', 'success');
        await loadCloudflareData();
        break;
      }
      case 'xray-detect': {
        const result = await api('/api/xray/detect', { method: 'POST', body: {} });
        const st = result.status || {};
        toast(st.available ? `Xray ${st.version} شناسایی شد` : (st.error || 'هستهٔ Xray پیدا نشد'), st.available ? 'success' : 'error', 'هستهٔ Xray');
        await reloadSettingsPreserve();
        break;
      }
      case 'cf-disable': {
        const ok = await confirmAction({ title: 'غیرفعال‌سازی اتوماسیون', message: 'سوییچ خودکار DNS غیرفعال شود؟ توکن ذخیره می‌ماند و می‌توانید بعداً دوباره فعال کنید.', confirmText: 'غیرفعال کن' });
        if (!ok) break;
        await api('/api/settings', { method: 'PUT', body: { cloudflare_enabled: false } });
        toast('اتوماسیون DNS غیرفعال شد', 'success');
        await loadCloudflareData();
        break;
      }
      case 'cf-enable': {
        await api('/api/settings', { method: 'PUT', body: { cloudflare_enabled: true } });
        toast('اتوماسیون DNS فعال شد', 'success');
        await loadCloudflareData();
        break;
      }
      case 'agent-guide': openAgentGuide(); break;
      case 'cf-clear-token': {
        const ok = await confirmAction({ title: 'قطع کلادفلر', message: 'توکن ذخیره‌شده پاک و اتوماسیون DNS غیرفعال شود؟', confirmText: 'قطع اتصال', danger: true });
        if (!ok) break;
        await api('/api/settings', { method: 'PUT', body: { cloudflare_token_clear: true, cloudflare_enabled: false } });
        toast('اتصال کلادفلر قطع شد', 'success');
        await loadView('cloudflare', null, false);
        break;
      }
      case 'cf-edit-record': {
        const r = state.cfRecords.find((x) => x.id === actionEl.dataset.id);
        if (r) openCFRecordForm(r);
        break;
      }
      case 'telegram-test': {
        await api('/api/settings/test/telegram', { method: 'POST', body: {} });
        toast('پیام تست به تلگرام ارسال شد', 'success');
        await refreshNotificationsPreserve();
        break;
      }
      case 'webhook-test': {
        await api('/api/settings/test/webhook', { method: 'POST', body: {} });
        toast('درخواست تست وب‌هوک ارسال شد', 'success');
        await refreshNotificationsPreserve();
        break;
      }
      case 'clear-telegram': {
        const ok = await confirmAction({ title: 'قطع تلگرام', message: 'توکن ربات حذف و ارسال تلگرام غیرفعال شود؟', confirmText: 'قطع اتصال', danger: true });
        if (!ok) break;
        await api('/api/settings', { method: 'PUT', body: { telegram_token_clear: true, telegram_enabled: false } });
        toast('اتصال تلگرام قطع شد', 'success');
        await refreshNotificationsPreserve('telegram');
        break;
      }
      case 'clear-webhook': {
        const ok = await confirmAction({ title: 'حذف وب‌هوک', message: 'آدرس وب‌هوک حذف شود؟', confirmText: 'حذف', danger: true });
        if (!ok) break;
        await api('/api/settings', { method: 'PUT', body: { webhook_url: '' } });
        toast('وب‌هوک حذف شد', 'success');
        await refreshNotificationsPreserve('webhook');
        break;
      }
      case 'push-enable': await enablePush(); break;
      case 'push-disable': await disablePush(); break;
      case 'push-test': {
        const res = await api('/api/push/test', { method: 'POST', body: {} });
        toast(`پیام تست ارسال شد (${num(res.sent || 0)} موفق)`, 'success');
        await refreshNotificationsPreserve();
        break;
      }
      case 'change-password': closeUserMenu(); openPasswordForm(); break;
      case 'settings-tab': {
        const tab = actionEl.dataset.tab || 'general';
        state.settingsTab = tab;
        $$('.settings-tab').forEach((el) => {
          const active = el.dataset.tab === tab;
          el.classList.toggle('active', active);
          el.setAttribute('aria-selected', String(active));
        });
        $$('[data-settings-pane]').forEach((pane) => pane.classList.toggle('hidden', pane.dataset.settingsPane !== tab));
        break;
      }
      case 'download-backup': await downloadBackup(); break;
      case 'install-pwa': {
        if (!state.installPrompt) { toast('نصب خودکار در این مرورگر در دسترس نیست. از منوی مرورگر «Add to Home Screen» را انتخاب کنید.', 'info'); break; }
        state.installPrompt.prompt();
        await state.installPrompt.userChoice;
        state.installPrompt = null;
        break;
      }
      case 'notify-placeholder': {
        const form = actionEl.closest('form');
        const area = form?.elements.namedItem('body_template');
        if (area) {
          const value = actionEl.dataset.value || '';
          const start = area.selectionStart ?? area.value.length;
          const end = area.selectionEnd ?? area.value.length;
          area.setRangeText(value, start, end, 'end');
          area.focus();
          area.dispatchEvent(new Event('input', { bubbles: true }));
        }
        break;
      }
      case 'notify-template-reset': {
        const form = actionEl.closest('form');
        if (form) {
          const title = form.elements.namedItem('title_template');
          const body = form.elements.namedItem('body_template');
          if (title) title.value = DEFAULT_NOTIFY_TITLE;
          if (body) body.value = DEFAULT_NOTIFY_BODY;
          notifyTemplatePreview(form);
        }
        break;
      }
      case 'copy': await copyText(textWithoutButtons(document.querySelector(actionEl.dataset.copyTarget))); break;
      default:
        break;
    }
  } catch (err) {
    if (err?.status !== 401) toast(err.message || 'عملیات ناموفق بود', 'error');
  }
}

/* ------------------------------------------------------------------ change / input */
async function onDocumentChange(e) {
  const el = e.target;
  if (el.matches('[data-rule-toggle]')) {
    const id = Number(el.dataset.ruleToggle);
    const rule = state.rules.find((r) => r.id === id);
    if (!rule) return;
    try {
      await api(`/api/rules/${id}`, { method: 'PUT', body: { name: rule.name, enabled: el.checked, trigger: rule.trigger, monitor_id: rule.monitor_id, threshold: rule.threshold, cooldown_sec: rule.cooldown_sec, channels: rule.channels } });
      rule.enabled = el.checked;
      toast(el.checked ? 'قانون فعال شد' : 'قانون غیرفعال شد', 'success');
      el.closest('.rule-card')?.classList.toggle('disabled', !el.checked);
    } catch (err) {
      el.checked = !el.checked;
      toast(err.message, 'error');
    }
    return;
  }
  if (el.id === 'm-vps-threshold-mode') {
    updateVPSThresholdFields($('#monitor-form'));
    return;
  }
  if (el.id === 'm-cf-enabled') {
    const extra = $('#m-cf-extra');
    extra?.classList.toggle('hidden', !el.checked);
    if (el.checked) {
      const form = $('#monitor-form');
      const zoneSel = form?.querySelector('#m-cf-zone');
      if (form && zoneSel && (!zoneSel.options.length || zoneSel.options[0].textContent.includes('در حال'))) hydrateCFSelects(form, state.monitors.find((m) => String(m.id) === form.dataset.id));
    }
    return;
  }
  if (el.id === 'm-cf-account') {
    const form = $('#monitor-form');
    if (!form) return;
    const accountID = Number(el.value) || 0;
    const zoneSel = form.querySelector('#m-cf-zone');
    const note = form.querySelector('#m-cf-note');
    if (zoneSel) zoneSel.innerHTML = '<option value="">در حال دریافت…</option>';
    try {
      const zones = accountID ? await ensureCFZones(accountID) : [];
      if (zoneSel) zoneSel.innerHTML = zones.length
        ? '<option value="">انتخاب Zone…</option>' + zones.map((z) => `<option value="${esc(z.id)}">${esc(z.name)}</option>`).join('')
        : '<option value="">دامنه‌ای پیدا نشد</option>';
      if (zoneSel && zones[0]) {
        zoneSel.value = zones[0].id;
        await populateCFRecordSelect(form, zoneSel.value, '', '', '', accountID);
      } else {
        const recordSel = form.querySelector('#m-cf-record');
        if (recordSel) recordSel.innerHTML = '<option value="">ابتدا Zone را انتخاب کنید</option>';
      }
      if (note) note.textContent = zones.length ? 'رکوردهای A/AAAA حساب انتخاب‌شده.' : 'برای این حساب دامنه‌ای پیدا نشد.';
    } catch (e) {
      if (zoneSel) zoneSel.innerHTML = '<option value="">دریافت دامنه‌ها ناموفق بود</option>';
      if (note) note.textContent = e.message;
    }
    return;
  }
  if (el.id === 'm-cf-zone') {
    const form = $('#monitor-form');
    if (form) await populateCFRecordSelect(form, el.value, '', '', '', Number(form.querySelector('#m-cf-account')?.value) || 0);
    return;
  }
  if (el.id === 'm-cf-record') {
    const opt = el.selectedOptions[0];
    if (opt?.dataset.name) $('#m-cf-primary').value = opt.dataset.content || '';
    return;
  }
  if (el.id === 'cf-zone') {
    const seq = ++state.cfZoneSeq;
    state.cfZone = el.value;
    state.cfRecordType = '';
    state.cfRecords = [];
    state.cfError = '';
    const sc = window.scrollY;
    try {
      const records = await fetchCFRecords(el.value);
      if (seq !== state.cfZoneSeq) return;
      state.cfRecords = records;
    } catch (err) {
      if (seq !== state.cfZoneSeq) return;
      state.cfError = err.message;
    }
    renderCloudflare();
    window.scrollTo({ top: sc });
    return;
  }
  if (el.id === 'cf-record-type') {
    state.cfRecordType = el.value;
    const tbody = $('#cf-records-body');
    if (tbody) tbody.innerHTML = cfRecordRows();
    return;
  }
}
function onDocumentInput(e) {
  if (e.target.id === 'panel-port-input') {
    const cmd = $('#panel-port-command');
    if (cmd) cmd.textContent = `sudo pulse port ${e.target.value || '8080'}`;
    return;
  }
  if (e.target.id === 'monitor-search') {
    state.filters.search = e.target.value;
    refreshMonitorGrid();
    return;
  }
  if (e.target.id === 'cf-search') {
    state.cfSearch = e.target.value;
    const tbody = $('#cf-records-body');
    if (tbody) tbody.innerHTML = cfRecordRows();
  }
}

/* ------------------------------------------------------------------ form submit */
async function onDocumentSubmit(e) {
  const form = e.target.closest('form[data-form]');
  if (!form) return;
  e.preventDefault();
  const mode = form.dataset.form;
  if (mode === 'login' || mode === 'setup') { await submitAuth(form); return; }
  const err = form.querySelector('.form-error');
  const btn = form.querySelector('button[type="submit"]') || (form.id ? document.querySelector(`button[type="submit"][form="${form.id}"]`) : null);
  const old = btn?.innerHTML;
  if (err) { err.classList.remove('show'); err.textContent = ''; }
  if (btn) { btn.disabled = true; btn.innerHTML = 'در حال انجام…'; }
  const fail = (message) => {
    if (err) { err.textContent = message; err.classList.add('show'); err.scrollIntoView({ behavior: 'smooth', block: 'nearest' }); }
    else toast(message, 'error');
  };
  try {
    if (mode === 'monitor') {
      const type = formVal(form, 'type');
      const cfg = collectMonitorConfig(form, type);
      let target = formVal(form, 'target');
      const name = formVal(form, 'name');
      if (!name) throw new Error('نام مانیتور الزامی است');
      if (!target && type === 'vps' && cfg.vps_id) target = `VPS ${cfg.vps_id}`;
      if (!target) throw new Error('آدرس/هدف مانیتور الزامی است');
      const cfEnabled = formChecked(form, 'cf_enabled');
      const recordSel = form.querySelector('#m-cf-record');
      const recOpt = recordSel?.selectedOptions?.[0];
      const cfZone = form.querySelector('#m-cf-zone')?.value || '';
      const cfRecord = recordSel?.value || '';
      const cfAccount = Number(form.querySelector('#m-cf-account')?.value) || 0;
      if (cfEnabled && (!cfZone || !cfRecord)) throw new Error('برای سوییچ خودکار DNS، Zone و رکورد را انتخاب کنید');
      if (cfEnabled && !cfAccount) throw new Error('ابتدا یک حساب Cloudflare انتخاب کنید');
      const body = {
        name, type, target, group: formVal(form, 'group'), config: cfg,
        probe_agent_id: Number(formVal(form, 'probe_agent_id')) || 0,
        interval_sec: Number(formVal(form, 'interval_sec')) || 60,
        timeout_ms: Number(formVal(form, 'timeout_ms')) || 12000,
        enabled: formChecked(form, 'enabled'),
        fail_threshold: Number(formVal(form, 'fail_threshold')) || 2,
        recover_threshold: Number(formVal(form, 'recover_threshold')) || 1,
        cf_enabled: cfEnabled,
        cf_account_id: cfAccount,
        cf_zone_id: cfEnabled ? cfZone : '',
        cf_record_id: cfEnabled ? cfRecord : '',
        cf_record_name: cfEnabled ? (recOpt?.dataset.name || '') : '',
        cf_primary: cfEnabled ? formVal(form, 'cf_primary') : '',
        cf_failover: cfEnabled ? formVal(form, 'cf_failover') : '',
      };
      const id = Number(form.dataset.id) || 0;
      await api(id ? `/api/monitors/${id}` : '/api/monitors', { method: id ? 'PUT' : 'POST', body });
      closeModal(true);
      const base = state.view === 'monitors' ? '#/monitors' : state.view === 'dashboard' ? '#/dashboard' : '#/monitors';
      history.replaceState(null, '', base);
      toast(id ? 'مانیتور به‌روزرسانی شد' : 'مانیتور ساخته شد', 'success');
      await route();
      return;
    }
    if (mode === 'rule') {
      const trigger = formVal(form, 'trigger');
      const name = formVal(form, 'name');
      if (!name) throw new Error('نام قانون الزامی است');
      const channels = $$('input[name="channel"]:checked', form).map((x) => x.value);
      if (!channels.length) throw new Error('حداقل یک کانال را انتخاب کنید');
      const body = {
        name, trigger,
        monitor_id: ['down', 'up', 'degraded', 'cert_expiring', 'dns_mismatch'].includes(trigger) ? Number(formVal(form, 'monitor_id')) || 0 : 0,
        threshold: Number(formVal(form, 'threshold')) || 0,
        cooldown_sec: (Number(formVal(form, 'cooldown')) || 0) * 60,
        channels,
        enabled: formChecked(form, 'enabled'),
      };
      const id = Number(form.dataset.id) || 0;
      await api(id ? `/api/rules/${id}` : '/api/rules', { method: id ? 'PUT' : 'POST', body });
      closeModal(true);
      toast(id ? 'قانون به‌روزرسانی شد' : 'قانون ساخته شد', 'success');
      if (state.view !== 'rules' || location.hash !== '#/rules') history.replaceState(null, '', '#/rules');
      await route();
      return;
    }
    if (mode === 'agent') {
      const data = await api('/api/agents', { method: 'POST', body: { name: formVal(form, 'name') } });
      closeModal(true);
      state.agents.push?.(data.agent);
      toast('ایجنت ساخته شد', 'success');
      history.replaceState(null, '', '#/servers');
      await route();
      openAgentToken(data);
      return;
    }
    if (mode === 'agent-edit') {
      await api(`/api/agents/${form.dataset.id}`, { method: 'PUT', body: { name: formVal(form, 'name'), notes: formVal(form, 'notes') } });
      closeModal(true);
      toast('اطلاعات سرور ذخیره شد', 'success');
      if (location.hash.includes('/servers/')) history.replaceState(null, '', '#/servers');
      await route();
      return;
    }
    if (mode === 'password') {
      const current = formVal(form, 'current');
      const next = formVal(form, 'new');
      if (next.length < 8) throw new Error('رمز جدید باید حداقل 8 کاراکتر باشد');
      if (next !== formVal(form, 'confirm')) throw new Error('تکرار رمز جدید مطابقت ندارد');
      await api('/api/auth/password', { method: 'POST', body: { current_password: current, new_password: next } });
      closeModal(true);
      toast('رمز عبور تغییر کرد', 'success');
      return;
    }
    if (mode === 'cf-account') {
      const id = Number(form.dataset.id) || 0;
      const name = formVal(form, 'name');
      const token = formVal(form, 'token');
      if (!name) throw new Error('نام حساب الزامی است');
      if (!id && !token) throw new Error('توکن API الزامی است');
      const body = { name, enabled: formChecked(form, 'enabled') };
      if (token) body.token = token;
      const data = await api(id ? `/api/cloudflare/accounts/${id}` : '/api/cloudflare/accounts', { method: id ? 'PUT' : 'POST', body });
      if (!id && data.account?.id) state.cfAccount = Number(data.account.id);
      closeModal(true);
      toast(id ? 'حساب Cloudflare به‌روزرسانی شد' : 'حساب Cloudflare اضافه شد', 'success');
      await loadCloudflareData();
      return;
    }
    if (mode === 'cf-record') {
      const id = formVal(form, 'id') || form.dataset.id;
      const body = {
        account_id: Number(formVal(form, 'account_id')) || 0,
        zone_id: formVal(form, 'zone_id'),
        type: formVal(form, 'type'),
        name: formVal(form, 'name'),
        content: formVal(form, 'content'),
        ttl: Number(formVal(form, 'ttl')) || 1,
        proxied: formChecked(form, 'proxied'),
        priority: Number(formVal(form, 'priority')) || 0,
      };
      if (!body.name || !body.content) throw new Error('نام و مقدار رکورد الزامی است');
      await api(id ? `/api/cloudflare/records/${encodeURIComponent(id)}` : '/api/cloudflare/records', { method: id ? 'PUT' : 'POST', body });
      closeModal(true);
      toast(id ? 'رکورد DNS ذخیره شد' : 'رکورد DNS ساخته شد', 'success');
      await loadCloudflareData();
      return;
    }
    if (mode === 'telegram') {
      const body = {
        telegram_enabled: formChecked(form, 'enabled'),
        telegram_chat_id: formVal(form, 'chat_id'),
      };
      const token = formVal(form, 'token');
      if (token) body.telegram_token = token;
      await api('/api/settings', { method: 'PUT', body });
      toast('تنظیمات تلگرام ذخیره شد', 'success');
      await refreshNotificationsPreserve('telegram');
      return;
    }
    if (mode === 'push-settings') {
      await api('/api/settings', { method: 'PUT', body: { webpush_enabled: formChecked(form, 'enabled') } });
      toast('تنظیمات Web Push ذخیره شد', 'success');
      await refreshNotificationsPreserve('push-settings');
      return;
    }
    if (mode === 'webhook') {
      const body = { webhook_url: formVal(form, 'url') };
      const secret = formVal(form, 'secret');
      if (secret) body.webhook_secret = secret;
      await api('/api/settings', { method: 'PUT', body });
      toast('تنظیمات وب‌هوک ذخیره شد', 'success');
      await refreshNotificationsPreserve('webhook');
      return;
    }
    if (mode === 'quiet-hours') {
      await api('/api/settings', {
        method: 'PUT',
        body: {
          quiet_hours_enabled: formChecked(form, 'enabled'),
          quiet_start: formVal(form, 'start'),
          quiet_end: formVal(form, 'end'),
          quiet_allow_critical: formChecked(form, 'critical'),
          notify_repeat_min: Number(formVal(form, 'repeat')) || 0,
        },
      });
      toast('تنظیمات ساعات سکوت ذخیره شد', 'success');
      await refreshNotificationsPreserve('quiet-hours');
      return;
    }
    if (mode === 'notify-template') {
      await api('/api/settings', {
        method: 'PUT',
        body: {
          notify_title_template: formVal(form, 'title_template'),
          notify_body_template: formVal(form, 'body_template'),
        },
      });
      toast('قالب اعلان ذخیره شد', 'success');
      await refreshNotificationsPreserve('notify-template');
      return;
    }
    if (mode === 'settings-general') {
      await api('/api/settings', {
        method: 'PUT',
        body: {
          site_title: formVal(form, 'site_title'),
          timezone: formVal(form, 'timezone'),
          public_base_url: formVal(form, 'public_base_url'),
          dns_resolver: formVal(form, 'dns_resolver'),
          agent_offline_after_sec: Number(formVal(form, 'agent_offline_after_sec')) || 120,
          check_retention_days: Number(formVal(form, 'check_retention_days')) || 14,
          metrics_retention_days: Number(formVal(form, 'metrics_retention_days')) || 30,
          agent_download_url: formVal(form, 'agent_download_url'),
        },
      });
      const siteTitle = formVal(form, 'site_title');
      if (siteTitle) state.auth = { ...(state.auth || {}), site_title: siteTitle };
      toast('تنظیمات سامانه ذخیره شد', 'success');
      await reloadSettingsPreserve('settings-general');
      updateShell('settings');
      return;
    }
    if (mode === 'system-port') {
      const port = Number(formVal(form, 'port'));
      if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error('پورت باید بین 1 و 65535 باشد');
      const ok = await confirmAction({
        title: 'تغییر پورت پنل',
        message: `پنل روی پورت ${port} راه‌اندازی مجدد شود؟ اگر از پروکسی خارجی استفاده می‌کنید، پورت مقصد آن را هم تغییر دهید.`,
        confirmText: 'تغییر پورت',
        danger: true,
      });
      if (!ok) return;
      const res = await api('/api/system/port', { method: 'POST', body: { port } });
      toast(res.message || 'درخواست تغییر پورت ثبت شد', 'success');
      const cmd = $('#panel-port-command');
      if (cmd) cmd.textContent = `sudo pulse port ${port}`;
      return;
    }
    if (mode === 'xray-settings') {
      await api('/api/settings', {
        method: 'PUT',
        body: {
          xray_binary: formVal(form, 'binary') || 'xray',
          xray_test_url: formVal(form, 'test_url'),
        },
      });
      const detected = await api('/api/xray/detect', { method: 'POST', body: {} });
      const st = detected.status || {};
      toast(st.available ? `Xray ${st.version} آماده است` : (st.error || 'Xray پیدا نشد'), st.available ? 'success' : 'error', 'هستهٔ Xray');
      await reloadSettingsPreserve('xray-settings');
      return;
    }
    throw new Error('فرم ناشناخته');
  } catch (err) {
    fail(err.message || 'عملیات ناموفق بود');
  } finally {
    if (btn) { btn.disabled = false; btn.innerHTML = old; }
  }
}

/* ------------------------------------------------------------------ push & backup */
function urlBase64ToUint8Array(base64String) {
  const padding = '='.repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(base64);
  const out = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}
async function enablePush() {
  if (!('serviceWorker' in navigator) || !('PushManager' in window)) throw new Error('این مرورگر از Web Push پشتیبانی نمی‌کند');
  const cfg = state.pushConfig || await api('/api/push/config');
  if (!cfg.public_key) throw new Error('کلید عمومی VAPID آماده نیست');
  if (!('Notification' in window)) throw new Error('Notification API در دسترس نیست');
  if (Notification.permission === 'denied') throw new Error('اعلان‌ها در مرورگر مسدود شده‌اند؛ از تنظیمات سایت اجازه دهید');
  if (Notification.permission !== 'granted') {
    const perm = await Notification.requestPermission();
    if (perm !== 'granted') throw new Error('اجازه اعلان داده نشد');
  }
  await navigator.serviceWorker.register('/sw.js');
  const reg = await navigator.serviceWorker.ready;
  let sub = await reg.pushManager.getSubscription();
  if (!sub) {
    sub = await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: urlBase64ToUint8Array(cfg.public_key) });
  }
  await api('/api/push/subscribe', { method: 'POST', body: sub.toJSON() });
  toast('اعلان مرورگر فعال شد', 'success');
  await refreshNotificationsPreserve();
}
async function disablePush() {
  if ('serviceWorker' in navigator) {
    const reg = await navigator.serviceWorker.getRegistration();
    const sub = reg ? await reg.pushManager.getSubscription() : null;
    if (sub) {
      await api('/api/push/unsubscribe', { method: 'POST', body: { endpoint: sub.endpoint } });
      await sub.unsubscribe();
    }
  }
  toast('اعلان این مرورگر غیرفعال شد', 'success');
  await refreshNotificationsPreserve();
}
async function downloadBackup() {
  const res = await fetch('/api/backup', { credentials: 'same-origin' });
  if (res.status === 401) { sessionExpired(); return; }
  if (!res.ok) throw new Error('تهیه نسخه پشتیبان ناموفق بود');
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `pulse-backup-${new Date().toISOString().slice(0, 10)}.json`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 5000);
  toast('نسخه پشتیبان دانلود شد', 'success');
}

/* end */

/* boot */
init().catch((e) => {
  const boot = $('#boot');
  if (boot) boot.classList.add('done');
  toast(e.message || 'خطای راه‌اندازی', 'error');
  renderAuth('login', e.message || '');
});

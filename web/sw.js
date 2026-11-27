/* Pulse PWA service worker */
const VERSION = 'pulse-v1.6.4';
const SHELL = [
  '/',
  '/index.html',
  '/assets/app.css?v=19',
  '/assets/app.js?v=19',
  '/assets/charts.js?v=19',
  '/assets/i18n.js?v=19',
  '/assets/lang-boot.js?v=19',
  '/fonts/IRANSansWebFaNum.woff2',
  '/fonts/IRANSansWebFaNum_Bold.woff2',
  '/fonts/IRANSansWebFaNum_Medium.woff2',
  '/fonts/IRANSansWebFaNum_Light.woff2',
  '/icons/icon.svg',
  '/icons/icon-192.png',
  '/icons/icon-512.png',
  '/icons/apple-touch-icon.png',
  '/icons/badge-72.png',
  '/manifest.webmanifest'
];

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(VERSION).then((cache) => {
      // Cache entries individually so one optional asset cannot prevent the
      // service worker from installing.
      return Promise.all(SHELL.map((url) => cache.add(url).catch(() => undefined)));
    }).then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) => Promise.all(keys.filter((k) => k !== VERSION).map((k) => caches.delete(k)))).then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', (event) => {
  const request = event.request;
  if (request.method !== 'GET') return;
  const url = new URL(request.url);
  if (url.origin !== location.origin) return;
  if (url.pathname.startsWith('/api/')) return;

  if (request.mode === 'navigate') {
    event.respondWith(
      fetch(request).then((res) => {
        if (res && res.ok) {
          const copy = res.clone();
          event.waitUntil(caches.open(VERSION).then((cache) => cache.put('/index.html', copy)).catch(() => undefined));
        }
        return res;
      }).catch(() => caches.match('/index.html').then((r) => r || caches.match('/')).then((r) => r || Response.error()))
    );
    return;
  }

  event.respondWith(
    (async () => {
      const cacheFirst = url.pathname.startsWith('/fonts/') || url.pathname.startsWith('/icons/');
      if (cacheFirst) {
        const cached = await caches.match(request);
        if (cached) return cached;
      }
      try {
        const res = await fetch(request);
        if (res && res.status === 200) {
          const copy = res.clone();
          event.waitUntil(caches.open(VERSION).then((cache) => cache.put(request, copy)).catch(() => undefined));
        }
        return res;
      } catch {
        const cached = await caches.match(request);
        if (cached) return cached;
        return new Response('', { status: 504, statusText: 'Offline' });
      }
    })()
  );
});

self.addEventListener('push', (event) => {
  let data = {};
  try { data = event.data ? event.data.json() : {}; } catch { data = { body: event.data ? event.data.text() : '' }; }
  const title = data.title || 'پالس';
  const options = {
    body: data.body || 'رویداد جدیدی در پالس ثبت شد',
    icon: data.icon || '/icons/icon-192.png',
    badge: data.badge || '/icons/badge-72.png',
    tag: data.tag || 'pulse',
    renotify: !!data.tag,
    data: { url: data.url || '/#/dashboard', timestamp: data.timestamp || Date.now() },
    dir: 'rtl',
    lang: 'fa',
    vibrate: [90, 40, 120],
    timestamp: data.timestamp || Date.now()
  };
  event.waitUntil(self.registration.showNotification(title, options));
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const target = (event.notification.data && event.notification.data.url) || '/#/dashboard';
  const absolute = new URL(target, self.location.origin).href;
  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clients) => {
      for (const client of clients) {
        if (client.url.startsWith(self.location.origin)) {
          client.navigate(absolute);
          return client.focus();
        }
      }
      return self.clients.openWindow(absolute);
    })
  );
});

self.addEventListener('message', (event) => {
  if (event.data === 'skip-waiting') self.skipWaiting();
});

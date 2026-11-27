/* Set the document direction and saved sidebar state before first paint. Kept
   as an external file so the strict CSP (script-src 'self') does not block it. */
(function () {
  try {
    if (localStorage.getItem('pulse-lang') === 'en') {
      document.documentElement.lang = 'en';
      document.documentElement.dir = 'ltr';
      document.documentElement.dataset.lang = 'en';
    }
    if (localStorage.getItem('pulse-sidebar-collapsed') === '1') {
      document.documentElement.classList.add('sidebar-collapsed');
    }
  } catch (e) { /* private mode */ }
})();

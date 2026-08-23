// Applies the persisted theme before the first paint, so the page never
// flashes the wrong palette.
//
// This has to run BEFORE the stylesheet is applied to the document, which is
// why it is a blocking <script> at the top of <head> and not part of app.js.
// It used to be inline there — and the CSP (`script-src 'self'`) blocked it on
// every load, so the flash it exists to prevent came back and the console
// carried a violation on every page view. A same-origin file is allowed by the
// same CSP with no exception, no nonce and no hash.
(function () {
  try {
    var t = localStorage.getItem('myserver-theme');
    if (!t) {
      t = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    }
    document.documentElement.classList.add(t);
    var c = localStorage.getItem('myserver-color') || 'slate';
    document.documentElement.classList.add('theme-' + c);
  } catch (e) {
    /* private mode, or storage disabled: fall through to the served markup */
  }
})();

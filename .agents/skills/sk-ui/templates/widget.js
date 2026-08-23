// Template for a bespoke behaviour in web/static/js/app.js.
//
// CONSTRAINTS, all enforced by the environment rather than by taste:
//
//   - The CSP is `script-src 'self' unpkg.com cdn.jsdelivr.net`. No inline
//     handlers (onclick/onsubmit/onerror), no inline <script> with content.
//     Everything is addEventListener, from this file.
//   - Templ does NOT interpolate inside <script>, so server data cannot be
//     baked in. It arrives on `data-*` attributes; read it here.
//   - No bundler, no modules, no npm. One IIFE per file, ES5-flavoured (`var`,
//     `function`) to match the surrounding code.
//   - Never build DOM from user data with innerHTML. Service and bookmark names
//     come from YAML: createElement + textContent, always.
//
// Register the setup function in the DOMContentLoaded block at the bottom of
// app.js, next to setupDatetime / setupGreeting / setupSearch.

(function () {
  'use strict';

  function setupExampleWidget() {
    var widget = document.getElementById('example-widget');
    var display = document.getElementById('example-widget-display');
    // Bail quietly: the widget is optional and most pages will not have it.
    if (!widget || !display) return;

    // Server data arrives as attributes. Always provide a fallback so a missing
    // one degrades instead of rendering "null".
    var locale = widget.getAttribute('data-locale') || 'en-US';
    var emptyLabel = widget.getAttribute('data-empty-label') || 'Loading...';

    function update() {
      try {
        display.textContent = new Date().toLocaleString(locale, {
          dateStyle: 'medium',
          timeStyle: 'short',
        });
      } catch (e) {
        // Intl can throw on an unknown locale. Never leave the widget blank.
        display.textContent = emptyLabel;
      }
    }

    update();
    setInterval(update, 1000);
  }

  // Building a list from user data. The only safe shape:
  function renderExampleRows(container, items) {
    container.replaceChildren();

    items.slice(0, 8).forEach(function (item) {
      var a = document.createElement('a');
      a.href = item.href || '#';
      a.target = '_blank';
      a.rel = 'noreferrer noopener';
      a.className = 'flex items-center gap-2 px-3 py-2 text-sm hover:bg-theme-100 dark:hover:bg-theme-700';
      // textContent, NOT innerHTML: a service named `O'Brien & Sons <x>` must
      // not be able to inject markup, and would break the string anyway.
      a.textContent = item.name;
      container.appendChild(a);
    });
  }

  // Class names used ONLY here are invisible to Tailwind's scanner unless the
  // file is in `content` — web/static/js/*.js is, so literal strings like the
  // className above are fine. A name ASSEMBLED at runtime
  // ('bg-theme-' + shade) is not: add a safelist RegExp in tailwind.config.js.

  // Anything that polls must stop when the tab is hidden, the same rule the
  // HTMX triggers follow.
  function setupExamplePolling() {
    var intervalId = null;
    function start() {
      if (intervalId) return;
      intervalId = setInterval(function () {
        fetch('/api/example', { credentials: 'same-origin' })
          .then(function (r) { return r.json(); })
          .then(function (data) { /* … */ })
          .catch(function () {});
      }, 10000);
    }
    function stop() {
      if (intervalId) { clearInterval(intervalId); intervalId = null; }
    }
    document.addEventListener('visibilitychange', function () {
      if (document.visibilityState === 'visible') start(); else stop();
    });
    if (document.visibilityState === 'visible') start();
  }

  document.addEventListener('DOMContentLoaded', function () {
    setupExampleWidget();
    setupExamplePolling();
  });
})();

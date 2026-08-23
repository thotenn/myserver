// MyServer — Main Application JS
(function () {
  'use strict';

  // ---------------------------------------------------------------------
  // Datetime widget — driven by data-locale attribute set server-side.
  // ---------------------------------------------------------------------
  function setupDatetime() {
    var widget = document.getElementById('datetime-widget');
    var display = document.getElementById('datetime-display');
    if (!widget || !display) return;
    var locale = widget.getAttribute('data-locale') || 'en-US';
    function update() {
      var now = new Date();
      try {
        display.textContent = now.toLocaleString(locale, {
          dateStyle: 'medium',
          timeStyle: 'short',
        });
      } catch (e) {
        display.textContent = now.toString();
      }
    }
    update();
    setInterval(update, 1000);
  }

  // ---------------------------------------------------------------------
  // Greeting widget — strings come from data-* attributes set server-side
  // so they are properly i18n'd without needing template interpolation
  // inside <script> blocks.
  // ---------------------------------------------------------------------
  function setupGreeting() {
    var widget = document.getElementById('greeting-widget');
    var display = document.getElementById('greeting-display');
    if (!widget || !display) return;
    var morning = widget.getAttribute('data-morning') || 'Good morning';
    var afternoon = widget.getAttribute('data-afternoon') || 'Good afternoon';
    var evening = widget.getAttribute('data-evening') || 'Good evening';
    var night = widget.getAttribute('data-night') || 'Good night';
    function update() {
      var hour = new Date().getHours();
      var msg;
      if (hour >= 5 && hour < 12) msg = morning;
      else if (hour >= 12 && hour < 18) msg = afternoon;
      else if (hour >= 18 && hour < 21) msg = evening;
      else msg = night;
      display.textContent = msg;
    }
    update();
    setInterval(update, 60 * 1000);
  }

  // ---------------------------------------------------------------------
  // Base path: the dashboard can be served under a URL prefix, so any URL
  // built here has to carry it. It arrives as a <meta> tag because Templ
  // emits interpolations inside <script> literally — same reason the
  // datetime and greeting widgets read data-* attributes.
  // ---------------------------------------------------------------------
  function basePath() {
    var meta = document.querySelector('meta[name="base-path"]');
    return (meta && meta.getAttribute('content')) || '';
  }

  // ---------------------------------------------------------------------
  // Config hot-reload: poll /api/hash every 10s, refresh when changed.
  // Stops polling when the tab is hidden to save battery/CPU.
  // ---------------------------------------------------------------------
  function setupHotReload() {
    var meta = document.querySelector('meta[name="config-hash"]');
    if (!meta) return;
    var initial = meta.getAttribute('content');
    if (!initial) return;
    var intervalId = null;
    function startPolling() {
      if (intervalId) return;
      intervalId = setInterval(function () {
        fetch(basePath() + '/api/hash', { credentials: 'same-origin' })
          .then(function (r) { return r.json(); })
          .then(function (data) {
            if (data && data.hash && data.hash !== initial) {
              window.location.reload();
            }
          })
          .catch(function () {});
      }, 10000);
    }
    function stopPolling() {
      if (intervalId) {
        clearInterval(intervalId);
        intervalId = null;
      }
    }
    document.addEventListener('visibilitychange', function () {
      if (document.visibilityState === 'visible') {
        startPolling();
      } else {
        stopPolling();
      }
    });
    if (document.visibilityState === 'visible') {
      startPolling();
    }
  }

  // ---------------------------------------------------------------------
  // HTMX setup: error handling + Content-Type validation.
  // ---------------------------------------------------------------------
  function setupHTMX() {
    if (typeof htmx === 'undefined') return;
    // Pause polling while the tab is hidden.
    //
    // This used to live in the markup as
    // `hx-trigger="every 30s [document.visibilityState === 'visible']"`, but
    // htmx compiles a trigger filter with `new Function(...)`, and the CSP
    // (`script-src 'self'`, no 'unsafe-eval') blocks that: every polled
    // element logged an EvalError on load and lost its filter, so the page
    // kept polling in a hidden tab anyway — the opposite of the intent, plus
    // a console full of violations. Doing the check here needs no eval.
    document.body.addEventListener('htmx:beforeRequest', function (evt) {
      if (!document.hidden) return;
      var trigger = evt.detail.elt && evt.detail.elt.getAttribute('hx-trigger');
      if (trigger && trigger.indexOf('every ') !== -1) {
        evt.preventDefault();
      }
    });
    document.body.addEventListener('htmx:responseError', function (evt) {
      evt.target.classList.add('widget-error');
      setTimeout(function () {
        evt.target.classList.remove('widget-error');
      }, 5000);
    });
    document.body.addEventListener('htmx:beforeRequest', function (evt) {
      evt.target.classList.remove('widget-error');
    });
    // Reject swaps when the response is not HTML — defense against
    // accidentally returning JSON to an innerHTML target.
    document.body.addEventListener('htmx:beforeSwap', function (evt) {
      var ct = evt.detail.xhr.getResponseHeader('Content-Type') || '';
      if (!/text\/html/i.test(ct)) {
        evt.detail.shouldSwap = false;
        evt.target.classList.add('widget-error');
        // Surface a hint in dev tools without breaking the page.
        if (window.console) {
          console.error('Refusing swap: expected text/html, got', ct, evt.detail.xhr);
        }
      }
    });
  }

  document.addEventListener('DOMContentLoaded', function () {
    setupHTMX();
    setupDatetime();
    setupGreeting();
    setupHotReload();
    setupEventListeners();
    setupIconErrors();
  });

  // ---------------------------------------------------------------------
  // Event listeners — attached via JS so inline handlers (onclick,
  // onsubmit, oninput, etc.) can be removed from templates, keeping CSP
  // strict (no 'unsafe-inline' in script-src).
  // ---------------------------------------------------------------------
  function setupEventListeners() {
    // Theme toggle
    var themeToggle = document.getElementById('theme-toggle');
    if (themeToggle && window.toggleTheme) {
      themeToggle.addEventListener('click', function () {
        window.toggleTheme();
      });
    }

    // Unified search bar: the top-right input acts as both a web-search
    // form (Enter → external engine) and a live QuickLaunch (typing filters
    // services and bookmarks into a dropdown anchored under the input).
    var searchForm = document.getElementById('search-form');
    var searchInput = document.getElementById('search-input');
    if (searchForm && searchInput) {
      searchInput.addEventListener('input', function () {
        renderSearchSuggestions(searchInput);
      });
      searchInput.addEventListener('keydown', function (event) {
        handleSearchKey(event, searchInput);
      });
      searchInput.addEventListener('blur', function () {
        // Defer so a click on a suggestion (which blurs the input) still
        // gets handled before the dropdown closes.
        setTimeout(closeSearchResults, 150);
      });
      searchForm.addEventListener('submit', function (event) {
        event.preventDefault();
        // If a suggestion is highlighted, "Enter" navigates to it.
        var highlighted = document.querySelector(
          '#search-results [data-suggestion].is-active',
        );
        if (highlighted && highlighted.getAttribute('href')) {
          highlighted.click();
          return;
        }
        submitSearchQuery(searchInput);
      });
    }
  }

  // ---------------------------------------------------------------------
  // Icon error handling — hides images that fail to load (CSP-safe
  // replacement for inline onerror="this.style.display='none'").
  // ---------------------------------------------------------------------
  function setupIconErrors() {
    document.querySelectorAll('.bookmark-icon, .service-icon, .script-icon').forEach(function (img) {
      img.addEventListener('error', function () {
        img.style.display = 'none';
      });
    });
  }

  // ---------------------------------------------------------------------
  // Unified search bar — combines web search and live QuickLaunch.
  //
  //   Type     → filter services + bookmarks live, show in dropdown
  //   ↓ / ↑    → navigate suggestions
  //   Enter    → if a suggestion is highlighted, follow it; else web search
  //   Esc      → close dropdown and blur
  //   Click    → follow the suggestion or "search the web" entry
  //
  // DOM construction uses createElement + textContent (no innerHTML
  // interpolation of user input) to keep the CSP strict and avoid
  // self-XSS via apostrophes or HTML in service names.
  // ---------------------------------------------------------------------
  function closeSearchResults() {
    var results = document.getElementById('search-results');
    var input = document.getElementById('search-input');
    if (results) {
      results.classList.add('hidden');
      results.replaceChildren();
    }
    if (input) input.setAttribute('aria-expanded', 'false');
  }

  function submitSearchQuery(input) {
    var query = input.value.trim();
    if (!query) return;
    if (/^(https?:\/\/|www\.)/.test(query)) {
      window.open(
        query.indexOf('http') === 0 ? query : 'https://' + query,
        '_blank',
      );
    } else {
      // The engine comes from the form itself — its action and the input's
      // name are rendered server-side from `widgets.yaml: search`. Hardcoding
      // Google here is what made the `provider:` setting do nothing.
      var form = document.getElementById('search-form');
      var action = (form && form.getAttribute('action')) || 'https://www.google.com/search';
      var param = input.getAttribute('name') || 'q';
      var target = (form && form.getAttribute('target')) || '_self';
      var url = action + (action.indexOf('?') === -1 ? '?' : '&') +
        param + '=' + encodeURIComponent(query);
      if (target === '_self') {
        window.location.assign(url);
      } else {
        window.open(url, target);
      }
    }
    input.value = '';
    closeSearchResults();
  }

  function highlightSuggestion(results, delta) {
    var suggestions = results.querySelectorAll('[data-suggestion]');
    if (suggestions.length === 0) return;
    var current = results.querySelector('[data-suggestion].is-active');
    var idx = -1;
    suggestions.forEach(function (el, i) { if (el === current) idx = i; });
    idx = (idx + delta + suggestions.length) % suggestions.length;
    suggestions.forEach(function (el) { el.classList.remove('is-active', 'bg-theme-100', 'dark:bg-theme-700'); });
    suggestions[idx].classList.add('is-active', 'bg-theme-100', 'dark:bg-theme-700');
    // Keep the active item in view inside the scrollable results box.
    suggestions[idx].scrollIntoView({ block: 'nearest' });
  }

  function renderSearchSuggestions(input) {
    var query = input.value.trim().toLowerCase();
    var results = document.getElementById('search-results');
    if (!results) return;
    if (query.length < 2) {
      closeSearchResults();
      return;
    }

    var items = [];
    document.querySelectorAll('[data-service]').forEach(function (el) {
      var name = el.getAttribute('data-service') || '';
      if (!name.toLowerCase().includes(query)) return;
      var link = el.querySelector('a[href]');
      items.push({
        name: name,
        href: link ? link.getAttribute('href') : '#',
        type: 'service',
      });
    });
    document.querySelectorAll('[data-bookmark-group]').forEach(function (el) {
      el.querySelectorAll('a').forEach(function (a) {
        var name = a.textContent.trim();
        if (!name.toLowerCase().includes(query)) return;
        items.push({
          name: name,
          href: a.getAttribute('href'),
          type: 'bookmark',
        });
      });
    });

    results.replaceChildren();

    var rowBase =
      'flex items-center gap-2 px-3 py-2 text-sm cursor-pointer hover:bg-theme-100 dark:hover:bg-theme-700 transition-colors';

    items.slice(0, 8).forEach(function (item) {
      var a = document.createElement('a');
      a.href = item.href || '#';
      a.target = '_blank';
      a.rel = 'noreferrer noopener';
      a.className = rowBase;
      a.setAttribute('data-suggestion', '');
      a.setAttribute('role', 'option');
      a.addEventListener('mousedown', function () {
        // mousedown fires before blur; ensures the click goes through.
      });
      var type = document.createElement('span');
      type.className =
        'text-[10px] text-theme-500 dark:text-theme-400 uppercase tracking-wide';
      type.textContent = item.type;
      var name = document.createElement('span');
      name.className = 'text-theme-900 dark:text-theme-100';
      name.textContent = item.name;
      a.appendChild(type);
      a.appendChild(name);
      results.appendChild(a);
    });

    // Always append a "search the web" fallback row so Enter never feels
    // like a dead end even when there are matches above.
    var fallback = document.createElement('div');
    fallback.className =
      rowBase + ' border-t border-theme-200 dark:border-theme-700';
    fallback.setAttribute('data-suggestion', '');
    fallback.setAttribute('role', 'option');
    var fbType = document.createElement('span');
    fbType.className =
      'text-[10px] text-theme-500 dark:text-theme-400 uppercase tracking-wide';
    fbType.textContent = 'web';
    var fbText = document.createElement('span');
    fbText.className = 'text-theme-900 dark:text-theme-100';
    fbText.textContent = 'Search the web for "' + query + '"';
    fallback.appendChild(fbType);
    fallback.appendChild(fbText);
    fallback.addEventListener('mousedown', function (ev) {
      ev.preventDefault();
      submitSearchQuery(input);
    });
    results.appendChild(fallback);

    results.classList.remove('hidden');
    input.setAttribute('aria-expanded', 'true');
  }

  function handleSearchKey(event, input) {
    var results = document.getElementById('search-results');
    if (!results) return;
    if (event.key === 'Escape') {
      closeSearchResults();
      input.blur();
      return;
    }
    if (results.classList.contains('hidden')) return;
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      highlightSuggestion(results, +1);
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      highlightSuggestion(results, -1);
    }
  }

  document.addEventListener('click', function (e) {
    var results = document.getElementById('search-results');
    var input = document.getElementById('search-input');
    if (
      results &&
      input &&
      !results.contains(e.target) &&
      e.target !== input
    ) {
      closeSearchResults();
    }
  });
})();

// MyServer — Theme Toggle
(function() {
  'use strict';

  const STORAGE_KEY = 'myserver-theme';
  const COLOR_KEY = 'myserver-color';

  function getSystemTheme() {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }

  function getStoredTheme() {
    return localStorage.getItem(STORAGE_KEY);
  }

  function setTheme(theme) {
    document.documentElement.classList.remove('light', 'dark');
    document.documentElement.classList.add(theme);
    localStorage.setItem(STORAGE_KEY, theme);
  }

  function getColor() {
    return localStorage.getItem(COLOR_KEY) || 'slate';
  }

  function setColor(color) {
    // Remove all theme-* classes
    document.documentElement.className
      .split(' ')
      .filter(c => c.startsWith('theme-'))
      .forEach(c => document.documentElement.classList.remove(c));

    document.documentElement.classList.add('theme-' + color);
    localStorage.setItem(COLOR_KEY, color);
  }

  function toggleTheme() {
    const current = document.documentElement.classList.contains('dark') ? 'dark' : 'light';
    setTheme(current === 'dark' ? 'light' : 'dark');
  }

  // Make toggleTheme available globally
  window.toggleTheme = toggleTheme;

  // Initialize theme
  const stored = getStoredTheme();
  if (stored) {
    setTheme(stored);
  } else {
    setTheme(getSystemTheme());
  }

  // Initialize color
  setColor(getColor());

  // Listen for system theme changes
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
    if (!getStoredTheme()) {
      setTheme(e.matches ? 'dark' : 'light');
    }
  });
})();

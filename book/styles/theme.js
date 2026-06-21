(function() {
  'use strict';

  var STORAGE_KEY = 'fds-theme';
  var DARK = 'dark';
  var LIGHT = 'light';

  function getTheme() {
    var saved = localStorage.getItem(STORAGE_KEY);
    if (saved === DARK || saved === LIGHT) return saved;
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? DARK : LIGHT;
  }

  function applyTheme(theme) {
    document.documentElement.setAttribute('data-theme', theme);
    localStorage.setItem(STORAGE_KEY, theme);
    updateIcon(theme);
  }

  function updateIcon(theme) {
    var btn = document.getElementById('theme-toggle-btn');
    if (!btn) return;
    btn.textContent = theme === DARK ? '☀️' : '🌙';
    btn.setAttribute('aria-label', theme === DARK ? 'Switch to light mode' : 'Switch to dark mode');
  }

  function createToggle() {
    var btn = document.createElement('button');
    btn.id = 'theme-toggle-btn';
    btn.className = 'theme-toggle';
    btn.setAttribute('aria-label', 'Toggle theme');
    btn.onclick = function() {
      var current = document.documentElement.getAttribute('data-theme');
      applyTheme(current === DARK ? LIGHT : DARK);
    };
    document.body.appendChild(btn);
  }

  function init() {
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', function() {
        createToggle();
        applyTheme(getTheme());
      });
    } else {
      createToggle();
      applyTheme(getTheme());
    }
  }

  init();
})();

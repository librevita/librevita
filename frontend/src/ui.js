// LibreVita frontend bootstrap.
// Written in modern JavaScript; esbuild transpiles it to the XP floor
// (target=firefox52). Load order in base.templ: htmx, sse extension,
// alpine-csp, then this bundle.

import './compat.js';

// HTMX: fail closed on dynamic evaluation and never accept scripts from
// swapped fragments.
htmx.config.allowEval = false;
htmx.config.allowScriptTags = false;
htmx.config.includeIndicatorStyles = false;

// Forward the double-submit CSRF token on every state-changing request.
document.addEventListener('htmx:configRequest', (evt) => {
  const token = readCookie('lv_csrf');
  if (token) {
    evt.detail.headers['X-CSRF-Token'] = token;
  }
});

// The Alpine CSP build does not auto-start. Components register on the
// alpine:init event, which fires before the reactive tree initializes.
document.addEventListener('alpine:init', () => {
  // Widget components are registered here as they are introduced.
});

if (window.Alpine) {
  Alpine.start();
}

function readCookie(name) {
  const parts = document.cookie.split(';');
  for (let i = 0; i < parts.length; i++) {
    const entry = parts[i].trim();
    if (entry.indexOf(name + '=') === 0) {
      return entry.substring(name.length + 1);
    }
  }
  return '';
}

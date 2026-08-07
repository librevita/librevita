// LibreVita frontend bootstrap.
// Written in TypeScript; esbuild bundles it to the XP floor
// (target=firefox58). Load order in base.templ: htmx, sse extension,
// ui.js, then alpine-csp.

import './compat.ts';

// HTMX: fail closed on dynamic evaluation and never accept scripts from
// swapped fragments.
htmx.config.allowEval = false;
htmx.config.allowScriptTags = false;
htmx.config.includeIndicatorStyles = false;

// Forward the double-submit CSRF token on every state-changing request.
document.addEventListener('htmx:configRequest', (evt) => {
  const detail = (evt as CustomEvent<HtmxRequestDetail>).detail;
  const token = readCookie('lv_csrf');
  if (token) {
    detail.headers['X-CSRF-Token'] = token;
  }
});

// The Alpine CSP cdn build auto-starts when it loads (after this bundle),
// so components must register before that happens. The alpine:init event
// fires at the start of Alpine initialization.
document.addEventListener('alpine:init', () => {
  const alpine = window.Alpine;
  if (!alpine) {
    return;
  }
  // Mobile navigation drawer. The aside is always visible from md up via
  // md:!flex (important beats the x-show inline style).
  alpine.data('sidebar', () => ({
    open: false,
    toggle() {
      this.open = !this.open;
    },
    close() {
      this.open = false;
    },
  }));

  // Topbar user menu.
  alpine.data('userMenu', () => ({
    open: false,
    toggle() {
      this.open = !this.open;
    },
    close() {
      this.open = false;
    },
  }));

  // Card tab switcher. Active tab classes are produced by a method so the
  // CSP build does not evaluate inline object literals.
  alpine.data('tabs', () => ({
    active: 'users',
    set(name: string) {
      this.active = name;
    },
    tabClass(name: string, activeClass: string) {
      return this.active === name ? activeClass : '';
    },
  }));
});

function readCookie(name: string): string {
  const parts = document.cookie.split(';');
  for (let i = 0; i < parts.length; i++) {
    const entry = parts[i].trim();
    if (entry.indexOf(name + '=') === 0) {
      return entry.substring(name.length + 1);
    }
  }
  return '';
}

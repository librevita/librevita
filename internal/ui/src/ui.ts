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

// hx-boost replaces the body on navigation. Alpine only initializes the
// original tree, so the swapped content must be initialized explicitly.
// The old DOM is discarded, so initTree cannot double-initialize.
document.addEventListener('htmx:afterSwap', (evt) => {
  const detail = (evt as CustomEvent<HtmxAfterSwapDetail>).detail;
  if (detail.elt) {
    window.Alpine?.initTree(detail.elt);
    const main = document.getElementById('app-main');
    if (main && (main as HTMLElement).focus) {
      (main as HTMLElement).focus({ preventScroll: true });
    }
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

  // Color scheme preference on the profile page. The initial class comes
  // from theme.js (head); this persists the user choice. 'system' removes
  // the stored value so theme.js follows the OS scheme again.
  alpine.data('themePref', () => ({
    pref: readThemePref(),
    set(pref: string) {
      this.pref = pref;
      try {
        if (pref === 'system') {
          localStorage.removeItem('librevita-theme');
        } else {
          localStorage.setItem('librevita-theme', pref);
        }
      } catch (_) {
        // Private mode: preference lasts for the session.
      }
      applyThemePref(pref);
    },
    prefClass(name: string, activeClass: string) {
      return this.pref === name ? activeClass : '';
    },
  }));
});

function readThemePref(): string {
  try {
    const stored = localStorage.getItem('librevita-theme');
    if (stored === 'light' || stored === 'dark') {
      return stored;
    }
  } catch (_) {
    // Private mode.
  }
  return 'system';
}

function applyThemePref(pref: string): void {
  if (pref === 'system') {
    const dark = typeof window.matchMedia === 'function' &&
      window.matchMedia('(prefers-color-scheme: dark)').matches;
    document.documentElement.classList.toggle('dark', dark);
  } else {
    document.documentElement.classList.toggle('dark', pref === 'dark');
  }
}

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

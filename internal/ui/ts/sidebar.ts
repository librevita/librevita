// Sidebar drawer: opened by the hamburger on small screens
// (data-lv-drawer + data-lv-backdrop, <md). The hidden class provides
// the SSR-correct initial state (closed), so this module only reacts to
// clicks.

const TOGGLE_SELECTOR = '[data-lv-nav-toggle]';
const DRAWER_SELECTOR = '[data-lv-drawer]';
const BACKDROP_SELECTOR = '[data-lv-backdrop]';
const OPEN_ICON_SELECTOR = '[data-lv-nav-open-icon]';
const CLOSE_ICON_SELECTOR = '[data-lv-nav-close-icon]';

function setOpen(open: boolean): void {
  const drawer = document.querySelector(DRAWER_SELECTOR);
  if (drawer) {
    drawer.classList.toggle('hidden', !open);
  }
  const backdrop = document.querySelector(BACKDROP_SELECTOR);
  if (backdrop) {
    backdrop.classList.toggle('hidden', !open);
  }
  const toggle = document.querySelector<HTMLElement>(TOGGLE_SELECTOR);
  if (toggle) {
    toggle.setAttribute('aria-expanded', String(open));
    toggle.setAttribute('aria-label', open ? 'Close navigation' : 'Open navigation');
  }
  document.querySelector(OPEN_ICON_SELECTOR)?.classList.toggle('hidden', open);
  document.querySelector(CLOSE_ICON_SELECTOR)?.classList.toggle('hidden', !open);
}

export function init(): void {
  document.addEventListener('click', (evt) => {
    if (closestOrSelf(evt.target, TOGGLE_SELECTOR)) {
      evt.preventDefault();
      const drawer = document.querySelector(DRAWER_SELECTOR);
      setOpen(drawer?.classList.contains('hidden') ?? true);
      return;
    }
    const drawer = document.querySelector(DRAWER_SELECTOR);
    if (drawer && !drawer.classList.contains('hidden') &&
        !closestOrSelf(evt.target, DRAWER_SELECTOR)) {
      setOpen(false);
    }
  });
}

export function refresh(_root: HTMLElement): void {
  // Delegated listener survives swaps; the drawer state is transient.
}

import { closestOrSelf } from './core.ts';

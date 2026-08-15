// Sidebar drawer: opened by the hamburger on small and medium screens
// (data-lv-drawer, <xl). The hidden class provides the SSR-correct
// initial state (closed), so this module only reacts to clicks.

const TOGGLE_SELECTOR = '[data-lv-nav-toggle]';
const DRAWER_SELECTOR = '[data-lv-drawer]';

export function init(): void {
  document.addEventListener('click', (evt) => {
    const toggle = closestOrSelf(evt.target, TOGGLE_SELECTOR);
    if (toggle) {
      evt.preventDefault();
      const drawer = document.querySelector(DRAWER_SELECTOR);
      const open = drawer?.classList.contains('hidden') ?? true;
      if (drawer) {
        drawer.classList.toggle('hidden', !open);
      }
      toggle.setAttribute('aria-expanded', String(open));
      return;
    }
    const drawer = document.querySelector(DRAWER_SELECTOR);
    if (drawer && !drawer.classList.contains('hidden') &&
        !closestOrSelf(evt.target, DRAWER_SELECTOR)) {
      drawer.classList.add('hidden');
      document.querySelector<HTMLElement>(TOGGLE_SELECTOR)?.setAttribute('aria-expanded', 'false');
    }
  });
}

export function refresh(_root: HTMLElement): void {
  // Delegated listener survives swaps; the drawer state is transient.
}

import { closestOrSelf } from './core.ts';

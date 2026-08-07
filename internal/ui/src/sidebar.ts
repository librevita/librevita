// Sidebar drawer: visible from md up via md:!flex; toggled by the
// hamburger on small screens. The hidden class provides the SSR-correct
// initial state (closed), so this module only reacts to clicks.

const TOGGLE_SELECTOR = '[data-lv-nav-toggle]';
const DRAWER_SELECTOR = '[data-lv-drawer]';

export function init(): void {
  document.addEventListener('click', (evt) => {
    const toggle = closestOrSelf(evt.target, TOGGLE_SELECTOR);
    if (toggle) {
      evt.preventDefault();
      const drawer = document.querySelector(DRAWER_SELECTOR);
      if (drawer) {
        drawer.classList.toggle('hidden');
      }
      return;
    }
    const drawer = document.querySelector(DRAWER_SELECTOR);
    if (drawer && !drawer.classList.contains('hidden') &&
        !closestOrSelf(evt.target, DRAWER_SELECTOR)) {
      drawer.classList.add('hidden');
    }
  });
}

export function refresh(_root: HTMLElement): void {
  // Delegated listener survives swaps; the drawer state is transient.
}

import { closestOrSelf } from './core.ts';

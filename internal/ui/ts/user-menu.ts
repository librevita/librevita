// Topbar user menu: a hidden dropdown toggled by its button, closed on
// outside clicks. The hidden class is the SSR-correct initial state.

const TOGGLE_SELECTOR = '[data-lv-user-menu-toggle]';
const MENU_SELECTOR = '[data-lv-user-menu]';

export function init(): void {
  document.addEventListener('click', (evt) => {
    const toggle = closestOrSelf(evt.target, TOGGLE_SELECTOR);
    const menu = document.querySelector(MENU_SELECTOR);
    if (!menu) {
      return;
    }
    if (toggle) {
      evt.preventDefault();
      const isOpen = menu.classList.toggle('hidden') === false;
      toggle.setAttribute('aria-expanded', isOpen ? 'true' : 'false');
      return;
    }
    if (!menu.classList.contains('hidden') &&
        !closestOrSelf(evt.target, MENU_SELECTOR)) {
      menu.classList.add('hidden');
    }
  });
}

export function refresh(_root: HTMLElement): void {
  // The dropdown is hidden by default; delegated clicks handle the rest.
}

import { closestOrSelf } from './core.ts';

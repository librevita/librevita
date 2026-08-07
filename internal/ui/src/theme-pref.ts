// Color scheme preference on the profile page (light, system, dark).
// The initial class comes from theme.js (head); this persists the user
// choice and applies the active button styling. 'system' removes the
// stored value so theme.js follows the OS scheme again.

const GROUP_SELECTOR = '[data-lv-theme-pref]';
const OPTION_SELECTOR = '[data-lv-theme]';
const THEME_KEY = 'librevita-theme';

export function init(): void {
  document.addEventListener('click', (evt) => {
    const option = closestOrSelf(evt.target, OPTION_SELECTOR);
    if (!option) {
      return;
    }
    const pref = option.getAttribute('data-lv-theme');
    if (!pref) {
      return;
    }
    setPreference(pref);
    applyActiveClass(option);
  });
  syncActiveClass();
}

export function refresh(_root: HTMLElement): void {
  syncActiveClass();
}

export function readStored(): string {
  try {
    const stored = localStorage.getItem(THEME_KEY);
    if (stored === 'light' || stored === 'dark') {
      return stored;
    }
  } catch (_) {
    // Private mode.
  }
  return 'system';
}

export function applyThemeClass(pref: string): void {
  if (pref === 'system') {
    const dark = typeof window.matchMedia === 'function' &&
      window.matchMedia('(prefers-color-scheme: dark)').matches;
    document.documentElement.classList.toggle('dark', dark);
  } else {
    document.documentElement.classList.toggle('dark', pref === 'dark');
  }
}

function setPreference(pref: string): void {
  try {
    if (pref === 'system') {
      localStorage.removeItem(THEME_KEY);
    } else {
      localStorage.setItem(THEME_KEY, pref);
    }
  } catch (_) {
    // Private mode: preference lasts for the session.
  }
  applyThemeClass(pref);
}

function syncActiveClass(): void {
  const group = document.querySelector(GROUP_SELECTOR);
  if (!group) {
    return;
  }
  const pref = readStored();
  const options = group.querySelectorAll(OPTION_SELECTOR);
  for (let i = 0; i < options.length; i++) {
    if (options[i].getAttribute('data-lv-theme') === pref) {
      applyActiveClass(options[i]);
    }
  }
}

function applyActiveClass(selected: Element): void {
  const group = selected.closest(GROUP_SELECTOR);
  if (!group) {
    return;
  }
  const activeClass = group.getAttribute('data-lv-active-class') ?? '';
  const options = group.querySelectorAll(OPTION_SELECTOR);
  for (let i = 0; i < options.length; i++) {
    if (options[i] === selected) {
      options[i].classList.add(...activeClass.split(' '));
    } else {
      options[i].classList.remove(...activeClass.split(' '));
    }
  }
}

import { closestOrSelf } from './core.ts';

// Dashboard card tabs. The server renders the initial active tab; this
// module switches the panel visibility and the active classes on click.

const GROUP_SELECTOR = '[data-lv-tabs]';
const TAB_SELECTOR = '[data-lv-tab]';
const PANEL_SELECTOR = '[data-lv-panel]';

export function init(): void {
  document.addEventListener('click', (evt) => {
    const tab = closestOrSelf(evt.target, TAB_SELECTOR);
    if (!tab || !tab.closest(GROUP_SELECTOR)) {
      return;
    }
    evt.preventDefault();
    const group = tab.closest(GROUP_SELECTOR);
    if (!group) {
      return;
    }
    const name = tab.getAttribute('data-lv-tab');
    if (!name) {
      return;
    }
    selectTab(group, name);
  });
}

export function refresh(_root: HTMLElement): void {
  // The server re-renders the initial tab state; nothing to resync.
}

export function selectTab(group: Element, name: string): void {
  const activeClass = group.getAttribute('data-lv-active-class') ?? '';
  const tabs = group.querySelectorAll(TAB_SELECTOR);
  for (let i = 0; i < tabs.length; i++) {
    const tab = tabs[i];
    const tabName = tab.getAttribute('data-lv-tab');
    if (tabName === name) {
      tab.classList.add(...activeClass.split(' '));
    } else {
      tab.classList.remove(...activeClass.split(' '));
    }
  }
  const panels = group.querySelectorAll(PANEL_SELECTOR);
  for (let i = 0; i < panels.length; i++) {
    panels[i].classList.toggle(
      'hidden',
      panels[i].getAttribute('data-lv-panel') !== name,
    );
  }
}

import { closestOrSelf } from './core.ts';

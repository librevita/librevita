// Unit tests for the component modules. linkedom provides the DOM shim;
// the modules under test only touch the DOM inside init()/refresh(), so
// the pure helpers are asserted directly.

import assert from 'node:assert/strict';
import { test } from 'node:test';
import { Event, parseHTML } from 'linkedom';
import { selectTab } from './tabs.ts';
import { rowBoxes } from './table-select.ts';
import * as dropdown from './dropdown.ts';

test('tabs.selectTab toggles panels and active classes', () => {
  const { document } = parseHTML(
    '<div data-lv-tabs data-lv-active-class="act">' +
      '<button data-lv-tab="users" class="act">Users</button>' +
      '<button data-lv-tab="policies">Policies</button>' +
      '<div data-lv-panel="users">u</div>' +
      '<div data-lv-panel="policies">p</div>' +
      '</div>',
  );
  const group = document.querySelector('[data-lv-tabs]') as Element;
  selectTab(group, 'policies');

  const tabs = group.querySelectorAll('[data-lv-tab]');
  assert.equal(tabs[0].className, '');
  assert.equal(tabs[1].className, 'act');
  const panels = group.querySelectorAll('[data-lv-panel]');
  assert.equal(panels[0].classList.contains('hidden'), true);
  assert.equal(panels[1].classList.contains('hidden'), false);
});

test('tableSelect.rowBoxes excludes the select-all box', () => {
  const { document } = parseHTML(
    '<div data-lv-table-select>' +
      '<input type="checkbox" name="select_all">' +
      '<input type="checkbox" name="ids" value="a">' +
      '<input type="checkbox" name="ids" value="b">' +
      '</div>',
  );
  const group = document.querySelector('[data-lv-table-select]') as Element;
  assert.equal(rowBoxes(group).length, 2);
});

test('theme bootstrap applies the server-provided theme class', async () => {
  const { document, window } = parseHTML('<html data-ui-theme="dark"></html>');
  const origDocument = globalThis.document;
  const origWindow = globalThis.window;
  (globalThis as Record<string, unknown>).document = document;
  (globalThis as Record<string, unknown>).window = window;
  try {
    // theme.ts is a side-effect script (no exports); it reads the
    // server-provided attribute and applies the class at import time.
    // @ts-expect-error side-effect module
    await import('./theme.ts');
    assert.equal(document.documentElement.classList.contains('dark'), true);
  } finally {
    (globalThis as Record<string, unknown>).document = origDocument;
    (globalThis as Record<string, unknown>).window = origWindow;
  }
});

test('dropdown opens pinned to the viewport and closes outside', () => {
  const { document } = parseHTML(
    '<div data-lv-dropdown>' +
      '<button type="button" data-lv-dropdown-toggle>...</button>' +
      '<div data-lv-dropdown-menu class="hidden"></div>' +
      '</div>' +
      '<button id="outside">x</button>',
  );
  (globalThis as unknown as { document: Document }).document = document;
  (globalThis as unknown as { window: unknown }).window = { innerWidth: 1024, innerHeight: 768 };
  dropdown.init();

  const toggle = document.querySelector('[data-lv-dropdown-toggle]') as HTMLElement;
  const menu = document.querySelector('[data-lv-dropdown-menu]') as HTMLElement;
  toggle.dispatchEvent(new Event('click', { bubbles: true }) as unknown as globalThis.Event);
  assert.equal(menu.classList.contains('hidden'), false);
  assert.equal(menu.style.position, 'fixed');

  const outside = document.getElementById('outside') as HTMLElement;
  outside.dispatchEvent(new Event('click', { bubbles: true }) as unknown as globalThis.Event);
  assert.equal(menu.classList.contains('hidden'), true);
});

/// <reference lib="deno.ns" />
// Unit tests for the component modules. linkedom provides the DOM shim;
// the modules under test only touch the DOM inside init()/refresh(), so
// the pure helpers are asserted directly.

import { assertEquals } from 'jsr:@std/assert';
import { parseHTML } from 'npm:linkedom';
import { selectTab } from './tabs.ts';
import { rowBoxes } from './table-select.ts';
import { applyThemeClass, readStored } from './theme-pref.ts';

Deno.test('tabs.selectTab toggles panels and active classes', () => {
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
  assertEquals(tabs[0].className, '');
  assertEquals(tabs[1].className, 'act');
  const panels = group.querySelectorAll('[data-lv-panel]');
  assertEquals(panels[0].classList.contains('hidden'), true);
  assertEquals(panels[1].classList.contains('hidden'), false);
});

Deno.test('tableSelect.rowBoxes excludes the select-all box', () => {
  const { document } = parseHTML(
    '<div data-lv-table-select>' +
      '<input type="checkbox" name="select_all">' +
      '<input type="checkbox" name="ids" value="a">' +
      '<input type="checkbox" name="ids" value="b">' +
      '</div>',
  );
  const group = document.querySelector('[data-lv-table-select]') as Element;
  assertEquals(rowBoxes(group).length, 2);
});

Deno.test('themePref.readStored falls back to system', () => {
  assertEquals(readStored(), 'system');
});

Deno.test('themePref.applyThemeClass sets the dark class', () => {
  const { document, window } = parseHTML('<html></html>');
  const orig = globalThis.document;
  const origWindow = globalThis.window;
  (globalThis as Record<string, unknown>).document = document;
  (globalThis as Record<string, unknown>).window = window;
  try {
    applyThemeClass('dark');
    assertEquals(document.documentElement.classList.contains('dark'), true);
    applyThemeClass('light');
    assertEquals(document.documentElement.classList.contains('dark'), false);
  } finally {
    (globalThis as Record<string, unknown>).document = orig;
    (globalThis as Record<string, unknown>).window = origWindow;
  }
});

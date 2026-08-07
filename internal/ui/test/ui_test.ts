// Unit tests for the component modules. linkedom provides the DOM shim;
// the modules under test only touch the DOM inside init()/refresh(), so
// the pure helpers are asserted directly.

import assert from 'node:assert/strict';
import { test } from 'node:test';
import { parseHTML } from 'linkedom';
import { selectTab } from '../src/tabs.ts';
import { rowBoxes } from '../src/table-select.ts';
import { applyThemeClass, readStored } from '../src/theme-pref.ts';

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

test('themePref.readStored falls back to system', () => {
  assert.equal(readStored(), 'system');
});

test('themePref.applyThemeClass sets the dark class', () => {
  const { document, window } = parseHTML('<html></html>');
  const origDocument = globalThis.document;
  const origWindow = globalThis.window;
  (globalThis as Record<string, unknown>).document = document;
  (globalThis as Record<string, unknown>).window = window;
  try {
    applyThemeClass('dark');
    assert.equal(document.documentElement.classList.contains('dark'), true);
    applyThemeClass('light');
    assert.equal(document.documentElement.classList.contains('dark'), false);
  } finally {
    (globalThis as Record<string, unknown>).document = origDocument;
    (globalThis as Record<string, unknown>).window = origWindow;
  }
});

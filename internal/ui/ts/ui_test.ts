// Unit tests for the component modules. linkedom provides the DOM shim;
// the modules under test only touch the DOM inside init()/refresh(), so
// the pure helpers are asserted directly.

import './test-stubs.ts';

import assert from 'node:assert/strict';
import { test } from 'node:test';
import { readFileSync } from 'node:fs';
import { Event, parseHTML } from 'linkedom';
import { selectTab } from './tabs.ts';
import * as dropdown from './dropdown.ts';
import { searchFieldLabel, searchPlaceholder } from './search-menu.ts';
import * as modal from './modal.ts';
import { parseISO, formatISODate } from './datepicker.ts';

function setDocument(): Document {
  const { document, window } = parseHTML('<html><body></body></html>');
  (globalThis as unknown as Record<string, unknown>).document = document;
  (globalThis as unknown as Record<string, unknown>).Node = window.Node;
  return document;
}

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

test('dropdown.pinMenu pins the menu to the viewport and resetMenu clears it', () => {
  const { document } = parseHTML(
    '<div data-lv-dropdown>' +
      '<button type="button" data-lv-dropdown-toggle>...</button>' +
      '<div data-lv-dropdown-menu></div>' +
      '</div>',
  );
  (globalThis as unknown as { document: Document }).document = document;
  (globalThis as unknown as { window: unknown }).window = { innerWidth: 1024, innerHeight: 768 };

  const toggle = document.querySelector('[data-lv-dropdown-toggle]') as HTMLElement;
  const menu = document.querySelector('[data-lv-dropdown-menu]') as HTMLElement;
  menu.style.display = 'block'; // what Alpine's x-show applies
  // linkedom's getBoundingClientRect is all zeros; assert the pinning
  // mechanics, not the exact coordinates.
  dropdown.pinMenu(toggle, menu);
  assert.equal(menu.style.position, 'fixed');
  assert.notEqual(menu.style.left, '');
  assert.notEqual(menu.style.top, '');

  dropdown.resetMenu(menu);
  assert.equal(menu.style.position, '');
  assert.equal(menu.style.left, '');
  assert.equal(menu.style.top, '');
});

test('dropdown.keyboardTarget moves focus through items with arrows, Home and End', () => {
  const { document } = parseHTML(
    '<div data-lv-dropdown>' +
      '<button type="button" data-lv-dropdown-toggle>toggle</button>' +
      '<div data-lv-dropdown-menu>' +
      '<a href="#" id="i1">a</a>' +
      '<button id="i2">b</button>' +
      '<button id="i3">c</button>' +
      '</div>' +
      '</div>',
  );
  const menu = document.querySelector('[data-lv-dropdown-menu]') as HTMLElement;
  const toggle = document.querySelector('[data-lv-dropdown-toggle]') as HTMLElement;
  const items = Array.from(menu.querySelectorAll('a[href], button')) as HTMLElement[];
  const i1 = items[0];
  const i2 = items[1];
  const i3 = items[2];

  assert.equal(dropdown.keyboardTarget(items, toggle, toggle, 'ArrowDown'), i1);
  assert.equal(dropdown.keyboardTarget(items, toggle, i1, 'ArrowDown'), i2);
  assert.equal(dropdown.keyboardTarget(items, toggle, i3, 'ArrowDown'), i1);
  assert.equal(dropdown.keyboardTarget(items, toggle, i1, 'ArrowUp'), i3);
  assert.equal(dropdown.keyboardTarget(items, toggle, toggle, 'ArrowUp'), i3);
  assert.equal(dropdown.keyboardTarget(items, toggle, i2, 'Home'), i1);
  assert.equal(dropdown.keyboardTarget(items, toggle, i1, 'End'), i3);
  assert.equal(dropdown.keyboardTarget(items, toggle, document.body, 'ArrowDown'), null);
  assert.equal(dropdown.keyboardTarget([], toggle, toggle, 'ArrowDown'), null);
  assert.equal(dropdown.keyboardTarget(items, toggle, i1, 'Escape'), null);
});

test('searchMenu.searchFieldLabel maps the scope to the button label', () => {
  assert.equal(searchFieldLabel(''), 'All fields');
  assert.equal(searchFieldLabel('name'), 'Name');
  assert.equal(searchFieldLabel('email'), 'Email');
  assert.equal(searchFieldLabel('document'), 'All fields');
  const systems = {
    'urn:librevita:id:br:cpf': 'CPF (Brasil)',
    'urn:librevita:id:pt:nif': 'NIF (Portugal)',
  };
  assert.equal(searchFieldLabel('urn:librevita:id:br:cpf', systems), 'CPF (Brasil)');
  assert.equal(searchFieldLabel('urn:librevita:id:pt:nif', systems), 'NIF (Portugal)');
  assert.equal(searchFieldLabel('urn:unknown', systems), 'All fields');
});

test('searchMenu.searchPlaceholder invites the exact lookup for a document type', () => {
  assert.equal(searchPlaceholder(''), 'Search patients...');
  assert.equal(searchPlaceholder('name'), 'Search patients...');
  assert.equal(searchPlaceholder('email'), 'Search patients...');
  const systems = { 'urn:librevita:id:br:cpf': 'CPF (Brasil)' };
  assert.equal(searchPlaceholder('urn:librevita:id:br:cpf', systems), 'Exact CPF (Brasil) lookup');
  assert.equal(searchPlaceholder('urn:unknown', systems), 'Search patients...');
});

test('datepicker.parseISO accepts ISO dates and rejects everything else', () => {
  assert.deepEqual(parseISO('2025-12-25'), new Date(2025, 11, 25));
  assert.deepEqual(parseISO('2024-02-29'), new Date(2024, 1, 29));
  assert.equal(parseISO(''), null);
  assert.equal(parseISO('2025-02-30'), null);
  assert.equal(parseISO('2025-13-01'), null);
  assert.equal(parseISO('2025-0-01'), null);
  assert.equal(parseISO('12/25/2025'), null);
  assert.equal(parseISO('2025-12-25T10:00:00'), null);
});

test('datepicker.formatISODate pads to the ISO layout', () => {
  assert.equal(formatISODate(new Date(2025, 0, 5)), '2025-01-05');
  assert.equal(formatISODate(new Date(2025, 11, 25)), '2025-12-25');
  assert.equal(formatISODate(parseISO('2025-01-05') as Date), '2025-01-05');
});

test('datepicker.ts source avoids syntax the legacy floor cannot parse', () => {
  const code = readFileSync(new URL('./datepicker.ts', import.meta.url), 'utf8');
  assert.equal(code.match(/\?\.|\?\?=|\?\?|\bcatch\s*\{/), null);
});

test('modal.openModal applies the a11y state and scroll lock, closeModal reverses it', () => {
  const { document } = parseHTML(
    '<body>' +
      '<div id="dlg" data-lv-modal aria-labelledby="dlg-title" class="hidden">' +
      '<h3 id="dlg-title">Title</h3>' +
      '<p id="dlg-desc" data-lv-modal-description>Description</p>' +
      '</div>' +
      '</body>',
  );
  (globalThis as unknown as { document: Document }).document = document;

  const modalEl = document.getElementById('dlg') as HTMLElement;
  modal.openModal(modalEl);
  assert.equal(modalEl.getAttribute('aria-hidden'), 'false');
  assert.equal(modalEl.getAttribute('aria-modal'), 'true');
  assert.equal(modalEl.getAttribute('aria-describedby'), 'dlg-desc');
  assert.equal(document.body.style.overflow, 'hidden');

  modal.closeModal(modalEl);
  assert.equal(modalEl.getAttribute('aria-hidden'), 'true');
  assert.equal(modalEl.getAttribute('aria-modal'), null);
  assert.equal(modalEl.getAttribute('aria-describedby'), null);
  assert.equal(document.body.style.overflow, '');
});

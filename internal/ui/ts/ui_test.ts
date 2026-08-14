// Unit tests for the component modules. linkedom provides the DOM shim;
// the modules under test only touch the DOM inside init()/refresh(), so
// the pure helpers are asserted directly.

import assert from 'node:assert/strict';
import { test } from 'node:test';
import { Event, parseHTML } from 'linkedom';
import { selectTab } from './tabs.ts';
import { rowBoxes } from './table-select.ts';
import * as dropdown from './dropdown.ts';
import * as modal from './modal.ts';
import { parseISO, formatISO, daysInMonth, isSameDay, addMonths, addDays, monthLabel, buildGrid } from './datepicker.ts';

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

test('datepicker.formatISO pads to the ISO layout', () => {
  assert.equal(formatISO(new Date(2025, 0, 5)), '2025-01-05');
  assert.equal(formatISO(new Date(2025, 11, 25)), '2025-12-25');
  assert.equal(formatISO(parseISO('2025-01-05') as Date), '2025-01-05');
});

test('datepicker.daysInMonth handles leap years', () => {
  assert.equal(daysInMonth(2025, 0), 31);
  assert.equal(daysInMonth(2025, 1), 28);
  assert.equal(daysInMonth(2024, 1), 29);
  assert.equal(daysInMonth(2025, 11), 31);
});

test('datepicker.isSameDay compares calendar days', () => {
  assert.equal(isSameDay(new Date(2025, 11, 25, 23, 59), new Date(2025, 11, 25, 0, 0)), true);
  assert.equal(isSameDay(new Date(2025, 11, 25), new Date(2025, 11, 26)), false);
  assert.equal(isSameDay(new Date(2025, 11, 25), new Date(2024, 11, 25)), false);
});

test('datepicker.addMonths clamps day-of-month', () => {
  assert.deepEqual(addMonths(new Date(2025, 0, 15), 1), new Date(2025, 1, 15));
  assert.deepEqual(addMonths(new Date(2025, 11, 25), 1), new Date(2026, 0, 25));
  assert.deepEqual(addMonths(new Date(2025, 0, 15), -1), new Date(2024, 11, 15));
  assert.deepEqual(addMonths(new Date(2025, 0, 31), 1), new Date(2025, 1, 28));
  assert.deepEqual(addMonths(new Date(2024, 0, 31), 1), new Date(2024, 1, 29));
});

test('datepicker.addDays shifts across month boundaries', () => {
  assert.deepEqual(addDays(new Date(2025, 11, 31), 1), new Date(2026, 0, 1));
  assert.deepEqual(addDays(new Date(2026, 0, 1), -1), new Date(2025, 11, 31));
  assert.deepEqual(addDays(new Date(2025, 11, 25), 7), new Date(2026, 0, 1));
});

test('datepicker.monthLabel renders the header text', () => {
  assert.equal(monthLabel(2025, 0), 'January 2025');
  assert.equal(monthLabel(2025, 11), 'December 2025');
});

test('datepicker.buildGrid returns 42 cells starting on the Sunday before the first', () => {
  // December 2025 starts on a Monday.
  const grid = buildGrid(2025, 11);
  assert.equal(grid.length, 42);
  assert.deepEqual(grid[0], new Date(2025, 10, 30));
  assert.deepEqual(grid[1], new Date(2025, 11, 1));
  assert.deepEqual(grid[31], new Date(2025, 11, 31));
  assert.deepEqual(grid[41], new Date(2026, 0, 10));
  // A month starting on a Sunday (March 2026) starts the grid itself.
  const march = buildGrid(2026, 2);
  assert.deepEqual(march[0], new Date(2026, 2, 1));
});

test('modal opens, locks scroll and closes on Escape', () => {
  const { document } = parseHTML(
    '<body>' +
      '<button id="trigger" data-lv-modal-open="dlg">open</button>' +
      '<div id="dlg" data-lv-modal class="hidden">' +
      '<button id="first">a</button>' +
      '<button id="last">b</button>' +
      '</div>' +
      '</body>',
  );
  (globalThis as unknown as { document: Document }).document = document;
  (globalThis as unknown as { window: unknown }).window = { innerWidth: 1024, innerHeight: 768 };
  modal.init();

  const modalEl = document.getElementById('dlg') as HTMLElement;
  const trigger = document.getElementById('trigger') as HTMLElement;

  trigger.dispatchEvent(new Event('click', { bubbles: true }) as unknown as globalThis.Event);
  assert.equal(modalEl.classList.contains('hidden'), false);
  assert.equal(modalEl.getAttribute('aria-modal'), 'true');
  assert.equal(document.body.style.overflow, 'hidden');

  const esc = new Event('keydown', { bubbles: true }) as unknown as globalThis.Event;
  (esc as unknown as { key: string }).key = 'Escape';
  document.dispatchEvent(esc);
  assert.equal(modalEl.classList.contains('hidden'), true);
  assert.equal(modalEl.getAttribute('aria-modal'), null);
  assert.equal(document.body.style.overflow, '');
});

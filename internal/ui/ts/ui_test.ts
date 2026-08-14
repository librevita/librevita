// Unit tests for the component modules. linkedom provides the DOM shim;
// the modules under test only touch the DOM inside init()/refresh(), so
// the pure helpers are asserted directly.

import assert from 'node:assert/strict';
import { test } from 'node:test';
import { readFileSync } from 'node:fs';
import { Event, parseHTML } from 'linkedom';
import { selectTab } from './tabs.ts';
import { rowBoxes } from './table-select.ts';
import * as dropdown from './dropdown.ts';
import * as modal from './modal.ts';
import { parseISO, formatISO, daysInMonth, isSameDay, addMonths, addDays, monthLabel, buildGrid } from './datepicker.ts';
import { h, type Children, type Component } from './jsx.ts';
import { buildPanel, dayCell, type DayCellProps } from './datepicker.tsx';

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

test('jsx.h renders tags and attributes', () => {
  setDocument();
  const el = h('button', {
    className: 'btn',
    id: 'b',
    type: 'button',
    'data-lv-reveal': 'x',
    'hx-get': '/reveal',
    'aria-label': 'Show value',
  });
  assert.equal(el.tagName, 'BUTTON');
  assert.equal(el.className, 'btn');
  assert.equal(el.id, 'b');
  assert.equal(el.getAttribute('type'), 'button');
  assert.equal(el.getAttribute('data-lv-reveal'), 'x');
  assert.equal(el.getAttribute('hx-get'), '/reveal');
  assert.equal(el.getAttribute('aria-label'), 'Show value');
});

test('jsx.h keeps aria booleans as string attributes', () => {
  setDocument();
  const on = h('button', { 'aria-pressed': true });
  const off = h('button', { 'aria-pressed': false });
  assert.equal(on.getAttribute('aria-pressed'), 'true');
  assert.equal(off.getAttribute('aria-pressed'), 'false');
});

test('jsx.h accepts style as object or string', () => {
  setDocument();
  const obj = h('div', { style: { position: 'fixed', left: '8px' } });
  assert.equal(obj.style.position, 'fixed');
  assert.equal(obj.style.left, '8px');
  const str = h('div', { style: 'color:red' });
  assert.equal(str.getAttribute('style'), 'color:red');
});

test('jsx.h maps dataset into data attributes', () => {
  setDocument();
  const el = h('button', { dataset: { lvReveal: 'x' } });
  assert.equal(el.getAttribute('data-lv-reveal'), 'x');
});

test('jsx.h flattens children and skips empty values', () => {
  setDocument();
  const span = document.createElement('span');
  span.textContent = 'x';
  const el = h('div', null, ['a', ['b', 'c'], null, false, undefined, 42, span]);
  assert.equal(el.textContent, 'abc42x');
  assert.equal(el.lastChild, span);
});

test('jsx.h registers on* handlers', () => {
  setDocument();
  let clicked = 0;
  const el = h('button', { onClick: () => { clicked += 1; } });
  el.dispatchEvent(new Event('click', { bubbles: true }) as unknown as globalThis.Event);
  assert.equal(clicked, 1);
});

test('jsx.h assigns properties for form state keys', () => {
  setDocument();
  const input = h('input', { value: 'v', hidden: true });
  assert.equal(input.value, 'v');
  assert.equal(input.hidden, true);
  const enabled = h('input', { disabled: false });
  assert.equal(enabled.hasAttribute('disabled'), false);
  assert.equal(enabled.disabled, false);
});

test('jsx.h passes normalized children to components', () => {
  setDocument();
  const wrap: Component = (props) => h('div', null, props.children);
  const single = h(wrap, null, 'one');
  assert.equal(single.textContent, 'one');
  const multi = h(wrap, null, 'one', h('span', null, 'two'));
  assert.equal(multi.textContent, 'onetwo');
});

test('jsx.ts source avoids syntax the XP floor cannot parse', () => {
  const code = readFileSync(new URL('./jsx.ts', import.meta.url), 'utf8');
  assert.equal(code.match(/\?\.|\?\?=|\?\?|\bcatch\s*\{/), null);
});

test('datepicker buildPanel renders the popover skeleton', () => {
  setDocument();
  const panel = buildPanel();
  assert.equal(panel.tagName, 'DIV');
  assert.equal(panel.getAttribute('data-lv-datepicker-panel'), '');
  assert.equal(panel.getAttribute('role'), 'dialog');
  assert.equal(panel.getAttribute('aria-label'), 'Date picker');
  assert.equal(panel.classList.contains('hidden'), true);
  const prev = panel.querySelector('[data-lv-datepicker-prev]') as HTMLElement | null;
  assert.ok(prev);
  assert.equal(prev.getAttribute('aria-label'), 'Previous month');
  const next = panel.querySelector('[data-lv-datepicker-next]') as HTMLElement | null;
  assert.ok(next);
  assert.equal(next.getAttribute('aria-label'), 'Next month');
  assert.equal(panel.querySelectorAll('[data-lv-datepicker-label]').length, 1);
  assert.equal(panel.querySelectorAll('[data-lv-datepicker-grid]').length, 1);
  const clear = panel.querySelector('[data-lv-datepicker-clear]') as HTMLElement | null;
  assert.ok(clear);
  assert.equal(clear.textContent, 'Clear');
  const todayBtn = panel.querySelector('[data-lv-datepicker-today]') as HTMLElement | null;
  assert.ok(todayBtn);
  assert.equal(todayBtn.textContent, 'Today');
  const weekdays = Array.from(panel.querySelectorAll('span')).map((s) => s.textContent);
  assert.deepEqual(weekdays, ['Su', 'Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa']);
});

test('datepicker dayCell renders the day states', () => {
  setDocument();
  const today = new Date(2025, 11, 25);
  const base = {
    day: new Date(2025, 11, 15),
    viewMonth: 11,
    selected: null,
    today,
    min: null,
    max: null,
    focused: null,
  };
  const make = (overrides: Partial<DayCellProps>) =>
    dayCell(Object.assign({}, base, overrides));

  const plain = make({}) as HTMLButtonElement;
  assert.equal(plain.getAttribute('data-lv-datepicker-day'), '');
  assert.equal(plain.getAttribute('data-date'), '2025-12-15');
  assert.equal(plain.getAttribute('aria-label'), '2025-12-15');
  assert.equal(plain.textContent, '15');
  assert.equal(plain.disabled, false);
  assert.equal(plain.getAttribute('aria-selected'), null);
  assert.equal(plain.className.includes('text-gray-900'), true);

  const selected = make({ selected: new Date(2025, 11, 15) });
  assert.equal(selected.getAttribute('aria-selected'), 'true');
  assert.equal(selected.className.includes('bg-indigo-600'), true);

  const disabled = make({ min: new Date(2025, 11, 20) }) as HTMLButtonElement;
  assert.equal(disabled.disabled, true);
  assert.equal(disabled.className.includes('cursor-not-allowed'), true);

  const outOfMonth = make({ day: new Date(2025, 10, 30) });
  assert.equal(outOfMonth.className.includes('text-gray-400'), true);

  const focused = make({ focused: new Date(2025, 11, 15) });
  assert.equal(focused.className.includes('ring-2'), true);

  const isToday = make({ day: new Date(2025, 11, 25) });
  assert.equal(isToday.className.includes('text-indigo-600'), true);
});

test('modal opens, locks scroll and closes on Escape', () => {
  const { document } = parseHTML(
    '<body>' +
      '<button id="trigger" data-lv-modal-open="dlg">open</button>' +
      '<div id="dlg" data-lv-modal aria-labelledby="dlg-title" class="hidden">' +
      '<h3 id="dlg-title">Title</h3>' +
      '<p id="dlg-desc" data-lv-modal-description>Description</p>' +
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
  assert.equal(modalEl.getAttribute('aria-labelledby'), 'dlg-title');

  trigger.dispatchEvent(new Event('click', { bubbles: true }) as unknown as globalThis.Event);
  assert.equal(modalEl.classList.contains('hidden'), false);
  assert.equal(modalEl.getAttribute('aria-modal'), 'true');
  assert.equal(modalEl.getAttribute('aria-describedby'), 'dlg-desc');
  assert.equal(document.body.style.overflow, 'hidden');

  const esc = new Event('keydown', { bubbles: true }) as unknown as globalThis.Event;
  (esc as unknown as { key: string }).key = 'Escape';
  document.dispatchEvent(esc);
  assert.equal(modalEl.classList.contains('hidden'), true);
  assert.equal(modalEl.getAttribute('aria-modal'), null);
  assert.equal(modalEl.getAttribute('aria-describedby'), null);
  assert.equal(document.body.style.overflow, '');
});

// Datepicker styled after the Flowbite datepicker: a text input with a
// calendar icon opens a month calendar popover (previous/next month
// navigation, a 6x7 day grid, Today/Clear actions). The popover is
// built lazily per container by this module; the server only renders
// the wrapper and the input. Dates use the ISO yyyy-mm-dd format, which
// is what the backend expects. init() uses delegated listeners so htmx
// swaps are covered automatically; refresh is a no-op for the same
// reason.

const ROOT_SELECTOR = '[data-lv-datepicker]';
const INPUT_SELECTOR = '[data-lv-datepicker-input]';
const PANEL_SELECTOR = '[data-lv-datepicker-panel]';
const DAY_SELECTOR = '[data-lv-datepicker-day]';
const PREV_SELECTOR = '[data-lv-datepicker-prev]';
const NEXT_SELECTOR = '[data-lv-datepicker-next]';
const TODAY_SELECTOR = '[data-lv-datepicker-today]';
const CLEAR_SELECTOR = '[data-lv-datepicker-clear]';
const PREV_ATTR = 'data-lv-datepicker-prev';
const NEXT_ATTR = 'data-lv-datepicker-next';

const MONTHS = [
  'January',
  'February',
  'March',
  'April',
  'May',
  'June',
  'July',
  'August',
  'September',
  'October',
  'November',
  'December',
];

const WEEKDAYS = ['Su', 'Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa'];

// View state per container: the year/month the calendar currently shows.
const viewByRoot = new WeakMap<HTMLElement, { year: number; month: number }>();
// The day highlighted by keyboard navigation, when different from the
// selection.
const focusedByRoot = new WeakMap<HTMLElement, Date>();

export function init(): void {
  document.addEventListener('click', (evt) => {
    const target = evt.target as HTMLElement;
    const input = target.closest(INPUT_SELECTOR) as HTMLElement | null;
    if (input) {
      const root = input.closest(ROOT_SELECTOR) as HTMLElement | null;
      if (root) {
        const panel = panelFor(root);
        if (panel.classList.contains('hidden')) {
          closeAll();
          open(root, panel);
        } else {
          closeAll();
        }
      }
      return;
    }
    const root = target.closest(ROOT_SELECTOR) as HTMLElement | null;
    if (!root) {
      closeAll();
      return;
    }
    const panel = panelFor(root);
    if (!panel.contains(target)) {
      closeAll();
      return;
    }
    if (target.closest(PREV_SELECTOR)) {
      stepMonth(root, panel, -1);
    } else if (target.closest(NEXT_SELECTOR)) {
      stepMonth(root, panel, 1);
    } else if (target.closest(TODAY_SELECTOR)) {
      select(root, panel, new Date());
    } else if (target.closest(CLEAR_SELECTOR)) {
      clear(root, panel);
    } else {
      const day = target.closest(DAY_SELECTOR) as HTMLElement | null;
      if (day) {
        const date = parseISO(day.dataset.date ?? '');
        if (date) {
          select(root, panel, date);
        }
      }
    }
  });

  document.addEventListener('keydown', (evt) => {
    if (evt.key === 'Escape') {
      closeAll();
      return;
    }
    const active = document.activeElement;
    if (!(active instanceof HTMLInputElement) || !active.matches(INPUT_SELECTOR)) {
      return;
    }
    const root = active.closest(ROOT_SELECTOR) as HTMLElement | null;
    if (!root) {
      return;
    }
    const panel = panelFor(root);
    if (panel.classList.contains('hidden')) {
      return;
    }
    const view = viewByRoot.get(root);
    if (!view) {
      return;
    }
    let focused = focusedByRoot.get(root);
    if (!focused) {
      focused = parseISO(active.value) ?? new Date(view.year, view.month, 1);
    }
    let next: Date | null = null;
    switch (evt.key) {
      case 'ArrowLeft':
        next = addDays(focused, -1);
        break;
      case 'ArrowRight':
        next = addDays(focused, 1);
        break;
      case 'ArrowUp':
        next = addDays(focused, -7);
        break;
      case 'ArrowDown':
        next = addDays(focused, 7);
        break;
      case 'Home':
        next = new Date(focused.getFullYear(), focused.getMonth(), 1);
        break;
      case 'End':
        next = new Date(focused.getFullYear(), focused.getMonth() + 1, 0);
        break;
      case 'Enter':
        select(root, panel, focused);
        return;
      default:
        return;
    }
    evt.preventDefault();
    viewByRoot.set(root, { year: next.getFullYear(), month: next.getMonth() });
    focusedByRoot.set(root, next);
    paint(root, panel);
  });
}

export function refresh(_root: HTMLElement): void {
  // Delegated listeners cover swapped content; nothing to resync.
}

// ---- Pure helpers (unit-tested) ------------------------------------------

// parseISO parses a yyyy-mm-dd string into a local Date, rejecting
// non-dates, out-of-range values and non-ISO formats.
export function parseISO(value: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) {
    return null;
  }
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  if (month < 1 || month > 12 || day < 1 || day > daysInMonth(year, month - 1)) {
    return null;
  }
  return new Date(year, month - 1, day);
}

// formatISO renders a Date as yyyy-mm-dd.
export function formatISO(date: Date): string {
  const year = String(date.getFullYear()).padStart(4, '0');
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return year + '-' + month + '-' + day;
}

// daysInMonth returns the number of days in the given month (0-based).
export function daysInMonth(year: number, month: number): number {
  return new Date(year, month + 1, 0).getDate();
}

// isSameDay compares two Dates by calendar day.
export function isSameDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}

// addMonths returns a new Date shifted by delta months, clamped to the
// last valid day of the target month (Jan 31 + 1 month -> Feb 28/29).
export function addMonths(date: Date, delta: number): Date {
  const year = date.getFullYear();
  const month = date.getMonth() + delta;
  const targetYear = year + Math.floor(month / 12);
  const targetMonth = ((month % 12) + 12) % 12;
  const day = Math.min(date.getDate(), daysInMonth(targetYear, targetMonth));
  return new Date(targetYear, targetMonth, day);
}

// addDays returns a new Date shifted by delta days.
export function addDays(date: Date, delta: number): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate() + delta);
}

// monthLabel renders "Month YYYY" for the header.
export function monthLabel(year: number, month: number): string {
  return MONTHS[month] + ' ' + String(year);
}

// buildGrid returns the 42 cells (6 weeks starting on Sunday) that make
// up the calendar for the given 0-based month, with out-of-month days
// included.
export function buildGrid(year: number, month: number): Date[] {
  const first = new Date(year, month, 1);
  const start = new Date(year, month, 1 - first.getDay());
  const cells: Date[] = [];
  for (let i = 0; i < 42; i++) {
    cells.push(new Date(start.getFullYear(), start.getMonth(), start.getDate() + i));
  }
  return cells;
}

// ---- Popover --------------------------------------------------------------

// panelFor returns the popover of a container, creating it on first use.
function panelFor(root: HTMLElement): HTMLElement {
  let panel = root.querySelector(PANEL_SELECTOR) as HTMLElement | null;
  if (!panel) {
    panel = buildPanel();
    root.appendChild(panel);
  }
  return panel;
}

function buildPanel(): HTMLElement {
  const panel = document.createElement('div');
  panel.setAttribute('data-lv-datepicker-panel', '');
  panel.setAttribute('role', 'dialog');
  panel.setAttribute('aria-label', 'Date picker');
  panel.className =
    'absolute z-10 mt-2 hidden w-72 rounded-lg border border-gray-200 bg-white p-3 ' +
    'shadow-lg dark:border-gray-700 dark:bg-gray-800';

  const header = document.createElement('div');
  header.className = 'flex items-center justify-between';
  const prev = iconButton(PREV_ATTR, 'Previous month', CHEVRON_LEFT);
  const label = document.createElement('div');
  label.className = 'text-sm font-medium text-gray-900 dark:text-white';
  label.dataset.lvDatepickerLabel = '';
  const next = iconButton(NEXT_ATTR, 'Next month', CHEVRON_RIGHT);
  header.append(prev, label, next);

  const weekdays = document.createElement('div');
  weekdays.className = 'mt-3 grid grid-cols-7 text-center text-xs text-gray-500 dark:text-gray-400';
  for (const name of WEEKDAYS) {
    const cell = document.createElement('span');
    cell.textContent = name;
    weekdays.appendChild(cell);
  }

  const grid = document.createElement('div');
  grid.className = 'mt-1 grid grid-cols-7 gap-1';
  grid.dataset.lvDatepickerGrid = '';

  const footer = document.createElement('div');
  footer.className =
    'mt-3 flex items-center justify-between border-t border-gray-200 pt-3 dark:border-gray-700';
  const clear = document.createElement('button');
  clear.type = 'button';
  clear.setAttribute('data-lv-datepicker-clear', '');
  clear.textContent = 'Clear';
  clear.className =
    'rounded-lg px-3 py-1.5 text-sm font-medium text-gray-500 hover:bg-gray-100 ' +
    'dark:text-gray-400 dark:hover:bg-gray-700';
  const today = document.createElement('button');
  today.type = 'button';
  today.setAttribute('data-lv-datepicker-today', '');
  today.textContent = 'Today';
  today.className =
    'rounded-lg px-3 py-1.5 text-sm font-medium text-indigo-600 hover:bg-indigo-50 ' +
    'dark:text-indigo-400 dark:hover:bg-gray-700';
  footer.append(clear, today);

  panel.append(header, weekdays, grid, footer);
  return panel;
}

function iconButton(attr: string, label: string, svg: string): HTMLElement {
  const button = document.createElement('button');
  button.type = 'button';
  button.setAttribute(attr, '');
  button.setAttribute('aria-label', label);
  button.className =
    'rounded-lg p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-900 ' +
    'dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-white';
  button.innerHTML = svg;
  return button;
}

// open shows the popover of a container, resetting the view to the
// selected date (or today when empty) and repainting the calendar.
function open(root: HTMLElement, panel: HTMLElement): void {
  const input = root.querySelector(INPUT_SELECTOR) as HTMLInputElement | null;
  const selected = input ? parseISO(input.value) : null;
  const view = selected ?? new Date();
  viewByRoot.set(root, { year: view.getFullYear(), month: view.getMonth() });
  focusedByRoot.set(root, selected ?? new Date());
  paint(root, panel);
  panel.classList.remove('hidden');
}

// stepMonth moves the calendar view by delta months and repaints.
function stepMonth(root: HTMLElement, panel: HTMLElement, delta: number): void {
  const view = viewByRoot.get(root);
  if (!view) {
    return;
  }
  const next = addMonths(new Date(view.year, view.month, 1), delta);
  viewByRoot.set(root, { year: next.getFullYear(), month: next.getMonth() });
  paint(root, panel);
}

// select sets the input value, repaints and leaves the popover open so
// the selection is visible (Flowbite behavior); the change event lets
// htmx hooks react.
function select(root: HTMLElement, panel: HTMLElement, date: Date): void {
  const input = root.querySelector(INPUT_SELECTOR) as HTMLInputElement | null;
  if (!input) {
    return;
  }
  const min = parseISO(input.dataset.lvMinDate ?? '');
  const max = parseISO(input.dataset.lvMaxDate ?? '');
  if ((min && date < min) || (max && date > max)) {
    return;
  }
  input.value = formatISO(date);
  viewByRoot.set(root, { year: date.getFullYear(), month: date.getMonth() });
  focusedByRoot.set(root, date);
  paint(root, panel);
  input.dispatchEvent(new Event('change', { bubbles: true }));
}

function clear(root: HTMLElement, panel: HTMLElement): void {
  const input = root.querySelector(INPUT_SELECTOR) as HTMLInputElement | null;
  if (input) {
    input.value = '';
    input.dispatchEvent(new Event('change', { bubbles: true }));
  }
  const view = viewByRoot.get(root) ?? { year: 0, month: 0 };
  const anchor = parseISO(input?.value ?? '') ?? new Date();
  viewByRoot.set(root, {
    year: view.year === 0 ? anchor.getFullYear() : view.year,
    month: view.month === 0 ? anchor.getMonth() : view.month,
  });
  paint(root, panel);
}

// paint redraws the header and the day grid from the stored view.
function paint(root: HTMLElement, panel: HTMLElement): void {
  const view = viewByRoot.get(root);
  const input = root.querySelector(INPUT_SELECTOR) as HTMLInputElement | null;
  if (!view || !input) {
    return;
  }
  const selected = parseISO(input.value);
  const today = new Date();
  const min = parseISO(input.dataset.lvMinDate ?? '');
  const max = parseISO(input.dataset.lvMaxDate ?? '');
  const focused = focusedByRoot.get(root);

  const label = panel.querySelector('[data-lv-datepicker-label]');
  if (label) {
    label.textContent = monthLabel(view.year, view.month);
  }
  const grid = panel.querySelector('[data-lv-datepicker-grid]');
  if (!grid) {
    return;
  }
  grid.textContent = '';
  for (const day of buildGrid(view.year, view.month)) {
    const cell = document.createElement('button');
    cell.type = 'button';
    cell.setAttribute('data-lv-datepicker-day', '');
    cell.dataset.date = formatISO(day);
    cell.textContent = String(day.getDate());
    cell.setAttribute('aria-label', formatISO(day));
    const outOfMonth = day.getMonth() !== view.month;
    const isToday = isSameDay(day, today);
    const isSelected = !!selected && isSameDay(day, selected);
    const isFocused = !!focused && isSameDay(day, focused);
    const disabled = (min && day < min) || (max && day > max);
    let cls = 'rounded-lg p-1 text-sm leading-5 ';
    if (disabled) {
      cls += 'cursor-not-allowed text-gray-300 dark:text-gray-600';
    } else if (isSelected) {
      cls += 'bg-indigo-600 font-medium text-white';
    } else if (isToday) {
      cls +=
        'font-medium text-indigo-600 hover:bg-indigo-50 ' +
        'dark:text-indigo-400 dark:hover:bg-gray-700';
    } else if (outOfMonth) {
      cls += 'text-gray-400 hover:bg-gray-100 dark:text-gray-500 dark:hover:bg-gray-700';
    } else {
      cls += 'text-gray-900 hover:bg-gray-100 dark:text-white dark:hover:bg-gray-700';
    }
    if (isFocused) {
      cls += ' ring-2 ring-indigo-500 ring-inset';
    }
    cell.className = cls;
    if (isSelected) {
      cell.setAttribute('aria-selected', 'true');
    }
    if (disabled) {
      cell.disabled = true;
    }
    grid.appendChild(cell);
  }
}

function closeAll(): void {
  document.querySelectorAll(PANEL_SELECTOR + ':not(.hidden)').forEach((panel) => {
    panel.classList.add('hidden');
  });
}

const CHEVRON_LEFT =
  '<svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" ' +
  'aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" d="M15 19l-7-7 7-7"/></svg>';
const CHEVRON_RIGHT =
  '<svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" ' +
  'aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7"/></svg>';

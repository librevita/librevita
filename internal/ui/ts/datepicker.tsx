// Datepicker styled after the Flowbite datepicker: a text input with a
// calendar icon opens a month calendar popover (previous/next month
// navigation, a 6x7 day grid, Today/Clear actions). The popover is
// built lazily per container by this module; the server only renders
// the wrapper and the input. The markup is written in TSX and compiles
// to h() calls (jsx.ts), returning plain HTMLElement nodes; the date
// math lives in datepicker.ts (pure, unit-tested). init() uses
// delegated listeners so htmx swaps are covered automatically; refresh
// is a no-op for the same reason. Dates use the ISO yyyy-mm-dd format,
// which is what the backend expects.

import { addDays, addMonths, buildGrid, formatISO, isSameDay, monthLabel, parseISO } from './datepicker.ts';
import { h } from './jsx.ts';

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

const WEEKDAYS = ['Su', 'Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa'];

const CHEVRON_LEFT =
  '<svg class="h-4 w-4" fill="currentColor" viewBox="0 -960 960 960" aria-hidden="true">' +
  '<path d="M560-240 320-480l240-240 56 56-184 184 184 184-56 56Z"/></svg>';
const CHEVRON_RIGHT =
  '<svg class="h-4 w-4" fill="currentColor" viewBox="0 -960 960 960" aria-hidden="true">' +
  '<path d="M504-480 320-664l56-56 240 240-240 240-56-56 184-184Z"/></svg>';

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

// ---- Popover markup (TSX) ------------------------------------------------

// buildPanel renders the popover skeleton; the grid is filled by paint
// and the header by monthLabel. The chevron buttons inject trusted SVG
// constants, which h() deliberately rejects, so iconButton stays
// imperative.
export function buildPanel(): HTMLElement {
  return (
    <div
      data-lv-datepicker-panel=""
      role="dialog"
      aria-label="Date picker"
      className="absolute z-10 mt-2 hidden w-72 rounded-lg border border-gray-200 bg-white p-3 shadow-lg dark:border-gray-700 dark:bg-gray-800"
    >
      <div className="flex items-center justify-between">
        {iconButton(PREV_ATTR, 'Previous month', CHEVRON_LEFT)}
        <div
          data-lv-datepicker-label=""
          className="text-sm font-medium text-gray-900 dark:text-white"
        />
        {iconButton(NEXT_ATTR, 'Next month', CHEVRON_RIGHT)}
      </div>
      <div className="mt-3 flex flex-wrap text-center text-xs text-gray-500 dark:text-gray-400">
        {WEEKDAYS.map((name) => (
          <span className="w-[calc(100%/7)]">{name}</span>
        ))}
      </div>
      <div data-lv-datepicker-grid="" className="-m-0.5 mt-1 flex flex-wrap" />
      <div className="mt-3 flex items-center justify-between border-t border-gray-200 pt-3 dark:border-gray-700">
        <button
          type="button"
          data-lv-datepicker-clear=""
          className="rounded-lg px-3 py-1.5 text-sm font-medium text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-700"
        >
          Clear
        </button>
        <button
          type="button"
          data-lv-datepicker-today=""
          className="rounded-lg px-3 py-1.5 text-sm font-medium text-indigo-600 hover:bg-indigo-50 dark:text-indigo-400 dark:hover:bg-gray-700"
        >
          Today
        </button>
      </div>
    </div>
  );
}

export type DayCellProps = {
  day: Date;
  viewMonth: number;
  selected: Date | null;
  today: Date;
  min: Date | null;
  max: Date | null;
  focused: Date | null;
};

// dayCell renders one calendar cell with the full selection state:
// disabled when outside the min/max range, highlighted when selected,
// today, focused or out-of-month.
export function dayCell(props: DayCellProps): HTMLElement {
  const day = props.day;
  const outOfMonth = day.getMonth() !== props.viewMonth;
  const isToday = isSameDay(day, props.today);
  const isSelected = !!props.selected && isSameDay(day, props.selected);
  const isFocused = !!props.focused && isSameDay(day, props.focused);
  const disabled = (props.min && day < props.min) || (props.max && day > props.max);
  let cls = 'w-[calc(100%/7)] rounded-lg p-0.5 text-sm leading-5 ';
  if (disabled) {
    cls += 'cursor-not-allowed text-gray-300 dark:text-gray-600';
  } else if (isSelected) {
    cls += 'bg-indigo-600 font-medium text-white';
  } else if (isToday) {
    cls += 'font-medium text-indigo-600 hover:bg-indigo-50 dark:text-indigo-400 dark:hover:bg-gray-700';
  } else if (outOfMonth) {
    cls += 'text-gray-400 hover:bg-gray-100 dark:text-gray-500 dark:hover:bg-gray-700';
  } else {
    cls += 'text-gray-900 hover:bg-gray-100 dark:text-white dark:hover:bg-gray-700';
  }
  if (isFocused) {
    cls += ' ring-2 ring-indigo-500 ring-inset';
  }
  const iso = formatISO(day);
  return (
    <button
      type="button"
      data-lv-datepicker-day=""
      data-date={iso}
      aria-label={iso}
      className={cls}
      disabled={disabled}
      aria-selected={isSelected ? 'true' : undefined}
    >
      {day.getDate()}
    </button>
  );
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

// ---- Popover behavior ----------------------------------------------------

// panelFor returns the popover of a container, creating it on first use.
function panelFor(root: HTMLElement): HTMLElement {
  let panel = root.querySelector(PANEL_SELECTOR) as HTMLElement | null;
  if (!panel) {
    panel = buildPanel();
    root.appendChild(panel);
  }
  return panel;
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
    grid.appendChild(
      dayCell({ day, viewMonth: view.month, selected, today, min, max, focused: focused ?? null }),
    );
  }
}

function closeAll(): void {
  document.querySelectorAll(PANEL_SELECTOR + ':not(.hidden)').forEach((panel) => {
    panel.classList.add('hidden');
  });
}

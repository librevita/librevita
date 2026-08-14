// Pure date helpers for the datepicker widget. Kept as a plain TS
// module (no DOM, no JSX) so `node --test` can exercise them directly;
// datepicker-panel.tsx and datepicker-widget.tsx import from here.

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

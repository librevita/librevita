// Datepicker Alpine component (CSP build): owns the ephemeral state of
// the popover — open/close, outside-click, Escape, arrow-key focus and
// the value pick. The calendar grid itself is rendered by the server
// (components.DatepickerPanel) and navigates months with htmx swaps,
// so no DOM is built here. Registered before Alpine.start() in main.ts.
//
// The Alpine CSP build forbids inline expressions beyond literals,
// property paths and registered method calls, so the template only uses
// pick('yyyy-mm-dd') / dayClass('yyyy-mm-dd') with literal arguments.

// Type-only import: the Alpine runtime is injected by main.ts, so this
// module stays side-effect-free and unit-testable.
import type { Alpine, AlpineComponent } from '@alpinejs/csp';

export function formatISODate(d: Date): string {
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${d.getFullYear()}-${m}-${day}`;
}

export function parseISO(s: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s);
  if (!m) {
    return null;
  }
  const y = Number(m[1]);
  const mo = Number(m[2]);
  const d = Number(m[3]);
  const date = new Date(y, mo - 1, d);
  // Reject rolled-over dates (2025-02-30 -> March 2nd).
  if (date.getFullYear() !== y || date.getMonth() !== mo - 1 || date.getDate() !== d) {
    return null;
  }
  return date;
}

// registerDatepicker wires the component into the injected Alpine
// instance; main.ts calls it before Alpine.start().
export function registerDatepicker(alpine: Alpine): void {
  alpine.data('datepicker', (() => ({
  open: false,
  focused: '',

  init(this: AlpineComponent): void {
    const input = this.$refs.input as HTMLInputElement | null;
    this.focused = input ? input.value : '';
    if (!this.focused) {
      this.focused = formatISODate(new Date());
    }
  },

  toggle(this: AlpineComponent): void {
    this.open = !this.open;
  },

  close(this: AlpineComponent): void {
    this.open = false;
  },

  pick(this: AlpineComponent, iso: string): void {
    const input = this.$refs.input as HTMLInputElement | null;
    if (input) {
      input.value = iso;
    }
    this.focused = iso;
    this.close();
  },

  clear(this: AlpineComponent): void {
    const input = this.$refs.input as HTMLInputElement | null;
    if (input) {
      input.value = '';
    }
    this.close();
  },

  today(this: AlpineComponent): void {
    this.pick(formatISODate(new Date()));
  },

  // move shifts the keyboard focus within the currently rendered
  // month; crossing into the previous/next month is left for a later
  // iteration (the server owns the grid).
  move(this: AlpineComponent, delta: number): void {
    const panel = this.$root.querySelector('[data-lv-datepicker-panel]');
    if (!panel) {
      return;
    }
    const days = Array.from(panel.querySelectorAll('[data-lv-datepicker-day]')).map(
      (c: Element) => c.getAttribute('data-lv-day') || '',
    );
    if (days.length === 0) {
      return;
    }
    let idx = days.indexOf(this.focused);
    if (idx === -1) {
      this.focused = days[0];
      return;
    }
    const next = idx + delta;
    if (next >= 0 && next < days.length) {
      this.focused = days[next];
    }
  },

  dayClass(this: AlpineComponent, iso: string): string {
    return this.focused === iso ? 'ring-2 ring-indigo-500 ring-inset' : '';
  },
  })) as unknown as () => AlpineComponent);
}

export function init(): void {
  // The component is registered via registerDatepicker; Alpine.start()
  // runs in main.ts.
}

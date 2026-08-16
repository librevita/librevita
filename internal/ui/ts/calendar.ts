// Calendar page Alpine component (CSP build): the ephemeral state of
// the schedule — the month/week view and the physician filter. The
// grids themselves are rendered by the server (CalendarPage) and the
// now-line/appointment geometry comes from server-rendered data
// attributes, bound via x-bind:style. Registered before Alpine.start()
// in main.ts.
//
// The Alpine CSP build forbids inline expressions beyond literals,
// property paths, basic operations and registered method calls; the
// template keeps the bindings to that subset.

import type { Alpine, AlpineComponent } from '@alpinejs/csp';

export function registerAgenda(Alpine: Alpine): void {
  Alpine.data('agenda', (() => ({
    view: 'month',
    physician: '',
  })) as unknown as () => AlpineComponent);
}

export function init(): void {
  // The component is registered via registerAgenda; Alpine.start() runs
  // in main.ts.
}

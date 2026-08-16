// Calendar page Alpine component (CSP build): the ephemeral state of
// the schedule — the month/week view and the physician filter. The
// grids themselves are rendered by the server (CalendarPage) and the
// now-line/appointment geometry comes from server-rendered style
// literals.
//
// The component follows the datepicker profile exactly: methods called
// with literal arguments, property reads, and full-literal style
// strings. x-model and inline expressions (ternaries, comparisons,
// assignments) are avoided — on the 45-era engines they silently broke
// the reactivity of the whole tree.

import type { Alpine, AlpineComponent } from '@alpinejs/csp';

export function registerAgenda(Alpine: Alpine): void {
  Alpine.data('agenda', (() => ({
    view: 'month',
    physician: '',

    setView(this: AlpineComponent, v: string): void {
      this.view = v;
    },

    // setPhysician reads the filter select value via the x-ref; the
    // method takes no arguments.
    setPhysician(this: AlpineComponent): void {
      this.physician = (this.$refs.select as HTMLSelectElement).value;
    },

    // isView decides the visible panel, like the datepicker's x-show
    // on plain properties (methods with literal arguments only).
    isView(this: AlpineComponent, v: string): boolean {
      return this.view === v;
    },

    // viewClass returns the active toggle button classes, like the
    // datepicker's dayClass.
    viewClass(this: AlpineComponent, v: string): string {
      return this.view === v ? 'bg-white text-gray-900 dark:bg-gray-800 dark:text-white' : '';
    },

    // showChip decides the physician filter visibility for one
    // appointment, called with a literal physician name.
    showChip(this: AlpineComponent, p: string): boolean {
      return this.physician === '' || this.physician === p;
    },
  })) as unknown as () => AlpineComponent);
}

export function init(): void {
  // The component is registered via registerAgenda; Alpine.start() runs
  // in main.ts.
}

// Identifier mask component (Alpine CSP build): applies the selected
// identifier system's typing mask to the document value field. The
// mask is stored per system (identifier_systems.mask) and carried on
// the select options as data-mask.
//
// The mask plugin evaluates x-mask:dynamic inside a reactive effect:
// the expression maskFor(system) reads the reactive `system` state, so
// the mask is recomputed whenever the select changes, and the method
// itself follows the datepicker profile (plain reads, no evaluator
// magic).

import type { Alpine, AlpineComponent } from '@alpinejs/csp';

export function registerIdentifiermask(Alpine: Alpine): void {
  Alpine.data('identifiermask', (() => ({
    system: '',

    systemChanged(this: AlpineComponent): void {
      this.system = (this.$refs.system as HTMLSelectElement).value;
    },

    // maskFor returns the selected system's mask (data-mask on the
    // option), or '' when none is configured — the raw pattern and
    // transform remain the validation authority.
    maskFor(this: AlpineComponent, _value: string): string {
      const select = this.$refs.system as HTMLSelectElement;
      const opt = select.selectedOptions.length > 0 ? select.selectedOptions[0] : null;
      return opt ? (opt.getAttribute('data-mask') || '') : '';
    },
  })) as unknown as () => AlpineComponent);
}

export function init(): void {
  // The component is registered via registerIdentifiermask;
  // Alpine.start() runs in main.ts.
}

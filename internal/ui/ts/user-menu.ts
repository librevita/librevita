// Topbar user menu component (Alpine CSP build): a dropdown toggled by
// its button, closed on outside clicks or Escape. The menu carries
// x-cloak so it stays hidden before Alpine initializes and without JS.

import type { Alpine, AlpineComponent } from '@alpinejs/csp';

export function registerUserMenu(Alpine: Alpine): void {
  Alpine.data('userMenu', (() => ({
    open: false,

    toggle(this: AlpineComponent): void {
      this.open = !this.open;
    },

    close(this: AlpineComponent): void {
      this.open = false;
    },
  })) as unknown as () => AlpineComponent);
}

export function init(): void {
  // The component is registered via registerUserMenu; Alpine.start()
  // runs in main.ts.
}

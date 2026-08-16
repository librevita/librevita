// Sidebar drawer component (Alpine CSP build) for the mobile hamburger.
// The scope lives on the shell root (x-data="sidebar" in shell.templ),
// which covers the toggle in the topbar and the drawer/backdrop in the
// sidebar. The backdrop carries the close click (it covers the whole
// screen while open, below the topbar), so no outside-click modifier is
// needed.

import type { Alpine, AlpineComponent } from '@alpinejs/csp';

export function registerSidebar(Alpine: Alpine): void {
  Alpine.data('sidebar', (() => ({
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
  // The component is registered via registerSidebar; Alpine.start()
  // runs in main.ts.
}

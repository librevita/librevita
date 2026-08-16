// Dropdown component (Alpine CSP build) for the table action menus. The
// wiring is declarative in dropdown.templ (x-data, @click.outside,
// Escape, the arrow/Home/End keys with .prevent); the two pieces of
// real logic — viewport pinning and keyboard focus — live in the pure
// helpers below so they stay unit-testable without the Alpine runtime.
//
// Pinning is fixed-position on purpose: the table containers have
// overflow-x-auto (which forces overflow-y to auto too), so an
// absolutely positioned menu would open inside the list and scroll with
// it; position fixed escapes the scroll container.

import type { Alpine, AlpineComponent } from '@alpinejs/csp';

const ITEM_SELECTOR =
  'a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])';

// TODO(alpine): move pinMenu and keyboardTarget into the Alpine
// component methods once the test harness can run Alpine (linkedom
// does not) or when @alpinejs/focus is adopted for the keyboard
// navigation. The helpers stay pure and unit-tested until then.

// pinMenu shows the menu pinned to the viewport: fixed position,
// clamped to the window edges. The menu must be visible when called
// (the caller measures it after Alpine applies x-show).
export function pinMenu(toggle: HTMLElement, menu: HTMLElement): void {
  const rect = toggle.getBoundingClientRect();
  const width = menu.offsetWidth;
  const height = menu.offsetHeight;
  const left = Math.max(8, Math.min(rect.right - width, window.innerWidth - width - 8));
  const top = Math.min(rect.bottom + 4, window.innerHeight - height - 8);
  menu.style.position = 'fixed';
  menu.style.left = left + 'px';
  menu.style.top = Math.max(8, top) + 'px';
}

// resetMenu clears the pinning; visibility is owned by Alpine (x-show).
export function resetMenu(menu: HTMLElement): void {
  menu.style.position = '';
  menu.style.left = '';
  menu.style.top = '';
}

// keyboardTarget picks the item that receives focus for a navigation
// key, or null when the key is not handled. The active element must be
// inside the menu or on its toggle; arrows wrap around.
export function keyboardTarget(
  items: HTMLElement[],
  toggle: HTMLElement | null,
  active: Element | null,
  key: string,
): HTMLElement | null {
  if (items.length === 0) {
    return null;
  }
  const index = items.indexOf(active as HTMLElement);
  const inside = index !== -1;
  if (!inside && active !== toggle) {
    return null;
  }
  if (key === 'Home') {
    return items[0];
  }
  if (key === 'End') {
    return items[items.length - 1];
  }
  if (key === 'ArrowDown') {
    return inside ? items[(index + 1) % items.length] : items[0];
  }
  if (key === 'ArrowUp') {
    return inside ? items[(index - 1 + items.length) % items.length] : items[items.length - 1];
  }
  return null;
}

export function registerDropdown(Alpine: Alpine): void {
  Alpine.data('dropdown', (() => ({
    open: false,

    toggle(this: AlpineComponent): void {
      this.open = !this.open;
      const menuEl = this.$refs.menu as HTMLElement;
      if (this.open) {
        // Measure after Alpine applies the x-show display.
        this.$nextTick(() => {
          if (this.open) {
            pinMenu(this.$refs.toggle as HTMLElement, menuEl);
          }
        });
      } else {
        resetMenu(menuEl);
      }
    },

    close(this: AlpineComponent): void {
      if (!this.open) {
        return;
      }
      this.open = false;
      resetMenu(this.$refs.menu as HTMLElement);
    },

    // move applies the keyboard navigation; the .prevent modifiers in
    // the template stop the page from scrolling on the arrows.
    move(this: AlpineComponent, key: string): void {
      if (!this.open) {
        return;
      }
      const menuEl = this.$refs.menu as HTMLElement;
      const target = keyboardTarget(
        Array.from(menuEl.querySelectorAll<HTMLElement>(ITEM_SELECTOR)),
        this.$refs.toggle as HTMLElement | null,
        document.activeElement,
        key,
      );
      if (target) {
        target.focus();
      }
    },
  })) as unknown as () => AlpineComponent);
}

export function init(): void {
  // The component is registered via registerDropdown; Alpine.start()
  // runs in main.ts.
}

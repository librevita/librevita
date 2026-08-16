// Modal component (Alpine CSP build) for the flowbite-style dialog.
// The wiring is declarative in modal.templ (x-show, x-trap from the
// focus plugin, Escape, backdrop and close-button clicks, the
// lv:open-modal window event dispatched by the trigger buttons). The
// a11y attributes and the scroll lock live in the pure helpers below
// so they stay unit-testable without the Alpine runtime; visibility
// itself is owned by Alpine (x-show).

import type { Alpine, AlpineComponent } from '@alpinejs/csp';

// openModal applies the a11y state of a dialog and locks the body
// scroll. Visibility is Alpine's (x-show="open").
export function openModal(modal: HTMLElement): void {
  modal.setAttribute('aria-hidden', 'false');
  modal.setAttribute('role', 'dialog');
  modal.setAttribute('aria-modal', 'true');
  // The name comes from the static aria-labelledby (template title); the
  // description is optional content: mark an element inside the dialog
  // with [data-lv-modal-description] to have it announced.
  const desc = modal.querySelector('[data-lv-modal-description]') as HTMLElement | null;
  if (desc && desc.id) {
    modal.setAttribute('aria-describedby', desc.id);
  }
  document.body.style.overflow = 'hidden';
}

export function closeModal(modal: HTMLElement): void {
  modal.setAttribute('aria-hidden', 'true');
  modal.removeAttribute('role');
  modal.removeAttribute('aria-modal');
  modal.removeAttribute('aria-describedby');
  document.body.style.overflow = '';
}

export function registerModal(Alpine: Alpine): void {
  Alpine.data('modal', (() => ({
    open: false,

    // openWith handles the lv:open-modal window event dispatched by
    // trigger buttons ($dispatch('lv:open-modal', { id })); only the
    // dialog whose id matches opens.
    openWith(this: AlpineComponent, evt: CustomEvent): void {
      const detail = evt.detail as { id?: string } | undefined;
      if (!detail || detail.id !== this.$root.id) {
        return;
      }
      this.open = true;
      openModal(this.$root as HTMLElement);
    },

    close(this: AlpineComponent): void {
      if (!this.open) {
        return;
      }
      this.open = false;
      closeModal(this.$root as HTMLElement);
    },
  })) as unknown as () => AlpineComponent);
}

export function init(): void {
  // The component is registered via registerModal; Alpine.start() runs
  // in main.ts.
}

// Modal toggling for the flowbite-style dialog. Buttons with
// data-lv-modal-open="#id" open a dialog, [data-lv-modal-close] closes
// the closest one; clicking the backdrop or pressing Escape also closes.
// While a modal is open the body does not scroll and Tab cycles inside
// the dialog (focus trap); closing restores focus to the trigger.

// Focusable candidates inside a dialog.
const FOCUSABLE =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), ' +
  'textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

// The element that opened each open modal, to restore focus on close.
const triggerByModal = new WeakMap<HTMLElement, HTMLElement | null>();

export function init(): void {
  document.addEventListener('click', (evt) => {
    const el = (evt.target as HTMLElement).closest(
      '[data-lv-modal-open], [data-lv-modal-close]',
    ) as HTMLElement | null;
    if (!el) {
      return;
    }
    if (el.hasAttribute('data-lv-modal-open')) {
      const id = el.getAttribute('data-lv-modal-open');
      const modal = id ? document.getElementById(id) : null;
      if (modal) {
        openModal(modal);
      }
      return;
    }
    const modal = el.closest('[data-lv-modal]') as HTMLElement | null;
    if (modal) {
      closeModal(modal);
    }
  });

  document.addEventListener('click', (evt) => {
    const backdrop = (evt.target as HTMLElement).closest('[data-lv-modal]') as HTMLElement | null;
    if (backdrop && evt.target === backdrop) {
      closeModal(backdrop);
    }
  });

  document.addEventListener('keydown', (evt) => {
    if (evt.key === 'Escape') {
      document.querySelectorAll('[data-lv-modal]:not(.hidden)').forEach((m) => {
        closeModal(m as HTMLElement);
      });
      return;
    }
    if (evt.key === 'Tab') {
      const modal = openModalRoot();
      if (!modal) {
        return;
      }
      const boxes = focusable(modal);
      if (boxes.length === 0) {
        evt.preventDefault();
        return;
      }
      const first = boxes[0];
      const last = boxes[boxes.length - 1];
      const active = document.activeElement;
      if (evt.shiftKey && (active === first || !modal.contains(active))) {
        evt.preventDefault();
        last.focus();
      } else if (!evt.shiftKey && (active === last || !modal.contains(active))) {
        evt.preventDefault();
        first.focus();
      }
    }
  });
}

export function refresh(_root: HTMLElement): void {
  // Delegated listeners cover swapped content; nothing to resync.
}

function openModal(modal: HTMLElement): void {
  triggerByModal.set(modal, document.activeElement as HTMLElement | null);
  modal.classList.remove('hidden');
  modal.classList.add('flex');
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
  // Scroll lock: the page behind the dimmed backdrop must not move.
  document.body.style.overflow = 'hidden';
  const boxes = focusable(modal);
  (boxes[0] ?? modal).focus();
}

function closeModal(modal: HTMLElement): void {
  modal.classList.remove('flex');
  modal.classList.add('hidden');
  modal.setAttribute('aria-hidden', 'true');
  modal.removeAttribute('role');
  modal.removeAttribute('aria-modal');
  modal.removeAttribute('aria-describedby');
  if (document.querySelectorAll('[data-lv-modal]:not(.hidden)').length === 0) {
    document.body.style.overflow = '';
  }
  const trigger = triggerByModal.get(modal);
  if (trigger && typeof trigger.focus === 'function') {
    trigger.focus();
  }
  triggerByModal.delete(modal);
}

function openModalRoot(): HTMLElement | null {
  return document.querySelector('[data-lv-modal]:not(.hidden)');
}

function focusable(modal: HTMLElement): HTMLElement[] {
  return Array.from(modal.querySelectorAll<HTMLElement>(FOCUSABLE));
}

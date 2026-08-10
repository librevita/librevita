// Modal toggling for the flowbite-style dialog. Buttons with
// data-lv-modal-open="#id" open a dialog, [data-lv-modal-close] closes
// the closest one; clicking the backdrop or pressing Escape also closes.

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
    }
  });
}

export function refresh(_root: HTMLElement): void {
  // Delegated listeners cover swapped content; nothing to resync.
}

function openModal(modal: HTMLElement): void {
  modal.classList.remove('hidden');
  modal.classList.add('flex');
}

function closeModal(modal: HTMLElement): void {
  modal.classList.remove('flex');
  modal.classList.add('hidden');
}

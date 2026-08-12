// Masked-value reveal: buttons marked [data-lv-reveal] toggle the
// plaintext of a sibling element. The plaintext lives in the DOM
// (data-lv-plain) on purpose: everyone rendering the page is already
// authorized, and the reception desk must be able to verify a document
// with one click -- the mask is screen privacy, not access control.

export function init(): void {
  document.addEventListener('click', (evt) => {
    const btn = (evt.target as HTMLElement).closest('[data-lv-reveal]') as HTMLElement | null;
    if (!btn) {
      return;
    }
    const target = document.getElementById(btn.getAttribute('data-lv-reveal') ?? '');
    if (!target) {
      return;
    }
    const masked = btn.getAttribute('data-lv-masked') ?? '';
    const plain = btn.getAttribute('data-lv-plain') ?? '';
    if (target.textContent === plain) {
      target.textContent = masked;
      btn.textContent = 'Show';
      btn.setAttribute('aria-pressed', 'false');
    } else {
      target.textContent = plain;
      btn.textContent = 'Hide';
      btn.setAttribute('aria-pressed', 'true');
    }
  });
}

export function refresh(_root: HTMLElement | null): void {
  // Delegated listener; nothing to resync after swaps.
}

// Dropdown toggling for the table action menus. A container with
// data-lv-dropdown holds a toggle button (data-lv-dropdown-toggle) and
// the menu ([data-lv-dropdown-menu], hidden by default). Clicking the
// toggle flips the menu, clicking anywhere else or pressing Escape
// closes every open menu.

export function init(): void {
  document.addEventListener('click', (evt) => {
    const target = evt.target as HTMLElement;
    const toggle = target.closest('[data-lv-dropdown-toggle]') as HTMLElement | null;
    if (toggle) {
      const container = toggle.closest('[data-lv-dropdown]') as HTMLElement | null;
      const menu = container
        ? (container.querySelector('[data-lv-dropdown-menu]') as HTMLElement | null)
        : null;
      if (container && menu) {
        const open = !menu.classList.contains('hidden');
        closeAll();
        if (!open) {
          openMenu(toggle, menu);
        }
      }
      return;
    }
    if (!target.closest('[data-lv-dropdown]')) {
      closeAll();
    }
  });

  document.addEventListener('keydown', (evt) => {
    if (evt.key === 'Escape') {
      closeAll();
    }
  });
}

export function refresh(_root: HTMLElement): void {
  // Delegated listeners cover swapped content; nothing to resync.
}

// openMenu shows the menu pinned to the viewport: table containers have
// overflow-x-auto (which forces overflow-y to auto too), so an
// absolutely positioned menu would open inside the list and scroll with
// it. position fixed escapes the scroll container.
function openMenu(toggle: HTMLElement, menu: HTMLElement): void {
  menu.classList.remove('hidden');
  const rect = toggle.getBoundingClientRect();
  const width = menu.offsetWidth;
  const height = menu.offsetHeight;
  const left = Math.max(8, Math.min(rect.right - width, window.innerWidth - width - 8));
  const top = Math.min(rect.bottom + 4, window.innerHeight - height - 8);
  menu.style.position = 'fixed';
  menu.style.left = left + 'px';
  menu.style.top = Math.max(8, top) + 'px';
}

function closeAll(): void {
  document.querySelectorAll('[data-lv-dropdown-menu]:not(.hidden)').forEach((menu) => {
    const m = menu as HTMLElement;
    m.classList.add('hidden');
    m.style.position = '';
    m.style.left = '';
    m.style.top = '';
  });
}

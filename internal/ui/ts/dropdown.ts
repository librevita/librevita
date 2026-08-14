// Dropdown toggling for the table action menus. A container with
// data-lv-dropdown holds a toggle button (data-lv-dropdown-toggle) and
// the menu ([data-lv-dropdown-menu], hidden by default). Clicking the
// toggle flips the menu, clicking anywhere else or pressing Escape
// closes every open menu. With a menu open, ArrowDown/ArrowUp move
// focus between the items (wrapping), Home/End jump to the first/last;
// the arrows also move focus into the menu from the toggle.

const MENU_OPEN_SELECTOR = '[data-lv-dropdown-menu]:not(.hidden)';
const ITEM_SELECTOR =
  'a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])';

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
      return;
    }
    const menu = document.querySelector(MENU_OPEN_SELECTOR) as HTMLElement | null;
    if (!menu) {
      return;
    }
    const container = menu.closest('[data-lv-dropdown]') as HTMLElement | null;
    const toggle = container
      ? (container.querySelector('[data-lv-dropdown-toggle]') as HTMLElement | null)
      : null;
    const target = keyboardTarget(
      menuItems(menu),
      toggle,
      document.activeElement,
      evt.key,
    );
    if (target) {
      evt.preventDefault();
      target.focus();
    }
  });
}

export function refresh(_root: HTMLElement): void {
  // Delegated listeners cover swapped content; nothing to resync.
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

function menuItems(menu: HTMLElement): HTMLElement[] {
  return Array.from(menu.querySelectorAll<HTMLElement>(ITEM_SELECTOR));
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

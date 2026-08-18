// Status filter menu component (Alpine CSP build) for the patient registry toolbar:
// the toggle button shows the active status and the hidden input named "status"
// (id "status-filter") carries the selection into htmx requests.
// The selected status is persisted in localStorage and cookie so the user's choice survives
// navigations and page reloads.

import type { Alpine, AlpineComponent } from '@alpinejs/csp';

export const STATUS_STORAGE_KEY = 'librevita:patient_status_filter';
export const STATUS_COOKIE_NAME = 'lv_patient_status';

export function statusLabel(status: string): string {
  if (status === 'active') {
    return 'Active';
  }
  if (status === 'inactive') {
    return 'Inactive';
  }
  return 'All statuses';
}

export function getSavedStatus(storage?: Storage): string {
  try {
    const s = storage || (typeof window !== 'undefined' ? window.localStorage : undefined);
    if (!s) {
      return '';
    }
    const val = s.getItem(STATUS_STORAGE_KEY);
    if (val === 'active' || val === 'inactive') {
      return val;
    }
  } catch (err) {
    // Ignore storage errors (e.g. private browsing or disabled storage)
  }
  return '';
}

export function saveStatus(status: string, storage?: Storage): void {
  try {
    const s = storage || (typeof window !== 'undefined' ? window.localStorage : undefined);
    if (s) {
      if (status === 'active' || status === 'inactive') {
        s.setItem(STATUS_STORAGE_KEY, status);
      } else {
        s.removeItem(STATUS_STORAGE_KEY);
      }
    }
  } catch (err) {
    // Ignore storage errors
  }

  try {
    if (typeof document !== 'undefined') {
      if (status === 'active' || status === 'inactive') {
        document.cookie = `${STATUS_COOKIE_NAME}=${status}; path=/; SameSite=Lax; max-age=31536000`;
      } else {
        document.cookie = `${STATUS_COOKIE_NAME}=; path=/; SameSite=Lax; max-age=0`;
      }
    }
  } catch (err) {
    // Ignore cookie errors
  }
}

function triggerFilter(input: HTMLElement): void {
  if (typeof htmx !== 'undefined' && typeof htmx.trigger === 'function') {
    htmx.trigger(input, 'change', {});
  } else {
    input.dispatchEvent(new Event('change', { bubbles: true }));
  }
}

export function registerStatusMenu(Alpine: Alpine): void {
  Alpine.data('statusMenu', (() => ({
    open: false,
    status: '',
    label: 'All statuses',

    init(this: AlpineComponent): void {
      const root = this.$root as HTMLElement;
      let initialStatus = root.dataset.status || '';

      if (!initialStatus) {
        const saved = getSavedStatus();
        if (saved) {
          initialStatus = saved;
          this.status = saved;
          this.label = statusLabel(saved);
          saveStatus(saved);
          this.$nextTick(() => {
            const input = (this.$refs.status as HTMLInputElement) || document.getElementById('status-filter');
            if (input instanceof HTMLInputElement) {
              input.value = saved;
              triggerFilter(input);
            }
          });
          return;
        }
      }

      this.status = initialStatus;
      this.label = statusLabel(this.status);
      saveStatus(this.status);
    },

    toggle(this: AlpineComponent): void {
      this.open = !this.open;
    },

    close(this: AlpineComponent): void {
      this.open = false;
    },

    isActive(this: AlpineComponent, val: string): boolean {
      return this.status === val;
    },

    choose(this: AlpineComponent, status: string): void {
      this.status = status;
      this.label = statusLabel(status);
      saveStatus(status);
      const input = (this.$refs.status as HTMLInputElement) || document.getElementById('status-filter');
      if (input instanceof HTMLInputElement) {
        input.value = status;
        triggerFilter(input);
      }
      this.open = false;
    },
  })) as unknown as () => AlpineComponent);
}

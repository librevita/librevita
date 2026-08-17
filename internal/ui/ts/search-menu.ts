// Search menu (Alpine CSP build) for the registry "search with
// dropdown": the toggle button shows the active scope, the menu picks
// the search field and the hidden input named "field" carries the
// selection into the htmx request (the debounced input and the Search
// button both include it). Document type scopes (system URNs, listed
// in the data-systems JSON map rendered by the template) turn the box
// into the exact document lookup: the label shows the document type
// and the placeholder invites typing the value. The label and
// placeholder helpers are pure so they stay unit-testable without the
// Alpine runtime.

import type { Alpine, AlpineComponent } from '@alpinejs/csp';

// searchFieldLabel maps the scope to the button label: the display
// name of the document system when the scope is a system URN, the
// known field names otherwise, and the combined search as fallback.
export function searchFieldLabel(field: string, systems: Record<string, string> = {}): string {
  const label = systems[field];
  if (label) {
    return label;
  }
  if (field === 'name') {
    return 'Name';
  }
  if (field === 'email') {
    return 'Email';
  }
  return 'All fields';
}

// searchPlaceholder is the input placeholder of the current scope: a
// document type invites the exact lookup, everything else the free
// text search.
export function searchPlaceholder(field: string, systems: Record<string, string> = {}): string {
  const label = systems[field];
  if (label) {
    return 'Exact ' + label + ' lookup';
  }
  return 'Search patients...';
}

// parseSystems reads the data-systems JSON map (system URN -> display
// name) rendered by the template; a missing or malformed attribute
// degrades to an empty map.
function parseSystems(raw: string | undefined): Record<string, string> {
  if (!raw) {
    return {};
  }
  try {
    const parsed: unknown = JSON.parse(raw);
    if (parsed !== null && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, string>;
    }
  } catch {
    // fall through to the empty map
  }
  return {};
}

export function registerSearchMenu(Alpine: Alpine): void {
  Alpine.data('searchMenu', (() => ({
    open: false,
    field: '',
    label: 'All fields',
    placeholder: 'Search patients...',
    systems: {} as Record<string, string>,

    init(this: AlpineComponent): void {
      const root = this.$root as HTMLElement;
      this.systems = parseSystems(root.dataset.systems);
      this.field = root.dataset.field || '';
      this.label = searchFieldLabel(this.field, this.systems);
      this.placeholder = searchPlaceholder(this.field, this.systems);
    },

    toggle(this: AlpineComponent): void {
      this.open = !this.open;
    },

    close(this: AlpineComponent): void {
      this.open = false;
    },

    // choose sets the search scope, syncs the hidden input and asks
    // htmx to re-run the search when the box already has a term. The
    // "change" trigger on the input (declared in the template) picks
    // the dispatched event up and fires the request immediately.
    choose(this: AlpineComponent, field: string): void {
      this.field = field;
      this.label = searchFieldLabel(field, this.systems);
      this.placeholder = searchPlaceholder(field, this.systems);
      (this.$refs.field as HTMLInputElement).value = field;
      this.open = false;
      const input = document.getElementById('patient-search');
      if (input instanceof HTMLInputElement && input.value !== '') {
        input.dispatchEvent(new Event('change', { bubbles: true }));
      }
    },
  })) as unknown as () => AlpineComponent);
}

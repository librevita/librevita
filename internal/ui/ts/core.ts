/// <reference path="./globals.d.ts" />
// Core bootstrap shared by the component modules: htmx hardening, the
// CSRF header, and focus management.

export function readCookie(name: string): string {
  const parts = document.cookie.split(';');
  for (let i = 0; i < parts.length; i++) {
    const entry = parts[i].trim();
    if (entry.indexOf(name + '=') === 0) {
      return entry.substring(name.length + 1);
    }
  }
  return '';
}

// Hardens htmx: fail closed on dynamic evaluation and never accept
// scripts from swapped fragments.
export function configureHtmx(): void {
  htmx.config.allowEval = false;
  htmx.config.allowScriptTags = false;
  htmx.config.includeIndicatorStyles = false;
}

// Forwards the double-submit CSRF token on every state-changing request.
export function forwardCsrf(): void {
  document.addEventListener('htmx:configRequest', (evt) => {
    const detail = (evt as CustomEvent<HtmxRequestDetail>).detail;
    const token = readCookie('lv_csrf');
    if (token) {
      detail.headers['X-CSRF-Token'] = token;
    }
  });
}

// Moves focus to the application main content after a navigation swap.
export function focusAppMain(): void {
  const main = document.getElementById('app-main');
  if (main) {
    main.focus({ preventScroll: true });
  }
}

export function closestOrSelf(
  node: EventTarget | null,
  selector: string,
): Element | null {
  if (!(node instanceof Element)) {
    return null;
  }
  return node.closest(selector);
}

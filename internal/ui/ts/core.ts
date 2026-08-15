/// <reference path="./globals.d.ts" />
// Core bootstrap shared by the component modules: htmx hardening, the
// CSRF header, and focus management.

// Hardens htmx: fail closed on dynamic evaluation and never accept
// scripts from swapped fragments.
export function configureHtmx(): void {
  htmx.config.allowEval = false;
  htmx.config.allowScriptTags = false;
  htmx.config.includeIndicatorStyles = false;
}

// Forwards the double-submit CSRF token on every state-changing request.
// The token comes from the server-rendered meta tag, never from the
// cookie (which is HttpOnly).
export function forwardCsrf(): void {
  document.addEventListener('htmx:configRequest', (evt) => {
    const detail = (evt as CustomEvent<HtmxRequestDetail>).detail;
    const meta = document.querySelector('meta[name="csrf-token"]');
    const token = meta?.getAttribute('content') ?? '';
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

// Reports the exact throw site of htmx's internal errors: the source
// map of the minified bundle is unreliable on the old engines, but
// Firefox errors carry fileName/lineNumber/columnNumber directly.
export function reportHtmxErrors(): void {
  document.addEventListener('htmx:onLoadError', (evt) => {
    const detail = (evt as CustomEvent<{ error: Error }>).detail;
    const e = detail.error as Error & { fileName?: string; lineNumber?: number; columnNumber?: number };
    console.error('htmx throw-site:', e.fileName, e.lineNumber, e.columnNumber);
    console.error('htmx error stack:', e.stack);
  });
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

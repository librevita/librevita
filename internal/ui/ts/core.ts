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

// Strips the full-document wrapper from boosted responses before the
// swap: the server renders complete pages, and old engines (the
// Firefox 45-based PowerPC browsers) choke on parsing/swapping a
// doctype + html/head document into the body. Fragment responses
// (search, pager) have no wrapper and pass through unchanged. The
// title is kept in sync since the head is discarded.
export function stripFullDocument(): void {
  document.addEventListener('htmx:beforeSwap', (evt) => {
    const detail = (evt as CustomEvent<HtmxBeforeSwapDetail>).detail;
    const response = detail.serverResponse;
    if (typeof response !== 'string' || !/<!doctype html/i.test(response)) {
      return;
    }
    const title = /<title[^>]*>([\s\S]*?)<\/title>/i.exec(response);
    if (title) {
      document.title = title[1].trim();
    }
    const body = /<body[^>]*>([\s\S]*)<\/body>/i.exec(response);
    detail.serverResponse = body ? body[1] : response;
  });
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

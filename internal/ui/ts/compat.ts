// Compatibility layer for the XP floor (Firefox 52 ESR / Goanna), the
// PowerPC-era engines (TenFourFox/AquaFox, Firefox 45-based) and the
// ES3-era engine of Safari 4.1.3 (WebKit 533). Every polyfill is
// feature-detected and removable when the floor moves on.
//
// Only DOM APIs live here: the ECMAScript built-ins (bind, Object.assign,
// Array.from, includes, WeakMap, ...) come from core-js, imported by
// main.ts and bundled inline by esbuild. The ambient declarations below
// are for APIs the TS lib (ES2017) already knows; the polyfills for the
// rest live with their own declarations.
//
// Do not add core-js here: keep this file small and auditable.

// location.origin (Safari 7+): htmx's verifyPath falls back to it when
// the URL constructor is missing (WebKit 533 has neither); the getter
// is defined on the host object when the engine allows it.
if (typeof window !== 'undefined' && window.location && !('origin' in window.location)) {
  try {
    Object.defineProperty(window.location, 'origin', {
      configurable: true,
      get: function (): string {
        return window.location.protocol + '//' + window.location.host;
      },
    });
  } catch (e) {
    // Host object refused the definition; the URL-constructor path is
    // unavailable too, so htmx's fallback keeps failing.
  }
}

// URL constructor (Safari 6+): htmx's verifyPath needs it; without it
// the engine falls back to location.origin, which WebKit 533 lacks
// (and refuses to polyfill on the host object). Minimal parser for the
// absolute and base-relative usages the bundle makes.
if (typeof window !== 'undefined' && typeof window.URL === 'undefined') {
  const URLPoly = function (this: Record<string, string>, url: string, base?: string): void {
    let href = url;
    if (base) {
      if (!/^[a-z][a-z0-9+.-]*:/i.test(href)) {
        if (href.indexOf('//') === 0) {
          const scheme = /^[a-z][a-z0-9+.-]*:/i.exec(base);
          href = (scheme ? scheme[0] : 'http:') + href;
        } else {
          const origin = /^[a-z][a-z0-9+.-]*:\/\/[^/]*/i.exec(base);
          const o = origin ? origin[0] : '';
          href = href.charAt(0) === '/' ? o + href : o + '/' + href;
        }
      }
    }
    const scheme = /^([a-z][a-z0-9+.-]*:)/i.exec(href);
    const rest = scheme ? href.slice(scheme[0].length) : href;
    const host = /^\/\/([^/]*)/.exec(rest);
    const afterHost = host ? rest.slice(host[0].length) : rest;
    const hash = afterHost.indexOf('#') >= 0 ? afterHost.slice(afterHost.indexOf('#')) : '';
    const noHash = hash ? afterHost.slice(0, afterHost.indexOf('#')) : afterHost;
    const search = noHash.indexOf('?') >= 0 ? noHash.slice(noHash.indexOf('?')) : '';
    const pathname = search ? noHash.slice(0, noHash.indexOf('?')) : noHash;
    this.protocol = scheme ? scheme[0] : '';
    this.host = host ? host[1] : '';
    this.origin = this.protocol + '//' + this.host;
    this.pathname = pathname;
    this.search = search;
    this.hash = hash;
    this.href = href;
  };
  const win = window as unknown as Record<string, unknown>;
  win.URL = URLPoly;
}

if (typeof document !== 'undefined' && typeof document.evaluate === 'function') {
  const evaluate = document.evaluate;
  document.evaluate = function (
    expression: string,
    contextNode: Node,
    resolver: XPathNSResolver | null,
    type?: number,
    result?: XPathResult | null,
  ): XPathResult {
    return evaluate.call(
      document,
      expression,
      contextNode,
      resolver === undefined ? null : resolver,
      type === undefined ? XPathResult.ORDERED_NODE_ITERATOR_TYPE : type,
      result === undefined ? null : result,
    );
  };
}

export {};

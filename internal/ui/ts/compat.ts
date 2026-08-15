// Compatibility layer for the XP floor (Firefox 52 ESR / Goanna) and
// the PowerPC-era engines (TenFourFox/AquaFox, Firefox 45-based) that
// still browse the app. Every polyfill is feature-detected and
// removable when the floor moves on. Do not add core-js: keep this
// file small and auditable.
//
// The ambient declarations below are required because the TS lib (ES2017)
// does not know these APIs; the polyfill and its declaration must always
// change together.

declare global {
  interface Window {
    globalThis: Window & typeof globalThis;
    queueMicrotask: (callback: () => void) => void;
  }
  interface Array<T> {
    flat<U>(this: U[], depth?: number): U[];
    flatMap<U, R>(this: U[], mapper: (value: U, index: number, array: U[]) => R, thisArg?: unknown): R[];
    at(index: number): T | undefined;
  }
  interface ObjectConstructor {
    fromEntries(entries: Iterable<readonly [PropertyKey, any]>): { [key: string]: any };
  }
}

if (typeof window !== 'undefined') {
  if (typeof window.globalThis === 'undefined') {
    window.globalThis = window;
  }
  if (typeof window.queueMicrotask !== 'function') {
    // Firefox 52 has Promise; a microtask queue is enough.
    window.queueMicrotask = (callback) => {
      Promise.resolve().then(callback);
    };
  }
}

if (typeof Array.prototype.flat !== 'function') {
  Array.prototype.flat = function (this: unknown[], depth?: number) {
    const maxDepth = depth === undefined ? 1 : depth;
    const result: unknown[] = [];
    const flatten = (list: unknown[], level: number) => {
      for (let i = 0; i < list.length; i++) {
        const item = list[i];
        if (Array.isArray(item) && level > 0) {
          flatten(item, level - 1);
        } else {
          result.push(item);
        }
      }
    };
    flatten(this, maxDepth);
    return result;
  } as Array<unknown>['flat'];
}

if (typeof Array.prototype.flatMap !== 'function') {
  Array.prototype.flatMap = function <U, R>(
    this: U[],
    mapper: (value: U, index: number, array: U[]) => R,
    thisArg?: unknown,
  ): R[] {
    return this.map(mapper, thisArg).flat();
  };
}

// Array.prototype.at (ES2022): missing on the old engines; htmx 2.0
// uses it in a few helpers.
if (typeof Array.prototype.at !== 'function') {
  Array.prototype.at = function (index: number): unknown {
    const n = Number(index);
    const len = this.length;
    const i = n >= 0 ? n : len + n;
    return i < 0 || i >= len ? undefined : this[i];
  };
}

// Object.fromEntries (ES2019): htmx 2.0 uses it when serializing forms.
if (typeof Object.fromEntries !== 'function') {
  Object.fromEntries = function (entries: Iterable<readonly [PropertyKey, unknown]>): Record<string, unknown> {
    const out: Record<string, unknown> = {};
    for (const [key, value] of entries) {
      out[String(key)] = value;
    }
    return out;
  };
}

// Object.values (ES2017): missing on the Firefox 45-based engines.
if (typeof Object.values !== 'function') {
  Object.values = function (obj: object): unknown[] {
    return Object.keys(obj).map((key) => (obj as Record<string, unknown>)[key]);
  };
}

// Element.prototype.append (Firefox 49): htmx 2.0 uses it while
// settling swapped fragments, and DocumentFragment.append while
// extracting the body of full-document responses.
if (typeof Element !== 'undefined' && typeof Element.prototype.append !== 'function') {
  Element.prototype.append = function (...nodes: Array<Node | string>): void {
    for (const node of nodes) {
      this.appendChild(typeof node === 'string' ? document.createTextNode(node) : node);
    }
  };
}
if (
  typeof DocumentFragment !== 'undefined' &&
  typeof DocumentFragment.prototype.append !== 'function'
) {
  DocumentFragment.prototype.append = function (...nodes: Array<Node | string>): void {
    for (const node of nodes) {
      this.appendChild(typeof node === 'string' ? document.createTextNode(node) : node);
    }
  };
}

// Old Gecko throws "SyntaxError: An invalid or illegal string was
// specified" for history URLs it cannot parse (relative or empty paths
// that modern engines accept). Normalize against the current location
// and never let a failed history update break the navigation.
if (typeof history !== 'undefined') {
  const pushState = history.pushState.bind(history);
  const replaceState = history.replaceState.bind(history);
  const normalizeUrl = (url: string | undefined): string => {
    if (!url) {
      return location.href;
    }
    try {
      return new URL(url, location.href).href;
    } catch {
      return location.href;
    }
  };
  history.pushState = function (state: unknown, title: string, url?: string | null): void {
    try {
      pushState(state, title, url === null || url === undefined ? undefined : normalizeUrl(url));
    } catch {
      // The URL could not be represented; the navigation already happened.
    }
  };
  history.replaceState = function (state: unknown, title: string, url?: string | null): void {
    try {
      replaceState(state, title, url === null || url === undefined ? undefined : normalizeUrl(url));
    } catch {
      // The URL could not be represented; the navigation already happened.
    }
  };
}

// Old Gecko does not support the `i` (case-insensitive) flag of
// attribute selectors — `[hx-history="false" i]` throws "An invalid or
// illegal string was specified". htmx 2.0 uses it in
// saveCurrentPageToHistory; strip the flag from selectors so the query
// does not throw (the attribute values are matched as-is anyway).
const stripSelectorCaseFlag = (selectors: string): string =>
  selectors.replace(/\s+i(?=\])/g, '');

if (typeof Document !== 'undefined') {
  const docQuery = Document.prototype.querySelector;
  const docQueryAll = Document.prototype.querySelectorAll;
  if (typeof docQuery === 'function') {
    Document.prototype.querySelector = function (selectors: string): Element | null {
      return docQuery.call(this, stripSelectorCaseFlag(selectors));
    };
  }
  if (typeof docQueryAll === 'function') {
    Document.prototype.querySelectorAll = function (selectors: string): NodeListOf<Element> {
      return docQueryAll.call(this, stripSelectorCaseFlag(selectors));
    };
  }
  const elQuery = Element.prototype.querySelector;
  const elQueryAll = Element.prototype.querySelectorAll;
  if (typeof elQuery === 'function') {
    Element.prototype.querySelector = function (selectors: string): Element | null {
      return elQuery.call(this, stripSelectorCaseFlag(selectors));
    };
  }
  if (typeof elQueryAll === 'function') {
    Element.prototype.querySelectorAll = function (selectors: string): NodeListOf<Element> {
      return elQueryAll.call(this, stripSelectorCaseFlag(selectors));
    };
  }
}

// Node.getRootNode (Firefox 53+): htmx 2.0 calls it (with composed:
// true) at the top of every click handler to decide whether the target
// belongs to the document; without it the handler throws before
// preventDefault and boosted links navigate normally. The old engines
// have no shadow DOM, so walking up to the top node is equivalent.
if (typeof Node !== 'undefined' && typeof Node.prototype.getRootNode !== 'function') {
  Node.prototype.getRootNode = function (): Node {
    let node: Node = this;
    while (node.parentNode) {
      node = node.parentNode;
    }
    return node;
  };
}

// htmx 2.0 checks `elt.parentNode instanceof ShadowRoot` in its parent
// lookup; the old engines have no ShadowRoot constructor at all, and a
// bare reference throws. Provide a stub whose instanceof is always
// false — the application uses no shadow DOM.
if (typeof ShadowRoot === 'undefined') {
  (window as unknown as { ShadowRoot: unknown }).ShadowRoot = function ShadowRoot(): void {};
}

// Old Gecko (TenFourFox/AquaFox, Firefox 45-era) requires the
// nsResolver argument of XPathEvaluator.createExpression; modern
// engines treat it as optional, and htmx 2.0 calls it with one
// argument. Pass null explicitly when the engine demands it.
if (typeof XPathEvaluator !== 'undefined' && typeof XPathEvaluator.prototype.createExpression === 'function') {
  const createExpression = XPathEvaluator.prototype.createExpression;
  XPathEvaluator.prototype.createExpression = function (
    expression: string,
    resolver: XPathNSResolver | null,
  ): XPathExpression {
    return createExpression.call(this, expression, resolver === undefined ? null : resolver);
  };
}

// The same engines also require the type and result arguments of
// XPathExpression.evaluate; htmx 2.0 calls it with just the context
// node and expects an iterator (iterateNext), so default to
// ORDERED_NODE_ITERATOR_TYPE, the spec default on modern engines.
if (typeof XPathExpression !== 'undefined' && typeof XPathExpression.prototype.evaluate === 'function') {
  const evaluate = XPathExpression.prototype.evaluate;
  XPathExpression.prototype.evaluate = function (
    contextNode: Node,
    type?: number,
    result?: XPathResult | null,
  ): XPathResult {
    return evaluate.call(
      this,
      contextNode,
      type === undefined ? XPathResult.ORDERED_NODE_ITERATOR_TYPE : type,
      result === undefined ? null : result,
    );
  };
}

export {};

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

// htmx 1.9 calls document.evaluate with just the expression and the
// context node; old Gecko (TenFourFox/AquaFox, Firefox 45-era)
// requires all five arguments. Default to an ordered node iterator,
// which is what the caller expects (iterateNext).
if (typeof document !== 'undefined' && typeof document.evaluate === 'function') {
  const evaluate = document.evaluate.bind(document);
  document.evaluate = function (
    expression: string,
    contextNode: Node,
    resolver: XPathNSResolver | null,
    type?: number,
    result?: XPathResult | null,
  ): XPathResult {
    return evaluate(
      expression,
      contextNode,
      resolver === undefined ? null : resolver,
      type === undefined ? XPathResult.ORDERED_NODE_ITERATOR_TYPE : type,
      result === undefined ? null : result,
    );
  };
}

export {};

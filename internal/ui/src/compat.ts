// Compatibility layer for the XP floor (Firefox 52 ESR / Goanna).
// Every polyfill is feature-detected and removable when the floor moves
// on. Do not add core-js: keep this file small and auditable.
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
    // Firefox 52 has Promise; a microtask queue is enough for Alpine.
    window.queueMicrotask = (callback) => {
      Promise.resolve().then(callback);
    };
  }
}

if (typeof Array.prototype.flat !== 'function') {
  Array.prototype.flat = function (depth) {
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
  Array.prototype.flatMap = function (mapper, thisArg) {
    return this.map(mapper, thisArg).flat();
  };
}

export {};

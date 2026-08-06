// Compatibility layer for the XP floor (Firefox 52 ESR / Goanna).
// Every polyfill is feature-detected and removable when the floor moves
// on. Do not add core-js: keep this file small and auditable.
//
// The rest of the frontend may use modern syntax freely; esbuild
// transpiles it (target=firefox52). API usage must stay within this
// baseline or be polyfilled here.

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
    const result = [];
    const flatten = (list, level) => {
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
  };
}

if (typeof Array.prototype.flatMap !== 'function') {
  Array.prototype.flatMap = function (mapper, thisArg) {
    return this.map(mapper, thisArg).flat();
  };
}

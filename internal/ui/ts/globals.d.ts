// Ambient declarations for the globals the frontend uses. The runtime
// is htmx 2.x: the package ships its own types (dist/htmx.esm.d.ts,
// typed as the module default export); the global `htmx` used by
// core.ts is declared from them below. The interfaces at the end are
// the event payloads this application consumes.

// The CommonJS wrapper bundled by build.ts: esbuild resolves the
// module.exports the same way it did the htmx 1.x UMD, and
// htmx-runtime.ts publishes the global explicitly.
declare module 'htmx.org/dist/htmx.cjs.js' {
  const htmx: typeof import('htmx.org/dist/htmx.esm.js').default;
  export default htmx;
}
declare module 'htmx-ext-sse/dist/sse.js';

declare const htmx: typeof import('htmx.org/dist/htmx.esm.js').default;

declare interface HtmxRequestDetail {
  headers: Record<string, string>;
}

declare interface HtmxAfterSwapDetail {
  elt: HTMLElement | null;
  target: HTMLElement | null;
}


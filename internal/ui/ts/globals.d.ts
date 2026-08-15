// Ambient declarations for the globals the frontend uses. The global
// `htmx` object and its config come from the htmx-types package (htmx
// 1.9); the interfaces below are the event payloads this application
// consumes.

/// <reference types="htmx-types" />

// The UMD runtime bundled by build.ts: esbuild resolves the
// module.exports the same way it did for the 1.x dist, and
// htmx-runtime.ts publishes the global explicitly.
declare module 'htmx.org/dist/htmx.min.js' {
  const htmx: unknown;
  export default htmx;
}
declare module 'htmx.org/dist/ext/sse.js';

declare const __LV_DEV__: boolean;

declare interface HtmxRequestDetail {
  headers: Record<string, string>;
}

declare interface HtmxAfterSwapDetail {
  elt: HTMLElement | null;
  target: HTMLElement | null;
}

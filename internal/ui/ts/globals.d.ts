// Ambient declarations for the globals the frontend uses. The global
// `htmx` object and its config come from the htmx-types package; the
// interfaces below are the event payloads this application consumes.
/// <reference types="htmx-types" />

// The HTMX runtime is bundled into the application bundle (main.ts
// imports these modules); the UMD wrapper resolves through the
// CommonJS branch under esbuild and never assigns the global, so the
// api object is published explicitly by htmx-runtime.ts.
declare module 'htmx.org/dist/htmx.min.js' {
  const htmx: unknown;
  export default htmx;
}
declare module 'htmx.org/dist/ext/sse.js';

declare interface HtmxRequestDetail {
  headers: Record<string, string>;
}

declare interface HtmxAfterSwapDetail {
  elt: HTMLElement | null;
  target: HTMLElement | null;
}


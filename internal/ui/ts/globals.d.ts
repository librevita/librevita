// Ambient declarations for the globals the frontend uses. The global
// `htmx` object and its config come from the htmx-types package; the
// interfaces below are the event payloads this application consumes.
/// <reference types="htmx-types" />

declare interface HtmxRequestDetail {
  headers: Record<string, string>;
}

declare interface HtmxAfterSwapDetail {
  elt: HTMLElement | null;
}


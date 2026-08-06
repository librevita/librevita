// Ambient declarations for the globals the frontend uses. Kept minimal and
// aligned with the XP floor: the TS lib (ES2017 + DOM) already rejects APIs
// missing from Firefox 52 / Goanna.

interface HtmxConfig {
  allowEval: boolean;
  allowScriptTags: boolean;
  includeIndicatorStyles: boolean;
}

declare const htmx: {
  config: HtmxConfig;
};

declare interface HtmxRequestDetail {
  headers: Record<string, string>;
}

declare const Alpine: {
  start: () => void;
  data: <T>(name: string, factory: () => T) => void;
};

interface Window {
  Alpine?: typeof Alpine;
}

// Frontend entry: the theme bootstrap runs first (it must apply the
// dark class before first paint), then every component module (this
// import list IS the manifest) is registered and the core bootstrap
// wired. Each module exposes init() (idempotent, delegated listeners)
// and refresh(root) (resync after htmx swaps where needed). This file
// is the single application bundle (build.ts): it is loaded blocking
// in the head, after the htmx runtime.

// core-js polyfills for the built-ins WebKit 533 lacks. They are
// bundled inline by esbuild (before the Babel es5 lowering), so no
// post-bundle imports; only the modules the application actually uses.
import 'core-js/modules/es.function.bind.js';
import 'core-js/modules/es.object.assign.js';
import 'core-js/modules/es.array.from.js';
import 'core-js/modules/es.array.includes.js';
import 'core-js/modules/es.string.includes.js';
import 'core-js/modules/es.weak-map.js';
// DOM polyfills from the ecosystem (bundled inline by esbuild).
import 'classlist-polyfill';
import elementClosest from 'element-closest';
elementClosest(window);
import 'nodelist-foreach';

import './compat.ts';
// The HTMX runtime and its SSE extension are bundled here, so the
// whole client is a single deferred file. htmx-runtime.ts must load
// before the extension (import order is preserved). The theme
// bootstrap is NOT here: it is rendered inline in the head by
// base.templ (build.ts bundles theme.ts separately for that).
import './htmx-runtime.ts';
import 'htmx.org/dist/ext/sse.js';

import { configureHtmx, forwardCsrf, focusAppMain, reportHtmxErrors } from './core.ts';
import * as sidebar from './sidebar.ts';
import * as userMenu from './user-menu.ts';
import * as tabs from './tabs.ts';
import * as dropdown from './dropdown.ts';
import * as modal from './modal.ts';
import * as tableSelect from './table-select.ts';
import * as reveal from './reveal.ts';
import * as datepicker from './datepicker.tsx';

configureHtmx();
forwardCsrf();
// Debug diagnostics for the old engines; only in dev-mode bundles.
if (__LV_DEV__) {
  reportHtmxErrors();
  // htmx 1.9 catches handler errors and logs them through console.error;
  // window.onerror never fires. Intercept to capture the error object
  // (message, line, stack when available) for the old engines.
  const origConsoleError = console.error;
  console.error = function (...args: unknown[]): void {
    origConsoleError.apply(console, args);
    const e = args[0] as Error & { line?: number; sourceId?: number; fileName?: string };
    if (e instanceof Error) {
      origConsoleError.call(
        console,
        'LV-CAUGHT:',
        e.message,
        '| line:', e.line,
        '| sourceId:', e.sourceId,
        '| file:', e.fileName,
        '| stack:', e.stack,
      );
    }
  };
  // Uncaught errors never reach the interceptor above.
  window.onerror = function (message, source, lineno, colno, error) {
    origConsoleError.call(console, 'LV-UNCAUGHT:', message, '|', source, lineno, colno, '|', error);
  };
}

sidebar.init();
userMenu.init();
tabs.init();
tableSelect.init();
modal.init();
dropdown.init();
reveal.init();
datepicker.init();

document.addEventListener('htmx:afterSwap', (evt) => {
  const detail = (evt as CustomEvent<HtmxAfterSwapDetail>).detail;
  const root = detail.elt as HTMLElement | null;
  if (!root) {
    return;
  }
  sidebar.refresh(root);
  userMenu.refresh(root);
  tabs.refresh(root);
  tableSelect.refresh(root);
  modal.refresh(root);
  dropdown.refresh(root);
  reveal.refresh(root);
  datepicker.refresh(root);
  // Only page navigations (boosted swaps target the body) move the focus
  // to the main content; fragment swaps (search, pager, row updates) must
  // leave the focus where the user is typing or clicking.
  const target = detail.target as HTMLElement | null;
  if (target && target.tagName.toLowerCase() === 'body') {
    focusAppMain();
  }
});

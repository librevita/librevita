// Frontend entry: the theme bootstrap runs first (it must apply the
// dark class before first paint), then every component module (this
// import list IS the manifest) is registered and the core bootstrap
// wired. Each module exposes init() (idempotent, delegated listeners)
// and refresh(root) (resync after htmx swaps where needed). This file
// is the single application bundle (build.ts): it is loaded blocking
// in the head, after the htmx runtime.

import './compat.ts';
// The HTMX runtime and its SSE extension are bundled here, so the
// whole client is a single deferred file. htmx-runtime.ts must load
// before the extension (import order is preserved). The theme
// bootstrap is NOT here: it is rendered inline in the head by
// base.templ (build.ts bundles theme.ts separately for that).
import './htmx-runtime.ts';
import 'htmx.org/dist/ext/sse.js';
import 'htmx.org/dist/ext/alpine-morph.js';

import { configureHtmx, forwardCsrf, focusAppMain, reportHtmxErrors } from './core.ts';
import * as sidebar from './sidebar.ts';
import * as userMenu from './user-menu.ts';
import * as tabs from './tabs.ts';
import * as dropdown from './dropdown.ts';
import * as modal from './modal.ts';
import * as tableSelect from './table-select.ts';
import * as reveal from './reveal.ts';
import Alpine from '@alpinejs/csp';
import morph from '@alpinejs/morph';
import focus from '@alpinejs/focus';
import * as datepicker from './datepicker.ts';
import { registerDatepicker } from './datepicker.ts';
import { registerAgenda } from './calendar.ts';

configureHtmx();
forwardCsrf();
// Debug diagnostics for the old engines; only in dev-mode bundles.
if (__LV_DEV__) {
  reportHtmxErrors();
}

sidebar.init();
userMenu.init();
tabs.init();
tableSelect.init();
modal.init();
dropdown.init();
reveal.init();
datepicker.init();
// The alpine-morph htmx extension morphs swapped fragments with the
// Alpine morph plugin (Alpine.morph), preserving component state.
Alpine.plugin(morph);
Alpine.plugin(focus);
(window as unknown as Record<string, unknown>).Alpine = Alpine;
registerDatepicker(Alpine);
registerAgenda(Alpine);
// Alpine processes the x-data trees (the datepicker popover); the
// CSP build runs without unsafe-eval.
Alpine.start();

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

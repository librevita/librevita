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
import { registerSidebar } from './sidebar.ts';
import { registerUserMenu } from './user-menu.ts';
import * as tabs from './tabs.ts';
import { registerDropdown } from './dropdown.ts';
import { registerModal } from './modal.ts';
import * as reveal from './reveal.ts';
import Alpine from '@alpinejs/csp';
import morph from '@alpinejs/morph';
import focus from '@alpinejs/focus';
import collapse from '@alpinejs/collapse';
import mask from '@alpinejs/mask';
import { registerDatepicker } from './datepicker.ts';
import { registerAgenda } from './calendar.ts';
import { registerIdentifiermask } from './identifiermask.ts';
import { registerSearchMenu } from './search-menu.ts';
import { registerStatusMenu } from './status-menu.ts';

configureHtmx();
forwardCsrf();
// Debug diagnostics for the old engines; only in dev-mode bundles.
if (__LV_DEV__) {
  reportHtmxErrors();
}

tabs.init();
reveal.init();
// The alpine-morph htmx extension morphs swapped fragments with the
// Alpine morph plugin (Alpine.morph), preserving component state.
Alpine.plugin(morph);
Alpine.plugin(focus);
Alpine.plugin(collapse);
Alpine.plugin(mask);
(window as unknown as Record<string, unknown>).Alpine = Alpine;
registerDatepicker(Alpine);
registerAgenda(Alpine);
registerIdentifiermask(Alpine);
registerDropdown(Alpine);
registerModal(Alpine);
registerSearchMenu(Alpine);
registerStatusMenu(Alpine);
registerSidebar(Alpine);
registerUserMenu(Alpine);
// Alpine processes the x-data trees (the datepicker popover); the
// CSP build runs without unsafe-eval.
Alpine.start();

document.addEventListener('htmx:afterSwap', (evt) => {
  const detail = (evt as CustomEvent<HtmxAfterSwapDetail>).detail;
  const root = detail.elt as HTMLElement | null;
  if (!root) {
    return;
  }
  tabs.refresh(root);
  reveal.refresh(root);
  // Only page navigations (boosted swaps target the body) move the focus
  // to the main content; fragment swaps (search, pager, row updates) must
  // leave the focus where the user is typing or clicking.
  const target = detail.target as HTMLElement | null;
  if (target && target.tagName.toLowerCase() === 'body') {
    focusAppMain();
  }
});

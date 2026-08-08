// Frontend entry: registers every component module (this import list IS
// the manifest) and wires the core bootstrap. Each module exposes
// init() (idempotent, delegated listeners) and refresh(root) (resync
// after htmx swaps where needed).

import './compat.ts';

import { configureHtmx, forwardCsrf, focusAppMain } from './core.ts';
import * as sidebar from './sidebar.ts';
import * as userMenu from './user-menu.ts';
import * as tabs from './tabs.ts';
import * as themePref from './theme-pref.ts';
import * as tableSelect from './table-select.ts';

configureHtmx();
forwardCsrf();

sidebar.init();
userMenu.init();
tabs.init();
themePref.init();
tableSelect.init();

document.addEventListener('htmx:afterSwap', (evt) => {
  const detail = (evt as CustomEvent<HtmxAfterSwapDetail>).detail;
  const root = detail.elt as HTMLElement | null;
  if (!root) {
    return;
  }
  sidebar.refresh(root);
  userMenu.refresh(root);
  tabs.refresh(root);
  themePref.refresh(root);
  tableSelect.refresh(root);
  // Only page navigations (boosted swaps target the body) move the focus
  // to the main content; fragment swaps (search, pager, row updates) must
  // leave the focus where the user is typing or clicking.
  const target = detail.target as HTMLElement | null;
  if (target && target.tagName.toLowerCase() === 'body') {
    focusAppMain();
  }
});

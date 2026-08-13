// Frontend entry: registers every component module (this import list IS
// the manifest) and wires the core bootstrap. Each module exposes
// init() (idempotent, delegated listeners) and refresh(root) (resync
// after htmx swaps where needed).

import './compat.ts';

import { configureHtmx, forwardCsrf, focusAppMain } from './core.ts';
import * as sidebar from './sidebar.ts';
import * as userMenu from './user-menu.ts';
import * as tabs from './tabs.ts';
import * as dropdown from './dropdown.ts';
import * as modal from './modal.ts';
import * as tableSelect from './table-select.ts';
import * as reveal from './reveal.ts';
import * as datepicker from './datepicker.ts';

configureHtmx();
forwardCsrf();

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

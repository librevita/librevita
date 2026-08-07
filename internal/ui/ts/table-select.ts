// Registry table selection: the header checkbox toggles every visible
// row and reflects whether all of them are checked. refresh resyncs the
// header after the search/load-more swaps replace the table.

const GROUP_SELECTOR = '[data-lv-table-select]';
const SELECT_ALL_SELECTOR = '[data-lv-select-all]';
const ROW_BOX_SELECTOR = 'input[name="ids"]:not([value="all"])';

export function init(): void {
  document.addEventListener('click', (evt) => {
    const check = closestOrSelf(evt.target, SELECT_ALL_SELECTOR);
    if (!check || !(check instanceof HTMLInputElement)) {
      return;
    }
    const group = check.closest(GROUP_SELECTOR);
    if (!group) {
      return;
    }
    const boxes = rowBoxes(group);
    for (let i = 0; i < boxes.length; i++) {
      boxes[i].checked = check.checked;
    }
  });
  document.addEventListener('change', (evt) => {
    const target = evt.target;
    if (!(target instanceof HTMLInputElement) || !target.matches(ROW_BOX_SELECTOR)) {
      return;
    }
    const group = target.closest(GROUP_SELECTOR);
    if (group) {
      syncHeader(group);
    }
  });
}

export function refresh(root: HTMLElement): void {
  const group = root.querySelector(GROUP_SELECTOR);
  if (group) {
    syncHeader(group);
  }
}

export function rowBoxes(group: Element): HTMLInputElement[] {
  const out: HTMLInputElement[] = [];
  const nodes = group.querySelectorAll(ROW_BOX_SELECTOR);
  for (let i = 0; i < nodes.length; i++) {
    out.push(nodes[i] as HTMLInputElement);
  }
  return out;
}

function syncHeader(group: Element): void {
  const check = group.querySelector(SELECT_ALL_SELECTOR);
  if (!(check instanceof HTMLInputElement)) {
    return;
  }
  const boxes = rowBoxes(group);
  let checked = 0;
  for (let i = 0; i < boxes.length; i++) {
    if (boxes[i].checked) {
      checked++;
    }
  }
  check.checked = boxes.length > 0 && checked === boxes.length;
}

import { closestOrSelf } from './core.ts';

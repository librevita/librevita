// Element factory for the TSX pipeline. TSX compiles to
// h(tag, props, ...children) and returns plain HTMLElement nodes: no
// virtual DOM, no reactivity, no reconciliation. Children are always
// positional arguments (classic JSX mode); a `children` key inside
// props is ignored. For components (tag function), the normalized
// children are delivered through props, matching the classic contract.
// Unsupported on purpose: fragments, SVG (namespaceURI), innerHTML and
// ref/key.

export type Child = Node | string | number | boolean | null | undefined;
export type Children = Child | Children[];

export type JsxProps = {
  class?: string;
  className?: string;
  id?: string;
  style?: string | Partial<CSSStyleDeclaration>;
  dataset?: Record<string, string>;
  onClick?: (event: MouseEvent) => void;
  onChange?: (event: Event) => void;
  onInput?: (event: Event) => void;
  onSubmit?: (event: Event) => void;
  onKeydown?: (event: KeyboardEvent) => void;
  onKeyup?: (event: KeyboardEvent) => void;
  onFocus?: (event: FocusEvent) => void;
  onBlur?: (event: FocusEvent) => void;
  onScroll?: (event: Event) => void;
  onMouseenter?: (event: MouseEvent) => void;
  onMouseleave?: (event: MouseEvent) => void;
  // Template-literal index signatures (TS 4.4+): data-*, aria-* and
  // hx-* always become attributes. hx-* is what htmx's delegated
  // listeners read at runtime, so it must reach the DOM verbatim.
  [key: `data-${string}`]: string | number | boolean | undefined;
  [key: `aria-${string}`]: string | boolean | undefined;
  [key: `hx-${string}`]: string | undefined;
  [key: string]: unknown;
};

export type ComponentProps = JsxProps & { children?: Children };
export type Component = (props: ComponentProps) => HTMLElement;

declare global {
  namespace JSX {
    type Element = HTMLElement;
    interface IntrinsicElements {
      [elemName: string]: JsxProps;
    }
  }
}

// Keys assigned as live DOM properties (when the key exists on the
// element) instead of attributes; boolean and numeric values stay raw.
// `value` and `checked` must round-trip as properties so form state is
// not lost; `hidden`/`disabled`/`tabIndex` behave correctly either way.
const PROPERTY_KEYS = [
  'checked',
  'disabled',
  'hidden',
  'muted',
  'multiple',
  'open',
  'readOnly',
  'required',
  'selected',
  'tabIndex',
  'value',
];

export function h<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  props?: JsxProps | null,
  ...children: Children[]
): HTMLElementTagNameMap[K];
export function h(
  tag: string,
  props?: JsxProps | null,
  ...children: Children[]
): HTMLElement;
export function h<P extends JsxProps>(
  tag: (props: P & { children?: Children }) => HTMLElement,
  props?: P | null,
  ...children: Children[]
): HTMLElement;
export function h(
  tag: string | ((props: ComponentProps) => HTMLElement),
  props?: JsxProps | null,
  ...children: Children[]
): HTMLElement {
  if (typeof tag === 'function') {
    const componentProps: ComponentProps = Object.assign({}, props, {
      children: normalizeChildren(children),
    });
    return tag(componentProps);
  }
  const el = document.createElement(tag);
  if (props) {
    applyProps(el, props);
  }
  appendChildren(el, children);
  return el;
}

function normalizeChildren(children: Children[]): Children {
  if (children.length === 0) {
    return undefined;
  }
  if (children.length === 1) {
    return children[0];
  }
  return children;
}

function applyProps(el: HTMLElement, props: JsxProps): void {
  for (const key of Object.keys(props)) {
    const value = (props as unknown as Record<string, unknown>)[key];
    if (value === null || value === undefined) {
      continue;
    }
    // ARIA semantics are strings: true -> "true", false -> "false".
    // Unlike HTML boolean attributes, false must not drop the attribute.
    if (key.startsWith('aria-')) {
      el.setAttribute(key, String(value));
      continue;
    }
    if (value === false) {
      continue;
    }
    if (key === 'class' || key === 'className') {
      el.className = String(value);
      continue;
    }
    if (key === 'style') {
      applyStyle(el, value as string | Partial<CSSStyleDeclaration>);
      continue;
    }
    if (key === 'dataset') {
      applyDataset(el, value as Record<string, string>);
      continue;
    }
    // Event listeners: onX -> addEventListener("x", handler).
    if (key.length > 2 && key.startsWith('on') && typeof value === 'function') {
      el.addEventListener(key.slice(2).toLowerCase(), value as EventListener);
      continue;
    }
    if (PROPERTY_KEYS.indexOf(key) !== -1 && key in el) {
      (el as unknown as Record<string, unknown>)[key] = value;
      continue;
    }
    if (typeof value === 'boolean') {
      el.setAttribute(key, '');
      continue;
    }
    el.setAttribute(key, String(value));
  }
}

function applyStyle(
  el: HTMLElement,
  style: string | Partial<CSSStyleDeclaration>,
): void {
  if (typeof style === 'string') {
    el.setAttribute('style', style);
    return;
  }
  // CamelCase keys (position, fontSize) assign onto el.style directly;
  // setProperty would need kebab-case, which the props object does not
  // use.
  const record = el.style as unknown as Record<string, string>;
  for (const prop of Object.keys(style)) {
    const value = (style as unknown as Record<string, unknown>)[prop];
    if (value !== null && value !== undefined) {
      record[prop] = String(value);
    }
  }
}

function applyDataset(el: HTMLElement, dataset: Record<string, string>): void {
  for (const key of Object.keys(dataset)) {
    const value = dataset[key];
    if (value !== null && value !== undefined) {
      el.dataset[key] = value;
    }
  }
}

function appendChildren(el: HTMLElement, children: Children[]): void {
  for (const child of children) {
    appendChild(el, child);
  }
}

function appendChild(el: HTMLElement, child: Children): void {
  if (child === null || child === undefined || typeof child === 'boolean') {
    return;
  }
  // Nested arrays flatten in place: <div>{items.map(...)}</div>.
  if (Array.isArray(child)) {
    for (const item of child) {
      appendChild(el, item);
    }
    return;
  }
  if (child instanceof Node) {
    el.appendChild(child);
    return;
  }
  // Text goes through createTextNode: the value is data, never markup.
  el.appendChild(document.createTextNode(String(child)));
}

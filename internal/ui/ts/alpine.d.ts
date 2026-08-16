// Typing for @alpinejs/csp (the CSP-safe Alpine build). The package
// ships no declarations; only the surface the app uses is declared.
declare module '@alpinejs/csp' {
  // The `this` available inside Alpine.data component methods: the
  // component state plus the Alpine magics the app relies on.
  export interface AlpineComponent {
    $refs: Record<string, HTMLElement>;
    $root: HTMLElement;
    $nextTick(callback: () => void): void;
    [key: string]: any;
  }

  export interface Alpine {
    data(name: string, factory: () => AlpineComponent): void;
    plugin(plugin: (alpine: Alpine) => void): void;
    start(): void;
  }

  declare const alpine: Alpine;
  export default alpine;
}

declare module '@alpinejs/morph' {
  import type { Alpine } from '@alpinejs/csp';
  const morph: (alpine: Alpine) => void;
  export default morph;
}

declare module '@alpinejs/focus' {
  import type { Alpine } from '@alpinejs/csp';
  const focus: (alpine: Alpine) => void;
  export default focus;
}

declare module '@alpinejs/collapse' {
  import type { Alpine } from '@alpinejs/csp';
  const collapse: (alpine: Alpine) => void;
  export default collapse;
}

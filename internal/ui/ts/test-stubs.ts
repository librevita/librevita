// DOM stubs for the linkedom-based unit tests. Import this module FIRST
// in ui_test.ts: ESM imports execute in order, so the globals exist
// before any module (e.g. the Alpine CSP build, which references
// MutationObserver at module scope) is loaded.

class MutationObserverStub {
  constructor(_callback: MutationCallback) {}
  observe(_target: Node, _options?: MutationObserverInit): void {}
  disconnect(): void {}
  takeRecords(): MutationRecord[] {
    return [];
  }
}

(globalThis as Record<string, unknown>).MutationObserver = MutationObserverStub;

// The Alpine CSP build references window/self at module scope; a
// self-reference keeps the import side-effect-free (linkedom's window
// is installed per-test by setDocument).
(globalThis as Record<string, unknown>).window = globalThis;
(globalThis as Record<string, unknown>).self = globalThis;

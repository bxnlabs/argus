// Vitest global test setup.
//
// Node ≥ 22 exposes an experimental `localStorage` global that is `undefined`
// when no --localstorage-file flag is provided.  This shadows jsdom's
// fully-functional `window.localStorage` in the test environment.
// Polyfill globalThis.localStorage with a minimal in-memory implementation so
// tests that call localStorage.* work correctly.

class LocalStorageMock implements Storage {
  private store: Map<string, string> = new Map();

  get length(): number {
    return this.store.size;
  }

  key(index: number): string | null {
    return [...this.store.keys()][index] ?? null;
  }

  getItem(key: string): string | null {
    return this.store.get(key) ?? null;
  }

  setItem(key: string, value: string): void {
    this.store.set(key, String(value));
  }

  removeItem(key: string): void {
    this.store.delete(key);
  }

  clear(): void {
    this.store.clear();
  }
}

if (globalThis.localStorage === undefined) {
  Object.defineProperty(globalThis, "localStorage", {
    value: new LocalStorageMock(),
    writable: true,
    configurable: true,
  });
}

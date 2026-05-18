import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach, beforeEach, vi } from "vitest";

afterEach(() => {
    cleanup();
});

// structuredClone polyfill (CampaignCreatePage uses it)
if (typeof globalThis.structuredClone === "undefined") {
    globalThis.structuredClone = (obj: unknown) =>
        JSON.parse(JSON.stringify(obj));
}

// EventSource mock (useSSE hook)
class MockEventSource {
    url: string;
    onopen: (() => void) | null = null;
    onmessage: ((e: MessageEvent) => void) | null = null;
    onerror: (() => void) | null = null;
    addEventListener = vi.fn();
    close = vi.fn();

    constructor(url: string) {
        this.url = url;
    }
}

Object.defineProperty(globalThis, "EventSource", {
    value: MockEventSource,
    writable: true,
});

// ResizeObserver mock
if (typeof globalThis.ResizeObserver === "undefined") {
    class MockResizeObserver {
        observe = vi.fn();
        unobserve = vi.fn();
        disconnect = vi.fn();
    }
    Object.defineProperty(globalThis, "ResizeObserver", {
        value: MockResizeObserver,
        writable: true,
    });
}

// localStorage polyfill (vitest 4 jsdom may not provide it)
if (typeof globalThis.localStorage === "undefined") {
    const store = new Map<string, string>();
    const storage = {
        getItem: (key: string) => store.get(key) ?? null,
        setItem: (key: string, value: string) => { store.set(key, value); },
        removeItem: (key: string) => { store.delete(key); },
        clear: () => { store.clear(); },
        get length() { return store.size; },
        key: (index: number) => [...store.keys()][index] ?? null,
    };
    Object.defineProperty(globalThis, "localStorage", {
        value: storage,
        writable: true,
    });
}

beforeEach(() => {
    localStorage.clear();
});

// Suppress jsdom requestSubmit warning
const originalError = console.error;
console.error = (...args: unknown[]) => {
    const msg = String(args[0]);
    if (msg.includes("Not implemented: HTMLFormElement.prototype.requestSubmit")) return;
    originalError.call(console, ...args);
};

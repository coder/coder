import { Blob as NativeBlob } from "node:buffer";
import ResizeObserver from "resize-observer-polyfill";

// JSDom `Blob` is missing important methods[1] that have been standardized for
// years. MDN categorizes this API as baseline[2].
// [1]: https://github.com/jsdom/jsdom/issues/2555
// [2]: https://developer.mozilla.org/en-US/docs/Web/API/Blob/arrayBuffer
// @ts-expect-error - Minor type incompatibilities due to TypeScript's
// introduction of the `ArrayBufferLife` type and the related generic parameters
// changes.
globalThis.Blob = NativeBlob;

globalThis.ResizeObserver = ResizeObserver;

// JSDOM does not implement the CSS Font Loading API. The web terminal listens
// for document.fonts loadingdone events. document.fonts is read-only in the
// DOM types, so it is defined rather than assigned.
if (!("fonts" in document)) {
	Object.defineProperty(document, "fonts", {
		configurable: true,
		value: new EventTarget(),
	});
}

// JSDOM does not implement window.matchMedia. Monaco editor (and other
// libraries) rely on it for dark/light theme detection.
if (typeof globalThis.matchMedia !== "function") {
	globalThis.matchMedia = (query: string) =>
		({
			matches: false,
			media: query,
			onchange: null,
			addListener: vi.fn(),
			removeListener: vi.fn(),
			addEventListener: vi.fn(),
			removeEventListener: vi.fn(),
			dispatchEvent: vi.fn(),
		}) as MediaQueryList;
}

import { server } from "testHelpers/server";
import { Blob as NativeBlob } from "node:buffer";
import { cleanup } from "@testing-library/react";
import { afterAll, afterEach, beforeAll, vi } from "vitest";

// JSDom `Blob` is missing important methods[1] that have been standardized for
// years. MDN categorizes this API as baseline[2].
// [1]: https://github.com/jsdom/jsdom/issues/2555
// [2]: https://developer.mozilla.org/en-US/docs/Web/API/Blob/arrayBuffer
// @ts-expect-error - Minor type incompatibilities due to TypeScript's
// introduction of the `ArrayBufferLife` type and the related generic parameters
// changes.
globalThis.Blob = NativeBlob;

globalThis.ResizeObserver = require("resize-observer-polyfill");

// Pointer capture stubs required for Radix UI in JSDOM.
globalThis.HTMLElement.prototype.hasPointerCapture = vi
	.fn()
	.mockReturnValue(false);
globalThis.HTMLElement.prototype.setPointerCapture = vi.fn();
globalThis.HTMLElement.prototype.releasePointerCapture = vi.fn();

// Mock useProxyLatency to avoid real network requests to external proxy URLs.
// Must use useMemo to return stable object references and prevent infinite re-renders.
vi.mock("contexts/useProxyLatency", async () => {
	const { useMemo } = await import("react");
	return {
		useProxyLatency: () => {
			const proxyLatencies = useMemo(() => ({}), []);
			return { proxyLatencies, refetch: () => new Date() };
		},
	};
});

// MSW server lifecycle
beforeAll(() => server.listen({ onUnhandledRequest: "warn" }));
afterEach(() => {
	cleanup();
	server.resetHandlers();
	vi.clearAllMocks();
});
afterAll(() => server.close());

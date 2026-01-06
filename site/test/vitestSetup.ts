import { server } from "testHelpers/server";
import { Blob as NativeBlob } from "node:buffer";
import crypto from "node:crypto";
import { cleanup } from "@testing-library/react";
import type { Region } from "api/typesGenerated";
import type { ProxyLatencyReport } from "contexts/useProxyLatency";
import { useMemo } from "react";
import { afterAll, afterEach, beforeAll, vi } from "vitest";

// JSDom `Blob` is missing important methods[1] that have been standardized for
// years. MDN categorizes this API as baseline[2].
// [1]: https://github.com/jsdom/jsdom/issues/2555
// [2]: https://developer.mozilla.org/en-US/docs/Web/API/Blob/arrayBuffer
// @ts-expect-error - Minor type incompatibilities due to TypeScript's
// introduction of the `ArrayBufferLife` type and the related generic parameters
// changes.
globalThis.Blob = NativeBlob;

// useProxyLatency does some http requests to determine latency.
// This would fail unit testing, or at least make it very slow with
// actual network requests. So just globally mock this hook.
vi.mock("contexts/useProxyLatency", () => ({
	useProxyLatency: (proxies?: Region[]) => {
		// Must use `useMemo` here to avoid infinite loop.
		// Mocking the hook with a hook.
		const proxyLatencies = useMemo(() => {
			if (!proxies) {
				return {} as Record<string, ProxyLatencyReport>;
			}
			return proxies.reduce(
				(acc, proxy) => {
					acc[proxy.id] = {
						accurate: true,
						// Return a constant latency of 8ms.
						// If you make this random it could break stories.
						latencyMS: 8,
						at: new Date(),
					};
					return acc;
				},
				{} as Record<string, ProxyLatencyReport>,
			);
		}, [proxies]);

		return { proxyLatencies, refetch: vi.fn() };
	},
}));

globalThis.scrollTo = vi.fn();

globalThis.HTMLElement.prototype.scrollIntoView = vi.fn();
// Polyfill pointer capture methods for JSDOM compatibility with Radix UI
globalThis.HTMLElement.prototype.hasPointerCapture = vi
	.fn()
	.mockReturnValue(false);
globalThis.HTMLElement.prototype.setPointerCapture = vi.fn();
globalThis.HTMLElement.prototype.releasePointerCapture = vi.fn();
globalThis.open = vi.fn();
navigator.sendBeacon = vi.fn();

globalThis.ResizeObserver = require("resize-observer-polyfill");

// Polyfill the getRandomValues that is used on utils/random.ts
Object.defineProperty(globalThis.self, "crypto", {
	value: {
		getRandomValues: crypto.randomFillSync,
	},
});

// Establish API mocking before all tests through MSW.
beforeAll(() =>
	server.listen({
		onUnhandledRequest: "warn",
	}),
);

// Reset any request handlers that we may add during the tests,
// so they don't affect other tests.
afterEach(() => {
	cleanup();
	server.resetHandlers();
	vi.resetAllMocks();
});

// Clean up after the tests are finished.
afterAll(() => server.close());

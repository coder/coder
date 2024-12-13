import "@testing-library/jest-dom";
import "jest-location-mock";
import crypto from "node:crypto";
import { cleanup } from "@testing-library/react";
import type { Region } from "api/typesGenerated";
import type { ProxyLatencyReport } from "contexts/useProxyLatency";
import { useMemo } from "react";
import { server } from "testHelpers/server";

// useProxyLatency does some http requests to determine latency.
// This would fail unit testing, or at least make it very slow with
// actual network requests. So just globally mock this hook.
jest.mock("contexts/useProxyLatency", () => ({
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

		return { proxyLatencies, refetch: jest.fn() };
	},
}));

global.scrollTo = jest.fn();

window.HTMLElement.prototype.scrollIntoView = jest.fn();
window.open = jest.fn();
navigator.sendBeacon = jest.fn();

global.ResizeObserver = require("resize-observer-polyfill");

// Polyfill the getRandomValues that is used on utils/random.ts
Object.defineProperty(global.self, "crypto", {
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
	jest.resetAllMocks();
});

// Clean up after the tests are finished.
afterAll(() => server.close());

// biome-ignore lint/complexity/noUselessEmptyExport: This is needed because we are compiling under `--isolatedModules`
export {};

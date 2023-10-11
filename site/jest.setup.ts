import "@testing-library/jest-dom";
import { cleanup } from "@testing-library/react";
import crypto from "crypto";
import { server } from "testHelpers/server";
import "jest-location-mock";
import { TextEncoder, TextDecoder } from "util";
import { Blob } from "buffer";
import jestFetchMock from "jest-fetch-mock";
import { ProxyLatencyReport } from "contexts/useProxyLatency";
import { Region } from "api/typesGenerated";
import { useMemo } from "react";

jestFetchMock.enableMocks();

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

global.TextEncoder = TextEncoder;
// eslint-disable-next-line @typescript-eslint/no-explicit-any -- Polyfill for jsdom
global.TextDecoder = TextDecoder as any;
// eslint-disable-next-line @typescript-eslint/no-explicit-any -- Polyfill for jsdom
global.Blob = Blob as any;
global.scrollTo = jest.fn();

// Polyfill the getRandomValues that is used on utils/random.ts
Object.defineProperty(global.self, "crypto", {
  value: {
    getRandomValues: function (buffer: Buffer) {
      return crypto.randomFillSync(buffer);
    },
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
  jest.clearAllMocks();
});

// Clean up after the tests are finished.
afterAll(() => server.close());

// This is needed because we are compiling under `--isolatedModules`
export {};

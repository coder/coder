import "testHelpers/localStorage";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import type { Region } from "api/typesGenerated";
import {
  MockPrimaryWorkspaceProxy,
  MockWorkspaceProxies,
  MockHealthyWildWorkspaceProxy,
  MockUnhealthyWildWorkspaceProxy,
} from "testHelpers/entities";
import {
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import {
  getPreferredProxy,
  ProxyProvider,
  saveUserSelectedProxy,
  useProxy,
} from "./ProxyContext";
import type * as ProxyLatency from "./useProxyLatency";

// Mock useProxyLatency to use a hard-coded latency. 'jest.mock' must be called
// here and not inside a unit test.
jest.mock("contexts/useProxyLatency", () => ({
  useProxyLatency: () => {
    return { proxyLatencies: hardCodedLatencies, refetch: jest.fn() };
  },
}));

let hardCodedLatencies: Record<string, ProxyLatency.ProxyLatencyReport> = {};

// fakeLatency is a helper function to make a Latency report from just a number.
const fakeLatency = (ms: number): ProxyLatency.ProxyLatencyReport => {
  return {
    latencyMS: ms,
    accurate: true,
    at: new Date(),
  };
};

describe("ProxyContextGetURLs", () => {
  it.each([
    ["empty", [], {}, undefined, "", ""],
    // Primary has no path app URL. Uses relative links
    [
      "primary",
      [MockPrimaryWorkspaceProxy],
      {},
      MockPrimaryWorkspaceProxy,
      "",
      MockPrimaryWorkspaceProxy.wildcard_hostname,
    ],
    [
      "regions selected",
      MockWorkspaceProxies,
      {},
      MockHealthyWildWorkspaceProxy,
      MockHealthyWildWorkspaceProxy.path_app_url,
      MockHealthyWildWorkspaceProxy.wildcard_hostname,
    ],
    // Primary is the default if none selected
    [
      "no selected",
      [MockPrimaryWorkspaceProxy],
      {},
      undefined,
      "",
      MockPrimaryWorkspaceProxy.wildcard_hostname,
    ],
    [
      "regions no select primary default",
      MockWorkspaceProxies,
      {},
      undefined,
      "",
      MockPrimaryWorkspaceProxy.wildcard_hostname,
    ],
    // Primary is the default if the selected is unhealthy
    [
      "unhealthy selection",
      MockWorkspaceProxies,
      {},
      MockUnhealthyWildWorkspaceProxy,
      "",
      MockPrimaryWorkspaceProxy.wildcard_hostname,
    ],
    // This should never happen, when there is no primary
    ["no primary", [MockHealthyWildWorkspaceProxy], {}, undefined, "", ""],
    // Latency behavior
    [
      "best latency",
      MockWorkspaceProxies,
      {
        [MockPrimaryWorkspaceProxy.id]: fakeLatency(100),
        [MockHealthyWildWorkspaceProxy.id]: fakeLatency(50),
        // This should be ignored because it's unhealthy
        [MockUnhealthyWildWorkspaceProxy.id]: fakeLatency(25),
        // This should be ignored because it is not in the list.
        ["not a proxy"]: fakeLatency(10),
      },
      undefined,
      MockHealthyWildWorkspaceProxy.path_app_url,
      MockHealthyWildWorkspaceProxy.wildcard_hostname,
    ],
  ])(
    `%p`,
    (
      _,
      regions,
      latencies,
      selected,
      preferredPathAppURL,
      preferredWildcardHostname,
    ) => {
      const preferred = getPreferredProxy(regions, selected, latencies);
      expect(preferred.preferredPathAppURL).toBe(preferredPathAppURL);
      expect(preferred.preferredWildcardHostname).toBe(
        preferredWildcardHostname,
      );
    },
  );
});

const TestingComponent = () => {
  return renderWithAuth(
    <ProxyProvider>
      <TestingScreen />
    </ProxyProvider>,
    {
      route: `/proxies`,
      path: "/proxies",
    },
  );
};

// TestingScreen just mounts some components that we can check in the unit test.
const TestingScreen = () => {
  const { proxy, userProxy, isFetched, isLoading, clearProxy, setProxy } =
    useProxy();
  return (
    <>
      <div data-testid="isFetched" title={isFetched.toString()}></div>
      <div data-testid="isLoading" title={isLoading.toString()}></div>
      <div
        data-testid="preferredProxy"
        title={proxy.proxy && proxy.proxy.id}
      ></div>
      <div data-testid="userProxy" title={userProxy && userProxy.id}></div>
      <button data-testid="clearProxy" onClick={clearProxy}></button>
      <div data-testid="userSelectProxyData"></div>
      <button
        data-testid="userSelectProxy"
        onClick={() => {
          const data = screen.getByTestId("userSelectProxyData");
          if (data.innerText) {
            setProxy(JSON.parse(data.innerText));
          }
        }}
      ></button>
    </>
  );
};

interface ProxyContextSelectionTest {
  // Regions is the list of regions to return via the "api" response.
  regions: Region[];
  // storageProxy should be the proxy stored in local storage before the
  // component is mounted and context is loaded. This simulates opening a
  // new window with a selection saved from before.
  storageProxy: Region | undefined;
  // latencies is the hard coded latencies to return. If empty, no latencies
  // are returned.
  latencies?: Record<string, ProxyLatency.ProxyLatencyReport>;
  // afterLoad are actions to take after loading the component, but before
  // assertions. This is useful for simulating user actions.
  afterLoad?: () => Promise<void>;

  // Assert these values.
  // expProxyID is the proxyID returned to be used.
  expProxyID: string;
  // expUserProxyID is the user's stored selection.
  expUserProxyID?: string;
}

describe("ProxyContextSelection", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  // A way to simulate a user clearing the proxy selection.
  const clearProxyAction = async (): Promise<void> => {
    const user = userEvent.setup();
    const clearProxyButton = screen.getByTestId("clearProxy");
    await user.click(clearProxyButton);
  };

  const userSelectProxy = (proxy: Region): (() => Promise<void>) => {
    return async (): Promise<void> => {
      const user = userEvent.setup();
      const selectData = screen.getByTestId("userSelectProxyData");
      selectData.innerText = JSON.stringify(proxy);

      const selectProxyButton = screen.getByTestId("userSelectProxy");
      await user.click(selectProxyButton);
    };
  };

  it.each([
    // Not latency behavior
    [
      "empty",
      {
        expProxyID: "",
        regions: [],
        storageProxy: undefined,
        latencies: {},
      },
    ],
    [
      "regions_no_selection",
      {
        expProxyID: MockPrimaryWorkspaceProxy.id,
        regions: MockWorkspaceProxies,
        storageProxy: undefined,
      },
    ],
    [
      "regions_selected_unhealthy",
      {
        expProxyID: MockPrimaryWorkspaceProxy.id,
        regions: MockWorkspaceProxies,
        storageProxy: MockUnhealthyWildWorkspaceProxy,
        expUserProxyID: MockUnhealthyWildWorkspaceProxy.id,
      },
    ],
    [
      "regions_selected_healthy",
      {
        expProxyID: MockHealthyWildWorkspaceProxy.id,
        regions: MockWorkspaceProxies,
        storageProxy: MockHealthyWildWorkspaceProxy,
        expUserProxyID: MockHealthyWildWorkspaceProxy.id,
      },
    ],
    [
      "regions_selected_clear",
      {
        expProxyID: MockPrimaryWorkspaceProxy.id,
        regions: MockWorkspaceProxies,
        storageProxy: MockHealthyWildWorkspaceProxy,
        afterLoad: clearProxyAction,
        expUserProxyID: undefined,
      },
    ],
    [
      "regions_make_selection",
      {
        expProxyID: MockHealthyWildWorkspaceProxy.id,
        regions: MockWorkspaceProxies,
        afterLoad: userSelectProxy(MockHealthyWildWorkspaceProxy),
        expUserProxyID: MockHealthyWildWorkspaceProxy.id,
      },
    ],
    // Latency behavior is disabled, so the primary should be selected.
    [
      "regions_default_low_latency",
      {
        expProxyID: MockPrimaryWorkspaceProxy.id,
        regions: MockWorkspaceProxies,
        storageProxy: undefined,
        latencies: {
          [MockPrimaryWorkspaceProxy.id]: fakeLatency(100),
          [MockHealthyWildWorkspaceProxy.id]: fakeLatency(50),
          // This is a trick. It's unhealthy so should be ignored.
          [MockUnhealthyWildWorkspaceProxy.id]: fakeLatency(25),
        },
      },
    ],
    [
      // User intentionally selected a high latency proxy.
      "regions_select_high_latency",
      {
        expProxyID: MockHealthyWildWorkspaceProxy.id,
        regions: MockWorkspaceProxies,
        storageProxy: undefined,
        afterLoad: userSelectProxy(MockHealthyWildWorkspaceProxy),
        expUserProxyID: MockHealthyWildWorkspaceProxy.id,
        latencies: {
          [MockHealthyWildWorkspaceProxy.id]: fakeLatency(500),
          [MockPrimaryWorkspaceProxy.id]: fakeLatency(100),
          // This is a trick. It's unhealthy so should be ignored.
          [MockUnhealthyWildWorkspaceProxy.id]: fakeLatency(25),
        },
      },
    ],
    [
      // Low latency proxy is selected, but it is unhealthy
      "regions_select_unhealthy_low_latency",
      {
        expProxyID: MockPrimaryWorkspaceProxy.id,
        regions: MockWorkspaceProxies,
        storageProxy: MockUnhealthyWildWorkspaceProxy,
        expUserProxyID: MockUnhealthyWildWorkspaceProxy.id,
        latencies: {
          [MockHealthyWildWorkspaceProxy.id]: fakeLatency(500),
          [MockPrimaryWorkspaceProxy.id]: fakeLatency(100),
          // This is a trick. It's unhealthy so should be ignored.
          [MockUnhealthyWildWorkspaceProxy.id]: fakeLatency(25),
        },
      },
    ],
    [
      // Excess proxies we do not have are low latency.
      // This will probably never happen in production.
      "unknown_regions_low_latency",
      {
        // Default to primary since we have unknowns
        expProxyID: MockPrimaryWorkspaceProxy.id,
        regions: MockWorkspaceProxies,
        storageProxy: MockUnhealthyWildWorkspaceProxy,
        expUserProxyID: MockUnhealthyWildWorkspaceProxy.id,
        latencies: {
          ["some"]: fakeLatency(500),
          ["random"]: fakeLatency(100),
          ["ids"]: fakeLatency(25),
        },
      },
    ],
  ] as [string, ProxyContextSelectionTest][])(
    `%s`,
    async (
      _,
      {
        expUserProxyID,
        expProxyID: expSelectedProxyID,
        regions,
        storageProxy,
        latencies = {},
        afterLoad,
      },
    ) => {
      // Mock the latencies
      hardCodedLatencies = latencies;

      // Initial selection if present
      if (storageProxy) {
        saveUserSelectedProxy(storageProxy);
      }

      // Mock the API response
      server.use(
        http.get("/api/v2/regions", () =>
          HttpResponse.json({
            regions,
          }),
        ),
        http.get("/api/v2/workspaceproxies", () =>
          HttpResponse.json({ regions }),
        ),
      );

      TestingComponent();
      await waitForLoaderToBeRemoved();

      if (afterLoad) {
        await afterLoad();
      }

      await screen.findByTestId("isFetched").then((x) => {
        expect(x.title).toBe("true");
      });
      await screen.findByTestId("isLoading").then((x) => {
        expect(x.title).toBe("false");
      });
      await screen.findByTestId("preferredProxy").then((x) => {
        expect(x.title).toBe(expSelectedProxyID);
      });
      await screen.findByTestId("userProxy").then((x) => {
        expect(x.title).toBe(expUserProxyID || "");
      });
    },
  );
});

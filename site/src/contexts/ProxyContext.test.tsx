import {
  MockPrimaryWorkspaceProxy,
  MockWorkspaceProxies,
  MockHealthyWildWorkspaceProxy,
  MockUnhealthyWildWorkspaceProxy,
} from "testHelpers/entities"
import {
  getPreferredProxy,
  ProxyProvider,
  saveUserSelectedProxy,
  useProxy,
} from "./ProxyContext"
import * as ProxyContextModule from "./ProxyContext"
import {
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers"
import { screen } from "@testing-library/react"
import { server } from "testHelpers/server"
import "testHelpers/localstorage"
import { rest } from "msw"
import { Region } from "api/typesGenerated"

describe("ProxyContextGetURLs", () => {
  it.each([
    ["empty", [], undefined, "", ""],
    // Primary has no path app URL. Uses relative links
    [
      "primary",
      [MockPrimaryWorkspaceProxy],
      MockPrimaryWorkspaceProxy,
      "",
      MockPrimaryWorkspaceProxy.wildcard_hostname,
    ],
    [
      "regions selected",
      MockWorkspaceProxies,
      MockHealthyWildWorkspaceProxy,
      MockHealthyWildWorkspaceProxy.path_app_url,
      MockHealthyWildWorkspaceProxy.wildcard_hostname,
    ],
    // Primary is the default if none selected
    [
      "no selected",
      [MockPrimaryWorkspaceProxy],
      undefined,
      "",
      MockPrimaryWorkspaceProxy.wildcard_hostname,
    ],
    [
      "regions no select primary default",
      MockWorkspaceProxies,
      undefined,
      "",
      MockPrimaryWorkspaceProxy.wildcard_hostname,
    ],
    // Primary is the default if the selected is unhealthy
    [
      "unhealthy selection",
      MockWorkspaceProxies,
      MockUnhealthyWildWorkspaceProxy,
      "",
      MockPrimaryWorkspaceProxy.wildcard_hostname,
    ],
    // This should never happen, when there is no primary
    ["no primary", [MockHealthyWildWorkspaceProxy], undefined, "", ""],
  ])(
    `%p`,
    (_, regions, selected, preferredPathAppURL, preferredWildcardHostname) => {
      const preferred = getPreferredProxy(regions, selected)
      expect(preferred.preferredPathAppURL).toBe(preferredPathAppURL)
      expect(preferred.preferredWildcardHostname).toBe(
        preferredWildcardHostname,
      )
    },
  )
})

// interface ProxySelectTest {
//   name: string
//   actions: ()
// }

const TestingComponent = () => {
  return renderWithAuth(
    <ProxyProvider>
      <TestingScreen />
    </ProxyProvider>,
    {
      route: `/proxies`,
      path: "/proxies",
    },
  )
}

// TestingScreen just mounts some components that we can check in the unit test.
const TestingScreen = () => {
  const { proxy, isFetched, isLoading } = useProxy()
  return (
    <>
      <div data-testid="isFetched" title={isFetched.toString()}></div>
      <div data-testid="isLoading" title={isLoading.toString()}></div>
      <div
        data-testid="preferredProxy"
        title={proxy.selectedProxy && proxy.selectedProxy.id}
      ></div>
    </>
  )
}

interface ProxyContextSelectionTest {
  expSelectedProxyID: string
  regions: Region[]
  storageProxy: Region | undefined
}

describe("ProxyContextSelection", () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it.each([
    [
      "empty",
      {
        expSelectedProxyID: "",
        regions: [],
        storageProxy: undefined,
      },
    ],
    [
      "regions_no_selection",
      {
        expSelectedProxyID: MockPrimaryWorkspaceProxy.id,
        regions: MockWorkspaceProxies,
        storageProxy: undefined,
      },
    ],
    [
      "regions_selected_unhealthy",
      {
        expSelectedProxyID: MockPrimaryWorkspaceProxy.id,
        regions: MockWorkspaceProxies,
        storageProxy: MockUnhealthyWildWorkspaceProxy,
      },
    ],
  ] as [string, ProxyContextSelectionTest][])(
    `%s`,
    async (_, { expSelectedProxyID, regions, storageProxy }) => {
      // Initial selection if present
      if (storageProxy) {
        saveUserSelectedProxy(storageProxy)
      }

      // Mock the API response
      server.use(
        rest.get("/api/v2/regions", async (req, res, ctx) => {
          return res(
            ctx.status(200),
            ctx.json({
              regions: regions,
            }),
          )
        }),
      )

      TestingComponent()
      await waitForLoaderToBeRemoved()

      await screen.findByTestId("isFetched").then((x) => {
        expect(x.title).toBe("true")
      })
      await screen.findByTestId("isLoading").then((x) => {
        expect(x.title).toBe("false")
      })
      await screen.findByTestId("preferredProxy").then((x) => {
        expect(x.title).toBe(expSelectedProxyID)
      })

      // const { proxy, proxies, isFetched, isLoading, proxyLatencies } = useProxy()
      // expect(isLoading).toBe(false)
      // expect(isFetched).toBe(true)

      // expect(x).toBe(2)
      // const preferred = getPreferredProxy(regions, selected)
      // expect(preferred.preferredPathAppURL).toBe(preferredPathAppURL)
      // expect(preferred.preferredWildcardHostname).toBe(
      //   preferredWildcardHostname,
      // )
    },
  )
})

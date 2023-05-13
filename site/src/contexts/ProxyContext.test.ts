import {
  MockPrimaryWorkspaceProxy,
  MockWorkspaceProxies,
  MockHealthyWildWorkspaceProxy,
  MockUnhealthyWildWorkspaceProxy,
} from "testHelpers/entities"
import { getPreferredProxy } from "./ProxyContext"

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

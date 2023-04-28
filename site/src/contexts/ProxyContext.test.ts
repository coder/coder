import {
  MockPrimaryRegion,
  MockRegions,
  MockHealthyWildRegion,
} from "testHelpers/entities"
import { getPreferredProxy } from "./ProxyContext"

describe("ProxyContextGetURLs", () => {
  it.each([
    ["empty", [], undefined, "", ""],
    // Primary has no path app URL. Uses relative links
    [
      "primary",
      [MockPrimaryRegion],
      MockPrimaryRegion,
      "",
      MockPrimaryRegion.wildcard_hostname,
    ],
    [
      "regions selected",
      MockRegions,
      MockHealthyWildRegion,
      MockHealthyWildRegion.path_app_url,
      MockHealthyWildRegion.wildcard_hostname,
    ],
    // Primary is the default if none selected
    [
      "no selected",
      [MockPrimaryRegion],
      undefined,
      "",
      MockPrimaryRegion.wildcard_hostname,
    ],
    [
      "regions no select primary default",
      MockRegions,
      undefined,
      "",
      MockPrimaryRegion.wildcard_hostname,
    ],
    // This should never happen, when there is no primary
    ["no primary", [MockHealthyWildRegion], undefined, "", ""],
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

import { Region } from "api/typesGenerated"
import { useDashboard } from "components/Dashboard/DashboardProvider"

/* 
 * PreferredProxy is stored in local storage. This contains the information
 * required to connect to a workspace via a proxy. 
*/
export interface PreferredProxy {
  // Regions is a list of all the regions returned by coderd.
  Regions: Region[]
  // SelectedRegion is the region the user has selected.
  SelectedRegion: Region
  // PreferredPathAppURL is the URL of the proxy or it is the empty string
  // to indicte using relative paths. To add a path to this:
  //  PreferredPathAppURL + "/path/to/app"
  PreferredPathAppURL: string
  // PreferredWildcardHostname is a hostname that includes a wildcard.
  // TODO: If the preferred proxy does not have this set, should we default to'
  //    the primary's?
  //  Example: "*.example.com"
  PreferredWildcardHostname: string
}

export const usePreferredProxy = (): PreferredProxy | undefined => {
  const dashboard = useDashboard()
  // Only use preferred proxy if the user has the moons experiment enabled
  if (!dashboard?.experiments.includes("moons")) {
    return undefined
  }

  const str = localStorage.getItem("preferred-proxy")
  if (str === undefined || str === null) {
    return undefined
  }
  const proxy = JSON.parse(str)
  if (proxy.id === undefined || proxy.id === null) {
    return undefined
  }
  return proxy
}

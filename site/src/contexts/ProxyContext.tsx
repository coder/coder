import { useQuery } from "@tanstack/react-query"
import { getApplicationsHost, getWorkspaceProxies } from "api/api"
import { Region } from "api/typesGenerated"
import { useDashboard } from "components/Dashboard/DashboardProvider"
import {
  createContext,
  FC,
  PropsWithChildren,
  useContext,
  useState,
} from "react"

interface ProxyContextValue {
  proxy: PreferredProxy
  isLoading: boolean
  error?: Error | unknown
  setProxy: (regions: Region[], selectedRegion: Region | undefined) => void
}

interface PreferredProxy {
  // SelectedRegion is the region the user has selected.
  // Do not use the fields 'path_app_url' or 'wildcard_hostname' from this
  // object. Use the preferred fields.
  selectedRegion: Region | undefined
  // PreferredPathAppURL is the URL of the proxy or it is the empty string
  // to indicate using relative paths. To add a path to this:
  //  PreferredPathAppURL + "/path/to/app"
  preferredPathAppURL: string
  // PreferredWildcardHostname is a hostname that includes a wildcard.
  preferredWildcardHostname: string
}

export const ProxyContext = createContext<ProxyContextValue | undefined>(
  undefined,
)

/**
 * ProxyProvider interacts with local storage to indicate the preferred workspace proxy.
 */
export const ProxyProvider: FC<PropsWithChildren> = ({ children }) => {
  // Try to load the preferred proxy from local storage.
  let savedProxy = loadPreferredProxy()
  if (!savedProxy) {
    savedProxy = getURLs([])
  }

  // The initial state is no regions and no selected region.
  const [proxy, setProxy] = useState<PreferredProxy>(savedProxy)
  const setAndSaveProxy = (
    regions: Region[],
    selectedRegion: Region | undefined,
  ) => {
    const preferred = getURLs(regions, selectedRegion)
    // Save to local storage to persist the user's preference across reloads
    // and other tabs.
    savePreferredProxy(preferred)
    // Set the state for the current context.
    setProxy(preferred)
  }

  const queryKey = ["get-regions"]
  const { error: regionsError, isLoading: regionsLoading } = useQuery({
    queryKey,
    queryFn: getWorkspaceProxies,
    // This onSuccess ensures the local storage is synchronized with the
    // regions returned by coderd. If the selected region is not in the list,
    // then the user selection is removed.
    onSuccess: (data) => {
      setAndSaveProxy(data.regions, proxy.selectedRegion)
    },
  })

  // ******************************* //
  // ** This code can be removed  **
  // ** when the experimental is  **
  // **       dropped             ** //
  const dashboard = useDashboard()
  const appHostQueryKey = ["get-application-host"]
  const {
    data: applicationHostResult,
    error: appHostError,
    isLoading: appHostLoading,
  } = useQuery({
    queryKey: appHostQueryKey,
    queryFn: getApplicationsHost,
  })

  // If the experiment is disabled, then make the setState do a noop.
  // This preserves an empty state, which is the default behavior.
  if (!dashboard?.experiments.includes("moons")) {
    const value = getURLs([])

    return (
      <ProxyContext.Provider
        value={{
          proxy: {
            ...value,
            preferredWildcardHostname:
              applicationHostResult?.host || value.preferredWildcardHostname,
          },
          isLoading: appHostLoading,
          error: appHostError,
          setProxy: () => {
            // Does a noop
          },
        }}
      >
        {children}
      </ProxyContext.Provider>
    )
  }
  // ******************************* //

  // TODO: @emyrk Should make an api call to /regions endpoint to update the
  // regions list.

  return (
    <ProxyContext.Provider
      value={{
        proxy: proxy,
        isLoading: regionsLoading,
        error: regionsError,
        // A function that takes the new regions and selected region and updates
        // the state with the appropriate urls.
        setProxy: setAndSaveProxy,
      }}
    >
      {children}
    </ProxyContext.Provider>
  )
}

export const useProxy = (): ProxyContextValue => {
  const context = useContext(ProxyContext)

  if (!context) {
    throw new Error("useProxy should be used inside of <ProxyProvider />")
  }

  return context
}

/**
 * getURLs is a helper function to calculate the urls to use for a given proxy configuration. By default, it is
 * assumed no proxy is configured and relative paths should be used.
 * Exported for testing.
 *
 * @param regions Is the list of regions returned by coderd. If this is empty, default behavior is used.
 * @param selectedRegion Is the region the user has selected. If this is undefined, default behavior is used.
 */
export const getURLs = (
  regions: Region[],
  selectedRegion?: Region,
): PreferredProxy => {
  // By default we set the path app to relative and disable wildcard hostnames.
  // We will set these values if we find a proxy we can use that supports them.
  let pathAppURL = ""
  let wildcardHostname = ""

  // If a region is selected, make sure it is in the list of regions. If it is not
  // we should default to the primary.
  selectedRegion = regions.find(
    (region) => selectedRegion && region.id === selectedRegion.id,
  )

  if (!selectedRegion) {
    // If no region is selected, default to the primary region.
    selectedRegion = regions.find((region) => region.name === "primary")
  }

  // Only use healthy regions.
  if (selectedRegion && selectedRegion.healthy) {
    // By default use relative links for the primary region.
    // This is the default, and we should not change it.
    if (selectedRegion.name !== "primary") {
      pathAppURL = selectedRegion.path_app_url
    }
    wildcardHostname = selectedRegion.wildcard_hostname
  }

  // TODO: @emyrk Should we notify the user if they had an unhealthy region selected?

  return {
    selectedRegion,
    // Trim trailing slashes to be consistent
    preferredPathAppURL: pathAppURL.replace(/\/$/, ""),
    preferredWildcardHostname: wildcardHostname,
  }
}

// Local storage functions

export const savePreferredProxy = (saved: PreferredProxy): void => {
  window.localStorage.setItem("preferred-proxy", JSON.stringify(saved))
}

export const loadPreferredProxy = (): PreferredProxy | undefined => {
  const str = localStorage.getItem("preferred-proxy")
  if (str === undefined || str === null) {
    return undefined
  }
  const proxy: PreferredProxy = JSON.parse(str)
  if (proxy.selectedRegion === undefined || proxy.selectedRegion === null) {
    return undefined
  }
  return proxy
}

export const clearPreferredProxy = (): void => {
  localStorage.removeItem("preferred-proxy")
}

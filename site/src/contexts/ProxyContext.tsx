import { useQuery } from "@tanstack/react-query"
import { getWorkspaceProxies } from "api/api"
import { Region } from "api/typesGenerated"
import { useDashboard } from "components/Dashboard/DashboardProvider"
import {
  createContext,
  FC,
  PropsWithChildren,
  useContext,
  useState,
} from "react"
import { ProxyLatencyReport, useProxyLatency } from "./useProxyLatency"

export interface ProxyContextValue {
  // proxy is **always** the workspace proxy that should be used.
  // The 'proxy.selectedProxy' field is the proxy being used and comes from either:
  //   1. The user manually selected this proxy. (saved to local storage)
  //   2. The default proxy auto selected because:
  //    a. The user has not selected a proxy.
  //    b. The user's selected proxy is not in the list of proxies.
  //    c. The user's selected proxy is not healthy.
  //   3. undefined if there are no proxies.
  //
  // The values 'proxy.preferredPathAppURL' and 'proxy.preferredWildcardHostname' can
  // always be used even if 'proxy.selectedProxy' is undefined. These values are sourced from
  // the 'selectedProxy', but default to relative paths if the 'selectedProxy' is undefined.
  proxy: PreferredProxy

  // proxies is the list of proxies returned by coderd. This is fetched async.
  // isFetched, isLoading, and error are used to track the state of the async call.
  proxies?: Region[]
  // isFetched is true when the 'proxies' api call is complete.
  isFetched: boolean
  isLoading: boolean
  error?: Error | unknown
  // proxyLatencies is a map of proxy id to latency report. If the proxyLatencies[proxy.id] is undefined
  // then the latency has not been fetched yet. Calculations happen async for each proxy in the list.
  // Refer to the returned report for a given proxy for more information.
  proxyLatencies: Record<string, ProxyLatencyReport>
  // setProxy is a function that sets the user's selected proxy. This function should
  // only be called if the user is manually selecting a proxy. This value is stored in local
  // storage and will persist across reloads and tabs.
  setProxy: (selectedProxy: Region) => void
  // clearProxy is a function that clears the user's selected proxy.
  // If no proxy is selected, then the default proxy will be used.
  clearProxy: () => void
}

interface PreferredProxy {
  // selectedProxy is the proxy the user has selected.
  // Do not use the fields 'path_app_url' or 'wildcard_hostname' from this
  // object. Use the preferred fields.
  selectedProxy: Region | undefined
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
  const savedProxy = loadUserSelectedProxy()
  // As the proxies are being loaded, default to using the saved proxy.
  // If the saved proxy is not valid when the async fetch happens, the
  // selectedProxy will be updated accordingly.
  let defaultPreferredProxy: PreferredProxy = {
    selectedProxy: savedProxy,
    preferredPathAppURL: savedProxy?.path_app_url.replace(/\/$/, "") || "",
    preferredWildcardHostname: savedProxy?.wildcard_hostname || "",
  }
  if (!savedProxy) {
    // If no preferred proxy is saved, then default to using relative paths
    // and no subdomain support until the proxies are properly loaded.
    // This is the same as a user not selecting any proxy.
    defaultPreferredProxy = getPreferredProxy([])
  }

  const [proxy, setProxy] = useState<PreferredProxy>(defaultPreferredProxy)

  const dashboard = useDashboard()
  const experimentEnabled = dashboard?.experiments.includes("moons")
  const queryKey = ["get-proxies"]
  const {
    data: proxiesResp,
    error: proxiesError,
    isLoading: proxiesLoading,
    isFetched: proxiesFetched,
  } = useQuery({
    queryKey,
    queryFn: getWorkspaceProxies,
    // This onSuccess ensures the local storage is synchronized with the
    // proxies returned by coderd. If the selected proxy is not in the list,
    // then the user selection is ignored.
    onSuccess: (resp) => {
      setAndSaveProxy(proxy.selectedProxy, resp.regions)
    },
  })

  // Every time we get a new proxiesResponse, update the latency check
  // to each workspace proxy.
  const proxyLatencies = useProxyLatency(proxiesResp)

  const setAndSaveProxy = (
    selectedProxy?: Region,
    // By default the proxies come from the api call above.
    // Allow the caller to override this if they have a more up
    // to date list of proxies.
    proxies: Region[] = proxiesResp?.regions || [],
  ) => {
    if (!proxies) {
      throw new Error(
        "proxies are not yet loaded, so selecting a proxy makes no sense. How did you get here?",
      )
    }

    if (selectedProxy) {
      // Save to local storage to persist the user's preference across reloads
      // and other tabs. We always save this, even if the selection is "bad".
      saveUserSelectedProxy(selectedProxy)
    }

    // The preferred proxy attempts to use the user's selection if it is valid.
    const preferred = getPreferredProxy(proxies, selectedProxy)
    // Set the state for the current context.
    setProxy(preferred)
  }

  const clearProxy = () => {
    // Clear the user's selection from local storage.
    clearUserSelectedProxy()
    // Set the state for the current context.
    // If we pass no values, then the default proxy will be used.
    setAndSaveProxy()
  }

  return (
    <ProxyContext.Provider
      value={{
        proxyLatencies: proxyLatencies,
        proxy: experimentEnabled
          ? proxy
          : {
              // If the experiment is disabled, then call 'getPreferredProxy' with the regions from
              // the api call. The default behavior is to use the `primary` proxy.
              ...getPreferredProxy(proxiesResp?.regions || []),
            },
        proxies: proxiesResp?.regions,
        isLoading: proxiesLoading,
        isFetched: proxiesFetched,
        error: proxiesError,
        // A function that takes the new proxies and selected proxy and updates
        // the state with the appropriate urls.
        setProxy: setAndSaveProxy,
        clearProxy: clearProxy,
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
 * @param proxies Is the list of proxies returned by coderd. If this is empty, default behavior is used.
 * @param selectedProxy Is the proxy the user has selected. If this is undefined, default behavior is used.
 */
export const getPreferredProxy = (
  proxies: Region[],
  selectedProxy?: Region,
): PreferredProxy => {
  // By default we set the path app to relative and disable wildcard hostnames.
  // We will set these values if we find a proxy we can use that supports them.
  let pathAppURL = ""
  let wildcardHostname = ""

  // If a proxy is selected, make sure it is in the list of proxies. If it is not
  // we should default to the primary.
  selectedProxy = proxies.find(
    (proxy) => selectedProxy && proxy.id === selectedProxy.id,
  )

  // If no proxy is selected, or the selected proxy is unhealthy default to the primary proxy.
  if (!selectedProxy || !selectedProxy.healthy) {
    selectedProxy = proxies.find((proxy) => proxy.name === "primary")
  }

  // Only use healthy proxies.
  if (selectedProxy && selectedProxy.healthy) {
    // By default use relative links for the primary proxy.
    // This is the default, and we should not change it.
    if (selectedProxy.name !== "primary") {
      pathAppURL = selectedProxy.path_app_url
    }
    wildcardHostname = selectedProxy.wildcard_hostname
  }

  // TODO: @emyrk Should we notify the user if they had an unhealthy proxy selected?

  return {
    selectedProxy: selectedProxy,
    // Trim trailing slashes to be consistent
    preferredPathAppURL: pathAppURL.replace(/\/$/, ""),
    preferredWildcardHostname: wildcardHostname,
  }
}

// Local storage functions

export const clearUserSelectedProxy = (): void => {
  window.localStorage.removeItem("user-selected-proxy")
}

export const saveUserSelectedProxy = (saved: Region): void => {
  window.localStorage.setItem("user-selected-proxy", JSON.stringify(saved))
}

export const loadUserSelectedProxy = (): Region | undefined => {
  const str = localStorage.getItem("user-selected-proxy")
  if (!str) {
    return undefined
  }

  return JSON.parse(str)
}

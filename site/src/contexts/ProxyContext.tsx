import { useQuery } from "@tanstack/react-query"
import { getWorkspaceProxies } from "api/api"
import { Region } from "api/typesGenerated"
import { useDashboard } from "components/Dashboard/DashboardProvider"
import PerformanceObserver from "@fastly/performance-observer-polyfill"
import {
  createContext,
  FC,
  PropsWithChildren,
  useContext,
  useEffect,
  useReducer,
  useState,
} from "react"
import axios from "axios"

interface ProxyContextValue {
  proxy: PreferredProxy
  proxies?: Region[]
  // proxyLatenciesMS are recorded in milliseconds.
  proxyLatenciesMS?: Record<string, number>
  // isfetched is true when the proxy api call is complete.
  isFetched: boolean
  // isLoading is true if the proxy is in the process of being fetched.
  isLoading: boolean
  error?: Error | unknown
  setProxy: (selectedProxy: Region) => void
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

interface ProxyLatencyAction {
  proxyID: string
  latencyMS: number
}

const proxyLatenciesReducer = (
  state: Record<string, number>,
  action: ProxyLatencyAction,
): Record<string, number> => {
  // Just overwrite any existing latency.
  state[action.proxyID] = action.latencyMS
  return state
}

/**
 * ProxyProvider interacts with local storage to indicate the preferred workspace proxy.
 */
export const ProxyProvider: FC<PropsWithChildren> = ({ children }) => {
  // Try to load the preferred proxy from local storage.
  let savedProxy = loadPreferredProxy()
  if (!savedProxy) {
    // If no preferred proxy is saved, then default to using relative paths
    // and no subdomain support until the proxies are properly loaded.
    // This is the same as a user not selecting any proxy.
    savedProxy = getPreferredProxy([])
  }

  const [proxy, setProxy] = useState<PreferredProxy>(savedProxy)
  const [proxyLatenciesMS, dispatchProxyLatenciesMS] = useReducer(
    proxyLatenciesReducer,
    {},
  )

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
    // then the user selection is removed.
    onSuccess: (resp) => {
      setAndSaveProxy(proxy.selectedProxy, resp.regions)
    },
  })

  // Everytime we get a new proxiesResponse, update the latency check
  // to each workspace proxy.
  useEffect(() => {
    if (!proxiesResp) {
      return
    }

    // proxyMap is a map of the proxy path_app_url to the proxy object.
    // This is for the observer to know which requests are important to
    // record.
    const proxyChecks2 = proxiesResp.regions.reduce((acc, proxy) => {
      if (!proxy.healthy) {
        return acc
      }

      const url = new URL("/latency-check", proxy.path_app_url)
      acc[url.toString()] = proxy
      return acc
    }, {} as Record<string, Region>)

    // Start a new performance observer to record of all the requests
    // to the proxies.
    const observer = new PerformanceObserver((list) => {
      list.getEntries().forEach((entry) => {
        if (entry.entryType !== "resource") {
          // We should never get these, but just in case.
          return
        }

        const check = proxyChecks2[entry.name]
        if (!check) {
          // This is not a proxy request.
          return
        }
        // These docs are super useful.
        // https://developer.mozilla.org/en-US/docs/Web/API/Performance_API/Resource_timing
        // dispatchProxyLatenciesMS({
        //   proxyID: check.id,
        //   latencyMS: entry.duration,
        // })

        console.log("performance observer entry", entry)
      })
      console.log("performance observer", list)
    })
    // The resource requests include xmlhttp requests.
    observer.observe({ entryTypes: ["resource"] })
    axios
      .get("https://dev.coder.com/healthz")
      .then((resp) => {
        console.log(resp)
      })
      .catch((err) => {
        console.log(err)
      })

    const proxyChecks = proxiesResp.regions.map((proxy) => {
      // TODO: Move to /derp/latency-check
      const url = new URL("/healthz", proxy.path_app_url)
      return axios
        .get(url.toString())
        .then((resp) => {
          return resp
        })
        .catch((err) => {
          return err
        })

      // Add a random query param to ensure the request is not cached.
      // url.searchParams.append("cache_bust", Math.random().toString())
    })

    Promise.all([proxyChecks])
      .then((resp) => {
        console.log(resp)
        console.log("done", observer.takeRecords())
        // observer.disconnect()
      })
      .catch((err) => {
        console.log(err)
        // observer.disconnect()
      })
      .finally(() => {
        console.log("finally", observer.takeRecords())
        // observer.disconnect()
      })
  }, [proxiesResp])

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
    const preferred = getPreferredProxy(proxies, selectedProxy)
    // Save to local storage to persist the user's preference across reloads
    // and other tabs.
    savePreferredProxy(preferred)
    // Set the state for the current context.
    setProxy(preferred)
  }

  return (
    <ProxyContext.Provider
      value={{
        proxyLatenciesMS: proxyLatenciesMS,
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

export const savePreferredProxy = (saved: PreferredProxy): void => {
  window.localStorage.setItem("preferred-proxy", JSON.stringify(saved))
}

const loadPreferredProxy = (): PreferredProxy | undefined => {
  const str = localStorage.getItem("preferred-proxy")
  if (!str) {
    return undefined
  }

  return JSON.parse(str)
}

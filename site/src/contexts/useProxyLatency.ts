import { Region, RegionsResponse } from "api/typesGenerated";
import { useEffect, useReducer } from "react";
import PerformanceObserver from "@fastly/performance-observer-polyfill"
import axios from "axios";


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

export const useProxyLatency = (proxies?: RegionsResponse): Record<string, number> => {
  const [proxyLatenciesMS, dispatchProxyLatenciesMS] = useReducer(
    proxyLatenciesReducer,
    {},
  );

  // Only run latency updates when the proxies change.
  useEffect(() => {
    if (!proxies) {
      return
    }

    // proxyMap is a map of the proxy path_app_url to the proxy object.
    // This is for the observer to know which requests are important to
    // record.
    const proxyChecks = proxies.regions.reduce((acc, proxy) => {
      if (!proxy.healthy) {
        return acc
      }

      const url = new URL("/latency-check", proxy.path_app_url)
      acc[url.toString()] = proxy
      return acc
    }, {} as Record<string, Region>)


    // dispatchProxyLatenciesMSGuarded will assign the latency to the proxy
    // via the reducer. But it will only do so if the performance entry is
    // a resource entry that we care about.
    const dispatchProxyLatenciesMSGuarded = (entry:PerformanceEntry):void => {
      if (entry.entryType !== "resource") {
        // We should never get these, but just in case.
        return
      }

      // The entry.name is the url of the request.
      const check = proxyChecks[entry.name]
      if (!check) {
        // This is not a proxy request.
        return
      }

        // These docs are super useful.
        // https://developer.mozilla.org/en-US/docs/Web/API/Performance_API/Resource_timing
        let latencyMS = 0
        if("requestStart" in entry && (entry as PerformanceResourceTiming).requestStart !== 0) {
          // This is the preferred logic to get the latency.
          const timingEntry = entry as PerformanceResourceTiming
          latencyMS = timingEntry.responseEnd - timingEntry.requestStart
        } else {
          // This is the total duration of the request and will be off by a good margin.
          // This is a fallback if the better timing is not available.
          console.log(`Using fallback latency calculation for "${entry.name}". Latency will be incorrect and larger then actual.`)
          latencyMS = entry.duration
        }
        dispatchProxyLatenciesMS({
          proxyID: check.id,
          latencyMS: latencyMS,
        })

      return
    }

    // Start a new performance observer to record of all the requests
    // to the proxies.
    const observer = new PerformanceObserver((list) => {
      // If we get entries via this callback, then dispatch the events to the latency reducer.
      list.getEntries().forEach((entry) => {
        dispatchProxyLatenciesMSGuarded(entry)
      })
    })

    // The resource requests include xmlhttp requests.
    observer.observe({ entryTypes: ["resource"] })

    const proxyRequests = proxies.regions.map((proxy) => {
      const url = new URL("/latency-check", proxy.path_app_url)
      return axios
        .get(url.toString(), {
          withCredentials: false,
          // Must add a custom header to make the request not a "simple request"
          // https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS#simple_requests
          headers: { "X-LATENCY-CHECK": "true" },
        })
    })

    // When all the proxy requests finish
    Promise.all(proxyRequests)
    // TODO: If there is an error on any request, we might want to store some indicator of that?
    .finally(() => {
      // takeRecords will return any entries that were not called via the callback yet.
      // We want to call this before we disconnect the observer to make sure we get all the
      // proxy requests recorded.
      observer.takeRecords().forEach((entry) => {
        dispatchProxyLatenciesMSGuarded(entry)
      })
      // At this point, we can be confident that all the proxy requests have been recorded
      // via the performance observer. So we can disconnect the observer.
      observer.disconnect()
    })
  }, [proxies])

  return proxyLatenciesMS
}

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

    // Start a new performance observer to record of all the requests
    // to the proxies.
    const observer = new PerformanceObserver((list) => {
      list.getEntries().forEach((entry) => {
        if (entry.entryType !== "resource") {
          // We should never get these, but just in case.
          return
        }

        console.log("performance observer entry", entry)
        const check = proxyChecks[entry.name]
        if (!check) {
          // This is not a proxy request.
          return
        }
        // These docs are super useful.
        // https://developer.mozilla.org/en-US/docs/Web/API/Performance_API/Resource_timing

        let latencyMS = 0
        if("requestStart" in entry && (entry as PerformanceResourceTiming).requestStart !== 0) {
          const timingEntry = entry as PerformanceResourceTiming
          latencyMS = timingEntry.responseEnd - timingEntry.requestStart
        } else {
          // This is the total duration of the request and will be off by a good margin.
          // This is a fallback if the better timing is not available.
          latencyMS = entry.duration
        }
        dispatchProxyLatenciesMS({
          proxyID: check.id,
          latencyMS: latencyMS,
        })

        // console.log("performance observer entry", entry)
      })
    })

    // The resource requests include xmlhttp requests.
    observer.observe({ entryTypes: ["resource"] })

    const proxyRequests = proxies.regions.map((proxy) => {
      // const url = new URL("/latency-check", proxy.path_app_url)
      const url = new URL("http://localhost:8081")
      return axios
        .get(url.toString(), {
          withCredentials: false,
          // Must add a custom header to make the request not a "simple request"
          // https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS#simple_requests
          headers: { "X-LATENCY-CHECK": "true" },
        })
    })

    Promise.all(proxyRequests)
    .finally(() => {
      console.log("finally outside", observer.takeRecords())
      observer.disconnect()
    })


  }, [proxies])

  return proxyLatenciesMS
}

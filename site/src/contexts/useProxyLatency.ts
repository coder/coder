import { Region, RegionsResponse } from "api/typesGenerated"
import { useEffect, useReducer } from "react"
import PerformanceObserver from "@fastly/performance-observer-polyfill"
import axios from "axios"
import { generateRandomString } from "utils/random"

export interface ProxyLatencyReport {
  // accurate identifies if the latency was calculated using the
  // PerformanceResourceTiming API. If this is false, then the
  // latency is calculated using the total duration of the request
  // and will be off by a good margin.
  accurate: boolean
  latencyMS: number
  // at is when the latency was recorded.
  at: Date
}

interface ProxyLatencyAction {
  proxyID: string
  report: ProxyLatencyReport
}

const proxyLatenciesReducer = (
  state: Record<string, ProxyLatencyReport>,
  action: ProxyLatencyAction,
): Record<string, ProxyLatencyReport> => {
  // Just overwrite any existing latency.
  return {
    ...state,
    [action.proxyID]: action.report,
  }
}

export const useProxyLatency = (
  proxies?: RegionsResponse,
): Record<string, ProxyLatencyReport> => {
  const [proxyLatencies, dispatchProxyLatencies] = useReducer(
    proxyLatenciesReducer,
    {},
  )

  // Only run latency updates when the proxies change.
  useEffect(() => {
    if (!proxies) {
      return
    }

    // proxyMap is a map of the proxy path_app_url to the proxy object.
    // This is for the observer to know which requests are important to
    // record.
    const proxyChecks = proxies.regions.reduce((acc, proxy) => {
      // Only run the latency check on healthy proxies.
      if (!proxy.healthy) {
        return acc
      }

      // Add a random query param to the url to make sure we don't get a cached response.
      // This is important in case there is some caching layer between us and the proxy.
      const url = new URL(
        `/latency-check?cache_bust=${generateRandomString(6)}`,
        proxy.path_app_url,
      )
      acc[url.toString()] = proxy
      return acc
    }, {} as Record<string, Region>)

    // dispatchProxyLatenciesGuarded will assign the latency to the proxy
    // via the reducer. But it will only do so if the performance entry is
    // a resource entry that we care about.
    const dispatchProxyLatenciesGuarded = (entry: PerformanceEntry): void => {
      if (entry.entryType !== "resource") {
        // We should never get these, but just in case.
        return
      }

      // The entry.name is the url of the request.
      const check = proxyChecks[entry.name]
      if (!check) {
        // This is not a proxy request, so ignore it.
        return
      }

      // These docs are super useful.
      // https://developer.mozilla.org/en-US/docs/Web/API/Performance_API/Resource_timing
      let latencyMS = 0
      let accurate = false
      if (
        "requestStart" in entry &&
        (entry as PerformanceResourceTiming).requestStart !== 0
      ) {
        // This is the preferred logic to get the latency.
        const timingEntry = entry as PerformanceResourceTiming
        latencyMS = timingEntry.responseStart - timingEntry.requestStart
        accurate = true
      } else {
        // This is the total duration of the request and will be off by a good margin.
        // This is a fallback if the better timing is not available.
        // eslint-disable-next-line no-console -- We can remove this when we display the "accurate" bool on the UI
        console.log(
          `Using fallback latency calculation for "${entry.name}". Latency will be incorrect and larger then actual.`,
        )
        latencyMS = entry.duration
      }
      dispatchProxyLatencies({
        proxyID: check.id,
        report: {
          latencyMS,
          accurate,
          at: new Date(),
        },
      })

      return
    }

    // Start a new performance observer to record of all the requests
    // to the proxies.
    const observer = new PerformanceObserver((list) => {
      // If we get entries via this callback, then dispatch the events to the latency reducer.
      list.getEntries().forEach((entry) => {
        dispatchProxyLatenciesGuarded(entry)
      })
    })

    // The resource requests include xmlhttp requests.
    observer.observe({ entryTypes: ["resource"] })

    const proxyRequests = Object.keys(proxyChecks).map((latencyURL) => {
      return axios.get(latencyURL, {
        withCredentials: false,
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
          dispatchProxyLatenciesGuarded(entry)
        })
        // At this point, we can be confident that all the proxy requests have been recorded
        // via the performance observer. So we can disconnect the observer.
        observer.disconnect()
      })
  }, [proxies])

  return proxyLatencies
}

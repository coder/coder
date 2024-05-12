import PerformanceObserver from "@fastly/performance-observer-polyfill";
import { useEffect, useReducer, useState } from "react";
import { API } from "api/api";
import type { Region } from "api/typesGenerated";
import { generateRandomString } from "utils/random";

const proxyIntervalSeconds = 30; // seconds

export interface ProxyLatencyReport {
  // accurate identifies if the latency was calculated using the
  // PerformanceResourceTiming API. If this is false, then the
  // latency is calculated using the total duration of the request
  // and will be off by a good margin.
  accurate: boolean;
  latencyMS: number;
  // at is when the latency was recorded.
  at: Date;
}

interface ProxyLatencyAction {
  proxyID: string;
  // cached indicates if the latency was loaded from a cache (local storage)
  cached: boolean;
  report: ProxyLatencyReport;
}

const proxyLatenciesReducer = (
  state: Record<string, ProxyLatencyReport>,
  action: ProxyLatencyAction,
): Record<string, ProxyLatencyReport> => {
  // Always return the new report. We have some saved latencies, but until we have a better
  // way to utilize them, we will ignore them for all practical purposes.
  return {
    ...state,
    [action.proxyID]: action.report,
  };
};

export const useProxyLatency = (
  proxies?: readonly Region[],
): {
  // Refetch can be called to refetch the proxy latencies.
  // Until the new values are loaded, the old values will still be used.
  refetch: () => Date;
  proxyLatencies: Record<string, ProxyLatencyReport>;
} => {
  // maxStoredLatencies is the maximum number of latencies to store per proxy in local storage.
  let maxStoredLatencies = 1;
  // The reason we pull this from local storage is so for development purposes, a user can manually
  // set a larger number to collect data in their normal usage. This data can later be analyzed to come up
  // with some better magic numbers.
  const maxStoredLatenciesVar = localStorage.getItem(
    "workspace-proxy-latencies-max",
  );
  if (maxStoredLatenciesVar) {
    maxStoredLatencies = Number(maxStoredLatenciesVar);
  }

  const [proxyLatencies, dispatchProxyLatencies] = useReducer(
    proxyLatenciesReducer,
    {},
  );

  // This latestFetchRequest is used to trigger a refetch of the proxy latencies.
  const [latestFetchRequest, setLatestFetchRequest] = useState(
    // The initial state is the current time minus the interval. Any proxies that have a latency after this
    // in the cache are still valid.
    new Date(new Date().getTime() - proxyIntervalSeconds * 1000).toISOString(),
  );

  // Refetch will always set the latestFetchRequest to the current time, making all the cached latencies
  // stale and triggering a refetch of all proxies in the list.
  const refetch = () => {
    const d = new Date();
    setLatestFetchRequest(d.toISOString());
    return d;
  };

  // Only run latency updates when the proxies change.
  useEffect(() => {
    if (!proxies) {
      return;
    }

    const storedLatencies = loadStoredLatencies();

    // proxyMap is a map of the proxy path_app_url to the proxy object.
    // This is for the observer to know which requests are important to
    // record.
    const proxyChecks = proxies.reduce(
      (acc, proxy) => {
        // Only run the latency check on healthy proxies.
        if (!proxy.healthy) {
          return acc;
        }

        // Do not run latency checks if a cached check exists below the latestFetchRequest Date.
        // This prevents fetching latencies too often.
        // 1. Fetch the latest stored latency for the given proxy.
        // 2. If the latest latency is after the latestFetchRequest, then skip the latency check.
        if (
          storedLatencies &&
          storedLatencies[proxy.id] &&
          storedLatencies[proxy.id].length > 0
        ) {
          const fetchRequestDate = new Date(latestFetchRequest);
          const latest = storedLatencies[proxy.id].reduce((prev, next) =>
            prev.at > next.at ? prev : next,
          );

          if (latest && latest.at > fetchRequestDate) {
            // dispatch the cached latency. This latency already went through the
            // guard logic below, so we can just dispatch it again directly.
            dispatchProxyLatencies({
              proxyID: proxy.id,
              cached: true,
              report: latest,
            });
            return acc;
          }
        }

        // Add a random query param to the url to make sure we don't get a cached response.
        // This is important in case there is some caching layer between us and the proxy.
        const url = new URL(
          `/latency-check?cache_bust=${generateRandomString(6)}`,
          proxy.path_app_url,
        );
        acc[url.toString()] = proxy;
        return acc;
      },
      {} as Record<string, Region>,
    );

    // dispatchProxyLatenciesGuarded will assign the latency to the proxy
    // via the reducer. But it will only do so if the performance entry is
    // a resource entry that we care about.
    const dispatchProxyLatenciesGuarded = (entry: PerformanceEntry) => {
      if (entry.entryType !== "resource") {
        // We should never get these, but just in case.
        return;
      }

      // The entry.name is the url of the request.
      const check = proxyChecks[entry.name];
      if (!check) {
        // This is not a proxy request, so ignore it.
        return;
      }

      // These docs are super useful.
      // https://developer.mozilla.org/en-US/docs/Web/API/Performance_API/Resource_timing
      let latencyMS = 0;
      let accurate = false;
      if (
        "requestStart" in entry &&
        (entry as PerformanceResourceTiming).requestStart !== 0
      ) {
        // This is the preferred logic to get the latency.
        const timingEntry = entry as PerformanceResourceTiming;
        latencyMS = timingEntry.responseStart - timingEntry.requestStart;
        accurate = true;
      } else {
        // This is the total duration of the request and will be off by a good margin.
        // This is a fallback if the better timing is not available.
        // eslint-disable-next-line no-console -- We can remove this when we display the "accurate" bool on the UI
        console.log(
          `Using fallback latency calculation for "${entry.name}". Latency will be incorrect and larger then actual.`,
        );
        latencyMS = entry.duration;
      }
      const update = {
        proxyID: check.id,
        cached: false,
        report: {
          latencyMS,
          accurate,
          at: new Date(),
        },
      };
      dispatchProxyLatencies(update);
      // Also save to local storage to persist the latency across page refreshes.
      updateStoredLatencies(update);

      return;
    };

    // Start a new performance observer to record of all the requests
    // to the proxies.
    const observer = new PerformanceObserver((list) => {
      // If we get entries via this callback, then dispatch the events to the latency reducer.
      list.getEntries().forEach((entry) => {
        dispatchProxyLatenciesGuarded(entry);
      });
    });

    // The resource requests include xmlhttp requests.
    observer.observe({ entryTypes: ["resource"] });

    const axiosInstance = API.getAxiosInstance();
    const proxyRequests = Object.keys(proxyChecks).map((latencyURL) => {
      return axiosInstance.get(latencyURL, {
        withCredentials: false,
        // Must add a custom header to make the request not a "simple request".
        // We want to force a preflight request.
        // https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS#simple_requests
        headers: { "X-LATENCY-CHECK": "true" },
      });
    });

    // When all the proxy requests finish
    void Promise.all(proxyRequests)
      // TODO: If there is an error on any request, we might want to store some indicator of that?
      .finally(() => {
        // takeRecords will return any entries that were not called via the callback yet.
        // We want to call this before we disconnect the observer to make sure we get all the
        // proxy requests recorded.
        observer.takeRecords().forEach((entry) => {
          dispatchProxyLatenciesGuarded(entry);
        });
        // At this point, we can be confident that all the proxy requests have been recorded
        // via the performance observer. So we can disconnect the observer.
        observer.disconnect();

        // Local storage cleanup
        garbageCollectStoredLatencies(proxies, maxStoredLatencies);
      });
  }, [proxies, latestFetchRequest, maxStoredLatencies]);

  return {
    proxyLatencies,
    refetch,
  };
};

// Local storage functions

// loadStoredLatencies will load the stored latencies from local storage.
// Latencies are stored in local storage to minimize the impact of outliers.
// If a single request is slow, we want to omit that latency check, and go with
// a more accurate latency check.
const loadStoredLatencies = (): Record<string, ProxyLatencyReport[]> => {
  const str = localStorage.getItem("workspace-proxy-latencies");
  if (!str) {
    return {};
  }

  return JSON.parse(str, (key, value) => {
    // By default json loads dates as strings. We want to convert them back to 'Date's.
    if (key === "at") {
      return new Date(value);
    }
    return value;
  });
};

const updateStoredLatencies = (action: ProxyLatencyAction): void => {
  const latencies = loadStoredLatencies();
  const reports = latencies[action.proxyID] || [];

  reports.push(action.report);
  latencies[action.proxyID] = reports;
  localStorage.setItem("workspace-proxy-latencies", JSON.stringify(latencies));
};

// garbageCollectStoredLatencies will remove any latencies that are older then 1 week or latencies of proxies
// that no longer exist. This is intended to keep the size of local storage down.
const garbageCollectStoredLatencies = (
  regions: readonly Region[],
  maxStored: number,
): void => {
  const latencies = loadStoredLatencies();
  const now = Date.now();
  const cleaned = cleanupLatencies(
    latencies,
    regions,
    new Date(now),
    maxStored,
  );

  localStorage.setItem("workspace-proxy-latencies", JSON.stringify(cleaned));
};

const cleanupLatencies = (
  stored: Record<string, ProxyLatencyReport[]>,
  regions: readonly Region[],
  now: Date,
  maxStored: number,
): Record<string, ProxyLatencyReport[]> => {
  Object.keys(stored).forEach((proxyID) => {
    if (!regions.find((region) => region.id === proxyID)) {
      delete stored[proxyID];
      return;
    }
    const reports = stored[proxyID];
    const nowMS = now.getTime();
    stored[proxyID] = reports.filter((report) => {
      // Only keep the reports that are less then 1 week old.
      return new Date(report.at).getTime() > nowMS - 1000 * 60 * 60 * 24 * 7;
    });
    // Only keep the 5 latest
    stored[proxyID] = stored[proxyID].slice(-1 * maxStored);
  });
  return stored;
};

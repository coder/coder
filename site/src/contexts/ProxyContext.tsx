import { API } from "api/api";
import { cachedQuery } from "api/queries/util";
import type { Region, WorkspaceProxy } from "api/typesGenerated";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import {
	type FC,
	type PropsWithChildren,
	createContext,
	useCallback,
	useContext,
	useEffect,
	useState,
} from "react";
import { useQuery } from "react-query";
import { type ProxyLatencyReport, useProxyLatency } from "./useProxyLatency";

export type Proxies = readonly Region[] | readonly WorkspaceProxy[];
export type ProxyLatencies = Record<string, ProxyLatencyReport>;
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
	proxy: PreferredProxy;

	// userProxy is always the proxy the user has selected. This value comes from local storage.
	// The value `proxy` should always be used instead of `userProxy`. `userProxy` is only exposed
	// so the caller can determine if the proxy being used is the user's selected proxy, or if it
	// was auto selected based on some other criteria.
	userProxy?: Region;

	// proxies is the list of proxies returned by coderd. This is fetched async.
	// isFetched, isLoading, and error are used to track the state of the async call.
	//
	// Region[] is returned if the user is a non-admin.
	// WorkspaceProxy[] is returned if the user is an admin. WorkspaceProxy extends Region with
	//  more information about the proxy and the status. More information includes the error message if
	//  the proxy is unhealthy.
	proxies?: Proxies;
	// isFetched is true when the 'proxies' api call is complete.
	isFetched: boolean;
	isLoading: boolean;
	error?: unknown;
	// proxyLatencies is a map of proxy id to latency report. If the proxyLatencies[proxy.id] is undefined
	// then the latency has not been fetched yet. Calculations happen async for each proxy in the list.
	// Refer to the returned report for a given proxy for more information.
	proxyLatencies: ProxyLatencies;
	// refetchProxyLatencies will trigger refreshing of the proxy latencies. By default the latencies
	// are loaded once.
	refetchProxyLatencies: () => Date;
	// setProxy is a function that sets the user's selected proxy. This function should
	// only be called if the user is manually selecting a proxy. This value is stored in local
	// storage and will persist across reloads and tabs.
	setProxy: (selectedProxy: Region) => void;
	// clearProxy is a function that clears the user's selected proxy.
	// If no proxy is selected, then the default proxy will be used.
	clearProxy: () => void;
}

interface PreferredProxy {
	// proxy is the proxy being used. It is provided for
	// getting the fields such as "display_name" and "id"
	// Do not use the fields 'path_app_url' or 'wildcard_hostname' from this
	// object. Use the preferred fields.
	proxy: Region | undefined;
	// PreferredPathAppURL is the URL of the proxy or it is the empty string
	// to indicate using relative paths. To add a path to this:
	//  PreferredPathAppURL + "/path/to/app"
	preferredPathAppURL: string;
	// PreferredWildcardHostname is a hostname that includes a wildcard.
	preferredWildcardHostname: string;
}

export const ProxyContext = createContext<ProxyContextValue | undefined>(
	undefined,
);

/**
 * ProxyProvider interacts with local storage to indicate the preferred workspace proxy.
 */
export const ProxyProvider: FC<PropsWithChildren> = ({ children }) => {
	// Using a useState so the caller always has the latest user saved
	// proxy.
	const [userSavedProxy, setUserSavedProxy] = useState(loadUserSelectedProxy());

	// Load the initial state from local storage.
	const [proxy, setProxy] = useState<PreferredProxy>(
		computeUsableURLS(userSavedProxy),
	);

	const { permissions } = useAuthenticated();
	const { metadata } = useEmbeddedMetadata();

	const {
		data: proxiesResp,
		error: proxiesError,
		isLoading: proxiesLoading,
		isFetched: proxiesFetched,
	} = useQuery(
		cachedQuery({
			metadata: metadata.regions,
			queryKey: ["get-proxies"],
			queryFn: async (): Promise<readonly Region[]> => {
				const apiCall = permissions.editWorkspaceProxies
					? API.getWorkspaceProxies
					: API.getWorkspaceProxyRegions;

				const resp = await apiCall();
				return resp.regions;
			},
		}),
	);

	// Every time we get a new proxiesResponse, update the latency check
	// to each workspace proxy.
	const { proxyLatencies, refetch: refetchProxyLatencies } =
		useProxyLatency(proxiesResp);

	// updateProxy is a helper function that when called will
	// update the proxy being used.
	const updateProxy = useCallback(() => {
		// Update the saved user proxy for the caller.
		setUserSavedProxy(loadUserSelectedProxy());
		setProxy(
			getPreferredProxy(
				proxiesResp ?? [],
				loadUserSelectedProxy(),
				proxyLatencies,
				// Do not auto select based on latencies, as inconsistent latencies can cause this
				// to behave poorly.
				false,
			),
		);
	}, [proxiesResp, proxyLatencies]);

	// This useEffect ensures the proxy to be used is updated whenever the state changes.
	// This includes proxies being loaded, latencies being calculated, and the user selecting a proxy.
	// biome-ignore lint/correctness/useExhaustiveDependencies: Only update if the source data changes
	useEffect(() => {
		updateProxy();
	}, [proxiesResp, proxyLatencies]);

	return (
		<ProxyContext.Provider
			value={{
				proxyLatencies,
				refetchProxyLatencies,
				userProxy: userSavedProxy,
				proxy: proxy,
				proxies: proxiesResp,
				isLoading: proxiesLoading,
				isFetched: proxiesFetched,
				error: proxiesError,

				// These functions are exposed to allow the user to select a proxy.
				setProxy: (proxy: Region) => {
					// Save to local storage to persist the user's preference across reloads
					saveUserSelectedProxy(proxy);
					// Update the selected proxy
					updateProxy();
				},
				clearProxy: () => {
					// Clear the user's selection from local storage.
					clearUserSelectedProxy();
					updateProxy();
				},
			}}
		>
			{children}
		</ProxyContext.Provider>
	);
};

export const useProxy = (): ProxyContextValue => {
	const context = useContext(ProxyContext);

	if (!context) {
		throw new Error("useProxy should be used inside of <ProxyProvider />");
	}

	return context;
};

/**
 * getPreferredProxy is a helper function to calculate the urls to use for a given proxy configuration. By default, it is
 * assumed no proxy is configured and relative paths should be used.
 * Exported for testing.
 *
 * @param proxies Is the list of proxies returned by coderd. If this is empty, default behavior is used.
 * @param selectedProxy Is the proxy saved in local storage. If this is undefined, default behavior is used.
 * @param latencies If provided, this is used to determine the best proxy to default to.
 *                  If not, `primary` is always the best default.
 */
export const getPreferredProxy = (
	proxies: readonly Region[],
	selectedProxy?: Region,
	latencies?: Record<string, ProxyLatencyReport>,
	autoSelectBasedOnLatency = true,
): PreferredProxy => {
	// If a proxy is selected, make sure it is in the list of proxies. If it is not
	// we should default to the primary.
	selectedProxy = proxies.find(
		(proxy) => selectedProxy && proxy.id === selectedProxy.id,
	);

	// If no proxy is selected, or the selected proxy is unhealthy default to the primary proxy.
	if (!selectedProxy || !selectedProxy.healthy) {
		// By default, use the primary proxy.
		selectedProxy = proxies.find((proxy) => proxy.name === "primary");

		// If we have latencies, then attempt to use the best proxy by latency instead.
		const best = selectByLatency(proxies, latencies);
		if (autoSelectBasedOnLatency && best) {
			selectedProxy = best;
		}
	}

	return computeUsableURLS(selectedProxy);
};

const selectByLatency = (
	proxies: readonly Region[],
	latencies?: Record<string, ProxyLatencyReport>,
): Region | undefined => {
	if (!latencies) {
		return undefined;
	}

	const proxyMap = Object.fromEntries(proxies.map((it) => [it.id, it]));
	const best = Object.entries(latencies)
		.map(([proxyId, latency]) => ({ id: proxyId, ...latency }))
		// If the proxy is not in our list, or it is unhealthy, ignore it.
		.filter((latency) => proxyMap[latency.id]?.healthy)
		.sort((a, b) => a.latencyMS - b.latencyMS)
		.at(0);

	// Found a new best, use it!
	if (best) {
		const bestProxy = proxies.find((proxy) => proxy.id === best.id);
		// Default to w/e it was before
		return bestProxy;
	}

	return undefined;
};

const computeUsableURLS = (proxy?: Region): PreferredProxy => {
	if (!proxy) {
		// By default use relative links for the primary proxy.
		// This is the default, and we should not change it.
		return {
			proxy: undefined,
			preferredPathAppURL: "",
			preferredWildcardHostname: "",
		};
	}

	let pathAppURL = proxy?.path_app_url.replace(/\/$/, "");
	// Primary proxy uses relative paths. It's the only exception.
	if (proxy.name === "primary") {
		pathAppURL = "";
	}

	return {
		proxy: proxy,
		// Trim trailing slashes to be consistent
		preferredPathAppURL: pathAppURL,
		preferredWildcardHostname: proxy.wildcard_hostname,
	};
};

// Local storage functions

export const clearUserSelectedProxy = (): void => {
	localStorage.removeItem("user-selected-proxy");
};

export const saveUserSelectedProxy = (saved: Region): void => {
	localStorage.setItem("user-selected-proxy", JSON.stringify(saved));
};

export const loadUserSelectedProxy = (): Region | undefined => {
	const str = localStorage.getItem("user-selected-proxy");
	if (!str) {
		return undefined;
	}

	return JSON.parse(str);
};

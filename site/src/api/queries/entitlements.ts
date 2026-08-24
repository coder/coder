import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type { Entitlements } from "#/api/typesGenerated";
import type { MetadataState } from "#/hooks/useEmbeddedMetadata";
import { cachedQuery } from "./util";

export const entitlementsQueryKey = ["entitlements"] as const;

export const entitlements = (metadata: MetadataState<Entitlements>) => {
	return {
		...cachedQuery({
			metadata,
			queryKey: entitlementsQueryKey,
			queryFn: () => API.getEntitlements(),
		}),
		// Entitlements carry operational warnings, such as usage publishing
		// health, that change while the dashboard stays open. The embedded
		// metadata cache never goes stale on its own, so refetch on window
		// focus and network reconnect ("always" bypasses the infinite
		// staleTime) to surface new warnings and clear resolved ones without
		// a full page reload. Event-driven on purpose; no polling interval.
		refetchOnWindowFocus: "always" as const,
		refetchOnReconnect: "always" as const,
	};
};

export const refreshEntitlements = (queryClient: QueryClient) => {
	return {
		mutationFn: API.refreshEntitlements,
		onSuccess: async () => {
			await queryClient.invalidateQueries({
				queryKey: entitlementsQueryKey,
			});
		},
	};
};

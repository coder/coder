import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import { entitlementsQueryKey } from "./entitlements";

export const licensesKey = ["licenses"] as const;

export const licenses = () => ({
	queryKey: licensesKey,
	queryFn: () => API.getLicenses(),
});

export const createTrialLicense = (queryClient: QueryClient) => ({
	mutationFn: API.createTrialLicense,
	onSuccess: async () => {
		await Promise.all([
			queryClient.invalidateQueries({ queryKey: entitlementsQueryKey }),
			queryClient.invalidateQueries({ queryKey: licensesKey }),
		]);
	},
});

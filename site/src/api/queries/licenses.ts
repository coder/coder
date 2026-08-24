import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import {
	deploymentCapabilitiesQueryKey,
	entitlementDetailsQueryKey,
} from "./entitlements";

export const licensesKey = ["licenses"] as const;

export const licenses = () => ({
	queryKey: licensesKey,
	queryFn: () => API.getLicenses(),
});

export const createTrialLicense = (queryClient: QueryClient) => ({
	mutationFn: API.createTrialLicense,
	onSuccess: async () => {
		await Promise.all([
			queryClient.invalidateQueries({
				queryKey: deploymentCapabilitiesQueryKey,
			}),
			queryClient.invalidateQueries({ queryKey: entitlementDetailsQueryKey }),
			queryClient.invalidateQueries({ queryKey: licensesKey }),
		]);
	},
});

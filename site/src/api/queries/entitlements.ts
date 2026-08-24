import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type {
	DeploymentCapabilities,
	Entitlements,
} from "#/api/typesGenerated";
import type { MetadataState } from "#/hooks/useEmbeddedMetadata";
import { cachedQuery } from "./util";

export const deploymentCapabilitiesQueryKey = [
	"deployment-capabilities",
] as const;
export const entitlementDetailsQueryKey = ["entitlements", "details"] as const;

export const deploymentCapabilities = (
	metadata: MetadataState<DeploymentCapabilities>,
) =>
	cachedQuery({
		metadata,
		queryKey: deploymentCapabilitiesQueryKey,
		queryFn: () => API.getDeploymentCapabilities(),
	});

export const entitlementDetails = () => ({
	queryKey: entitlementDetailsQueryKey,
	queryFn: (): Promise<Entitlements> => API.getEntitlements(),
});

export const refreshEntitlements = (queryClient: QueryClient) => ({
	mutationFn: API.refreshEntitlements,
	onSuccess: async () => {
		await Promise.all([
			queryClient.invalidateQueries({
				queryKey: deploymentCapabilitiesQueryKey,
			}),
			queryClient.invalidateQueries({ queryKey: entitlementDetailsQueryKey }),
		]);
	},
});

import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type {
	AIProvider,
	CreateAIProviderRequest,
	UpdateAIProviderRequest,
} from "#/api/typesGenerated";

const aiProvidersListKey = ["ai", "providers"] as const;

export const aiProviderKeyFor = (idOrName: string) =>
	[...aiProvidersListKey, idOrName] as const;

export const aiProvidersList = () => ({
	queryKey: aiProvidersListKey,
	queryFn: (): Promise<AIProvider[]> => API.getAIProviders(),
});

export const aiProvider = (idOrName: string) => ({
	queryKey: aiProviderKeyFor(idOrName),
	queryFn: (): Promise<AIProvider> => API.getAIProvider(idOrName),
});

export const createAIProviderMutation = (queryClient: QueryClient) => ({
	mutationFn: (request: CreateAIProviderRequest): Promise<AIProvider> =>
		API.createAIProvider(request),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: aiProvidersListKey });
	},
});

export const updateAIProviderMutation = (
	queryClient: QueryClient,
	idOrName: string,
) => ({
	mutationFn: (request: UpdateAIProviderRequest): Promise<AIProvider> =>
		API.updateAIProvider(idOrName, request),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: aiProvidersListKey });
		await queryClient.invalidateQueries({
			queryKey: aiProviderKeyFor(idOrName),
		});
	},
});

export const deleteAIProviderMutation = (
	queryClient: QueryClient,
	idOrName: string,
) => ({
	mutationFn: () => API.deleteAIProvider(idOrName),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: aiProvidersListKey });
		queryClient.removeQueries({ queryKey: aiProviderKeyFor(idOrName) });
	},
});

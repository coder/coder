import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import { invalidateChatProviderDependentQueries } from "#/api/queries/chats";
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
		await Promise.all([
			queryClient.invalidateQueries({ queryKey: aiProvidersListKey }),
			invalidateChatProviderDependentQueries(queryClient),
		]);
	},
});

export const updateAIProviderMutation = (
	queryClient: QueryClient,
	idOrName: string,
) => ({
	mutationFn: (request: UpdateAIProviderRequest): Promise<AIProvider> =>
		API.updateAIProvider(idOrName, request),
	onSuccess: async () => {
		await Promise.all([
			queryClient.invalidateQueries({ queryKey: aiProvidersListKey }),
			queryClient.invalidateQueries({
				queryKey: aiProviderKeyFor(idOrName),
			}),
			invalidateChatProviderDependentQueries(queryClient),
		]);
	},
});

export const deleteAIProviderMutation = (
	queryClient: QueryClient,
	idOrName: string,
) => ({
	mutationFn: () => API.deleteAIProvider(idOrName),
	onSuccess: async () => {
		queryClient.removeQueries({ queryKey: aiProviderKeyFor(idOrName) });
		await Promise.all([
			queryClient.invalidateQueries({ queryKey: aiProvidersListKey }),
			invalidateChatProviderDependentQueries(queryClient),
		]);
	},
});

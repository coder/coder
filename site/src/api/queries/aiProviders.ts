import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import { aiProvidersKey } from "#/api/queries/aiProviderKeys";
import { invalidateChatProviderDependentQueries } from "#/api/queries/chats";
import type {
	AIProvider,
	CreateAIProviderRequest,
	UpdateAIProviderRequest,
} from "#/api/typesGenerated";

export const aiProviderKeyFor = (idOrName: string) =>
	[...aiProvidersKey, idOrName] as const;

export const aiProvidersList = () => ({
	queryKey: aiProvidersKey,
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
			queryClient.invalidateQueries({ queryKey: aiProvidersKey }),
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
			queryClient.invalidateQueries({ queryKey: aiProvidersKey }),
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
			queryClient.invalidateQueries({ queryKey: aiProvidersKey }),
			invalidateChatProviderDependentQueries(queryClient),
		]);
	},
});

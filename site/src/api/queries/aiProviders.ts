import { type QueryClient, queryOptions } from "react-query";
import { API } from "#/api/api";
import { invalidateChatProviderDependentQueries } from "#/api/queries/chats";
import type {
	AIProvider,
	ChatProviderConfig,
	CreateAIProviderRequest,
	UpdateAIProviderRequest,
} from "#/api/typesGenerated";

export const aiProvidersListKey = ["ai", "providers"] as const;

const aiModelPricesKey = ["ai", "model-prices"] as const;

export const aiModelPrices = (provider: string, model: string) =>
	queryOptions({
		queryKey: [...aiModelPricesKey, provider, model] as const,
		queryFn: () => API.experimental.getAIModelPrices({ provider, model }),
	});

export const aiProviderKeyFor = (idOrName: string) =>
	[...aiProvidersListKey, idOrName] as const;

export const aiProvidersList = () => ({
	queryKey: aiProvidersListKey,
	queryFn: (): Promise<AIProvider[]> => API.getAIProviders(),
});

const selectChatProviderConfigs = (
	providers: readonly AIProvider[],
): ChatProviderConfig[] =>
	providers.map((provider) => ({
		id: provider.id,
		provider: provider.type,
		display_name: provider.display_name || provider.type,
		icon: provider.icon,
		enabled: provider.enabled,
		has_api_key: provider.api_keys.length > 0,
		central_api_key_enabled: true,
		allow_user_api_key: true,
		allow_central_api_key_fallback: true,
		base_url: provider.base_url,
		source: "database",
		created_at: provider.created_at,
		updated_at: provider.updated_at,
	}));

export const chatProviderConfigs = () =>
	queryOptions({
		queryKey: aiProvidersListKey,
		queryFn: (): Promise<AIProvider[]> => API.getAIProviders(),
		select: selectChatProviderConfigs,
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

import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type {
	AIProvider,
	CreateAIProviderRequest,
	UpdateAIProviderRequest,
} from "#/api/typesGenerated";

const aiProvidersListKey = ["ai", "providers"] as const;

const aiProviderKeyFor = (idOrName: string) =>
	[...aiProvidersListKey, idOrName] as const;

export const aiProvidersList = () => ({
	queryKey: aiProvidersListKey,
	queryFn: (): Promise<AIProvider[]> => API.getAIProviders(),
});

export const aiProvider = (idOrName: string) => ({
	queryKey: aiProviderKeyFor(idOrName),
	queryFn: (): Promise<AIProvider> => API.getAIProvider(idOrName),
});

/**
 * Create a new AI provider. For OpenAI/Anthropic, plaintext API keys travel
 * with the request body as `api_keys`. Bedrock providers carry their AWS
 * credentials inside `settings` and must leave `api_keys` empty.
 */
export const createAIProviderMutation = (queryClient: QueryClient) => ({
	mutationFn: (request: CreateAIProviderRequest): Promise<AIProvider> =>
		API.createAIProvider(request),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: aiProvidersListKey });
	},
});

/**
 * Update an AI provider. Key rotation happens atomically inside the PATCH via
 * the `api_keys` mutation list: a single `{ api_key: newPlaintext }` entry
 * implicitly deletes every existing key (none of their IDs are referenced)
 * and inserts the new plaintext. Omitting `api_keys` leaves the key set
 * untouched.
 */
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

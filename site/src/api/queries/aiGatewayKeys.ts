import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type {
	AIGatewayKey,
	CreateAIGatewayKeyRequest,
	CreateAIGatewayKeyResponse,
} from "#/api/typesGenerated";

const aiGatewayKeysListKey = ["ai", "gatewayKeys"] as const;

export const aiGatewayKeysList = () => ({
	queryKey: aiGatewayKeysListKey,
	queryFn: (): Promise<AIGatewayKey[]> => API.getAIGatewayKeys(),
});

export const createAIGatewayKeyMutation = (queryClient: QueryClient) => ({
	mutationFn: (
		request: CreateAIGatewayKeyRequest,
	): Promise<CreateAIGatewayKeyResponse> => API.createAIGatewayKey(request),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: aiGatewayKeysListKey });
	},
});

export const deleteAIGatewayKeyMutation = (queryClient: QueryClient) => ({
	mutationFn: (id: string): Promise<void> => API.deleteAIGatewayKey(id),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: aiGatewayKeysListKey });
	},
});

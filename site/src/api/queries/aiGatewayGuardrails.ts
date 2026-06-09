import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type {
	AIGatewayGuardrail,
	AIGatewayGuardrailVersion,
	CreateAIGatewayGuardrailRequest,
	CreateAIGatewayGuardrailVersionRequest,
	UpdateAIGatewayGuardrailRequest,
} from "#/api/typesGenerated";

const guardrailsListKey = ["ai", "gateway", "guardrails"] as const;

export const aiGatewayGuardrailsList = () => ({
	queryKey: guardrailsListKey,
	queryFn: (): Promise<AIGatewayGuardrail[]> => API.getAIGatewayGuardrails(),
});

export const aiGatewayGuardrail = (id: string) => ({
	queryKey: [...guardrailsListKey, id] as const,
	queryFn: (): Promise<AIGatewayGuardrail> => API.getAIGatewayGuardrail(id),
});

export const createAIGatewayGuardrailMutation = (queryClient: QueryClient) => ({
	mutationFn: (
		request: CreateAIGatewayGuardrailRequest,
	): Promise<AIGatewayGuardrail> => API.createAIGatewayGuardrail(request),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: guardrailsListKey });
	},
});

export const createAIGatewayGuardrailVersionMutation = (
	queryClient: QueryClient,
) => ({
	mutationFn: ({
		id,
		request,
	}: {
		id: string;
		request: CreateAIGatewayGuardrailVersionRequest;
	}): Promise<AIGatewayGuardrailVersion> =>
		API.createAIGatewayGuardrailVersion(id, request),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: guardrailsListKey });
	},
});

export const updateAIGatewayGuardrailMutation = (queryClient: QueryClient) => ({
	mutationFn: ({
		id,
		request,
	}: {
		id: string;
		request: UpdateAIGatewayGuardrailRequest;
	}): Promise<AIGatewayGuardrail> => API.updateAIGatewayGuardrail(id, request),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: guardrailsListKey });
	},
});

export const deleteAIGatewayGuardrailMutation = (queryClient: QueryClient) => ({
	mutationFn: (id: string): Promise<void> => API.deleteAIGatewayGuardrail(id),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: guardrailsListKey });
	},
});

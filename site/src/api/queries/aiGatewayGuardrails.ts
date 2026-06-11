import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type {
	AIGatewayGuardrail,
	AIGatewayGuardrailVersion,
	CreateAIGatewayGuardrailRequest,
	CreateAIGatewayGuardrailVersionRequest,
	UpdateAIGatewayGuardrailRequest,
} from "#/api/typesGenerated";

// Shared prefix for every AI gateway query. Activating a guardrail version
// mints new pipeline versions, so that mutation invalidates this prefix to
// refresh guardrails and pipelines (drift) together.
const aiGatewayKey = ["ai", "gateway"] as const;
const guardrailsListKey = ["ai", "gateway", "guardrails"] as const;

export const aiGatewayGuardrailsList = () => ({
	queryKey: guardrailsListKey,
	queryFn: (): Promise<AIGatewayGuardrail[]> => API.getAIGatewayGuardrails(),
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
		// Activating mints pipeline versions; refresh pipelines + drift too.
		await queryClient.invalidateQueries({ queryKey: aiGatewayKey });
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
		// Activating a guardrail version mints pipeline versions; refresh both.
		await queryClient.invalidateQueries({ queryKey: aiGatewayKey });
	},
});

export const deleteAIGatewayGuardrailMutation = (queryClient: QueryClient) => ({
	mutationFn: (id: string): Promise<void> => API.deleteAIGatewayGuardrail(id),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: guardrailsListKey });
	},
});

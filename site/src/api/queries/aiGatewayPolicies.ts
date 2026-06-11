import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type {
	AIGatewayPipeline,
	AIGatewayPipelineVersion,
	AIGatewayPolicy,
	AIGatewayPolicyVersion,
	CreateAIGatewayPipelineRequest,
	CreateAIGatewayPipelineVersionRequest,
	CreateAIGatewayPolicyRequest,
	CreateAIGatewayPolicyVersionRequest,
	UpdateAIGatewayPipelineMemberRequest,
	UpdateAIGatewayPipelineRequest,
	UpdateAIGatewayPolicyRequest,
} from "#/api/typesGenerated";

// Shared prefix for every AI gateway query. Activating a policy or guardrail
// mints new pipeline versions, so those mutations invalidate this prefix to
// refresh policies, pipelines (drift), and pipeline version history together.
const aiGatewayKey = ["ai", "gateway"] as const;
const policiesListKey = ["ai", "gateway", "policies"] as const;
const pipelinesListKey = ["ai", "gateway", "pipelines"] as const;

export const aiGatewayPoliciesList = () => ({
	queryKey: policiesListKey,
	queryFn: (): Promise<AIGatewayPolicy[]> => API.getAIGatewayPolicies(),
});

export const aiGatewayPolicy = (id: string) => ({
	queryKey: [...policiesListKey, id] as const,
	queryFn: (): Promise<AIGatewayPolicy> => API.getAIGatewayPolicy(id),
});

export const createAIGatewayPolicyVersionMutation = (
	queryClient: QueryClient,
) => ({
	mutationFn: ({
		id,
		request,
	}: {
		id: string;
		request: CreateAIGatewayPolicyVersionRequest;
	}): Promise<AIGatewayPolicyVersion> =>
		API.createAIGatewayPolicyVersion(id, request),
	onSuccess: async () => {
		// Activating mints pipeline versions; refresh pipelines + drift too.
		await queryClient.invalidateQueries({ queryKey: aiGatewayKey });
	},
});

export const aiGatewayPipelinesList = () => ({
	queryKey: pipelinesListKey,
	queryFn: (): Promise<AIGatewayPipeline[]> => API.getAIGatewayPipelines(),
});

export const aiGatewayPipelineVersions = (id: string) => ({
	queryKey: [...pipelinesListKey, id, "versions"] as const,
	queryFn: (): Promise<AIGatewayPipelineVersion[]> =>
		API.getAIGatewayPipelineVersions(id),
});

export const createAIGatewayPolicyMutation = (queryClient: QueryClient) => ({
	mutationFn: (
		request: CreateAIGatewayPolicyRequest,
	): Promise<AIGatewayPolicy> => API.createAIGatewayPolicy(request),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: policiesListKey });
	},
});

export const updateAIGatewayPolicyMutation = (queryClient: QueryClient) => ({
	mutationFn: ({
		id,
		request,
	}: {
		id: string;
		request: UpdateAIGatewayPolicyRequest;
	}): Promise<AIGatewayPolicy> => API.updateAIGatewayPolicy(id, request),
	onSuccess: async () => {
		// Activating/reverting mints pipeline versions; refresh pipelines too.
		await queryClient.invalidateQueries({ queryKey: aiGatewayKey });
	},
});

export const deleteAIGatewayPolicyMutation = (queryClient: QueryClient) => ({
	mutationFn: (id: string): Promise<void> => API.deleteAIGatewayPolicy(id),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: policiesListKey });
	},
});

export const createAIGatewayPipelineMutation = (queryClient: QueryClient) => ({
	mutationFn: (
		request: CreateAIGatewayPipelineRequest,
	): Promise<AIGatewayPipeline> => API.createAIGatewayPipeline(request),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: pipelinesListKey });
	},
});

export const createAIGatewayPipelineVersionMutation = (
	queryClient: QueryClient,
) => ({
	mutationFn: ({
		id,
		request,
	}: {
		id: string;
		request: CreateAIGatewayPipelineVersionRequest;
	}): Promise<AIGatewayPipelineVersion> =>
		API.createAIGatewayPipelineVersion(id, request),
	onSuccess: async () => {
		// Also refresh the per-pipeline version history sub-query.
		await queryClient.invalidateQueries({ queryKey: pipelinesListKey });
	},
});

export const updateAIGatewayPipelineMutation = (queryClient: QueryClient) => ({
	mutationFn: ({
		id,
		request,
	}: {
		id: string;
		request: UpdateAIGatewayPipelineRequest;
	}): Promise<AIGatewayPipeline> => API.updateAIGatewayPipeline(id, request),
	onSuccess: async () => {
		// Promoting clears drift and changes version history; refresh both.
		await queryClient.invalidateQueries({ queryKey: pipelinesListKey });
	},
});

export const deleteAIGatewayPipelineMutation = (queryClient: QueryClient) => ({
	mutationFn: (id: string): Promise<void> => API.deleteAIGatewayPipeline(id),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: pipelinesListKey });
	},
});

export const updateAIGatewayPipelineMemberMutation = (
	queryClient: QueryClient,
) => ({
	mutationFn: ({
		id,
		request,
	}: {
		id: string;
		request: UpdateAIGatewayPipelineMemberRequest;
	}): Promise<AIGatewayPipeline> =>
		API.updateAIGatewayPipelineMember(id, request),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: pipelinesListKey });
	},
});

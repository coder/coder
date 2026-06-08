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
	UpdateAIGatewayPipelineRequest,
	UpdateAIGatewayPolicyRequest,
} from "#/api/typesGenerated";

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
		await queryClient.invalidateQueries({ queryKey: policiesListKey });
	},
});

export const aiGatewayPipelinesList = () => ({
	queryKey: pipelinesListKey,
	queryFn: (): Promise<AIGatewayPipeline[]> => API.getAIGatewayPipelines(),
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
		await queryClient.invalidateQueries({ queryKey: policiesListKey });
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
		await queryClient.invalidateQueries({ queryKey: pipelinesListKey });
	},
});

export const deleteAIGatewayPipelineMutation = (queryClient: QueryClient) => ({
	mutationFn: (id: string): Promise<void> => API.deleteAIGatewayPipeline(id),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: pipelinesListKey });
	},
});

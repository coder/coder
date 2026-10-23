import { mutationOptions, type QueryClient } from "react-query";
import { API } from "#/api/api";
import type {
	CreateNetworkPolicyRequest,
	CreateNetworkPolicyRevisionRequest,
	CreateNetworkPolicyTargetRequest,
	DeleteNetworkPolicyRequest,
	PublishNetworkPolicyRequest,
	SetNetworkPolicyTemplatesRequest,
} from "#/api/typesGenerated";

/** All policy caches are isolated by organization. */
export const networkPoliciesKey = (organization: string) =>
	["organizations", organization, "network-policies"] as const;

/** The detail cache includes immutable versions, targets, and run status. */
export const networkPolicyKey = (organization: string, policyId: string) =>
	[...networkPoliciesKey(organization), policyId] as const;

/** Template eligibility is independent of policy attachment changes. */
const networkPolicyTemplatesKey = (organization: string) =>
	["organizations", organization, "network-policy-templates"] as const;

/** Persisted output is scoped to the policy as well as the individual run. */
export const networkPolicyRunLogsKey = (
	organization: string,
	policyId: string,
	runId: string,
) =>
	[...networkPolicyKey(organization, policyId), "runs", runId, "logs"] as const;

/** Polls persisted output while the run log viewer is mounted. */
export const networkPolicyRunLogs = (
	organization: string,
	policyId: string,
	runId: string,
) => ({
	queryKey: networkPolicyRunLogsKey(organization, policyId, runId),
	queryFn: () =>
		API.experimental.getNetworkPolicyRunLogs(organization, policyId, runId),
	refetchInterval: 5000,
});

/** The server returns only templates the caller can administer. */
export const networkPolicyTemplates = (organization: string) => ({
	queryKey: networkPolicyTemplatesKey(organization),
	queryFn: () => API.experimental.getNetworkPolicyTemplates(organization),
});

/** Reads the organization's policy summaries without synthesizing local state. */
export const networkPolicies = (organization: string) => ({
	queryKey: networkPoliciesKey(organization),
	queryFn: () => API.experimental.getNetworkPolicies(organization),
	refetchInterval: 5000,
});

/** Polling observes asynchronous provisioner completion. */
export const networkPolicy = (organization: string, policyId: string) => ({
	queryKey: networkPolicyKey(organization, policyId),
	queryFn: () => API.experimental.getNetworkPolicy(organization, policyId),
	refetchInterval: 5000,
});

const invalidateNetworkPolicy = (
	queryClient: QueryClient,
	organization: string,
	policyId: string,
) =>
	Promise.all([
		queryClient.invalidateQueries({
			queryKey: networkPoliciesKey(organization),
			exact: true,
		}),
		queryClient.invalidateQueries({
			queryKey: networkPolicyKey(organization, policyId),
			exact: true,
		}),
	]);

/** Refreshes the list even when a failed response may have followed a committed write. */
export const createNetworkPolicy = (
	queryClient: QueryClient,
	organization: string,
) =>
	mutationOptions({
		mutationFn: (request: CreateNetworkPolicyRequest) =>
			API.experimental.createNetworkPolicy(organization, request),
		onSettled: () =>
			queryClient.invalidateQueries({
				queryKey: networkPoliciesKey(organization),
				exact: true,
			}),
	});

/** Keeps cleanup progress and conflicts visible in both policy caches. */
export const deleteNetworkPolicy = (
	queryClient: QueryClient,
	organization: string,
	policyId: string,
) =>
	mutationOptions({
		mutationFn: (request: DeleteNetworkPolicyRequest) =>
			API.experimental.deleteNetworkPolicy(organization, policyId, request),
		onSettled: () =>
			invalidateNetworkPolicy(queryClient, organization, policyId),
	});

/** Saves an immutable version and refreshes generation tokens after conflicts. */
export const createNetworkPolicyRevision = (
	queryClient: QueryClient,
	organization: string,
	policyId: string,
) =>
	mutationOptions({
		mutationFn: (request: CreateNetworkPolicyRevisionRequest) =>
			API.experimental.createNetworkPolicyRevision(
				organization,
				policyId,
				request,
			),
		onSettled: () =>
			invalidateNetworkPolicy(queryClient, organization, policyId),
	});

/** Live attachment updates may queue target runs as part of the same server write. */
export const setNetworkPolicyTemplates = (
	queryClient: QueryClient,
	organization: string,
	policyId: string,
) =>
	mutationOptions({
		mutationFn: (request: SetNetworkPolicyTemplatesRequest) =>
			API.experimental.setNetworkPolicyTemplates(
				organization,
				policyId,
				request,
			),
		onSettled: () =>
			invalidateNetworkPolicy(queryClient, organization, policyId),
	});

/** Registers infrastructure coordinates without optimistically marking them applied. */
export const createNetworkPolicyTarget = (
	queryClient: QueryClient,
	organization: string,
	policyId: string,
) =>
	mutationOptions({
		mutationFn: (request: CreateNetworkPolicyTargetRequest) =>
			API.experimental.createNetworkPolicyTarget(
				organization,
				policyId,
				request,
			),
		onSettled: () =>
			invalidateNetworkPolicy(queryClient, organization, policyId),
	});

/** Queue acceptance is not apply success; subsequent reads supply target status. */
export const publishNetworkPolicy = (
	queryClient: QueryClient,
	organization: string,
	policyId: string,
) =>
	mutationOptions({
		mutationFn: (request: PublishNetworkPolicyRequest) =>
			API.experimental.publishNetworkPolicy(organization, policyId, request),
		onSettled: () =>
			invalidateNetworkPolicy(queryClient, organization, policyId),
	});

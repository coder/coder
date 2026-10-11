import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import { permittedOrganizations } from "./organizations";

const agentHoursKey = ["agentHours"] as const;

const agentHoursOrganizationAllotmentsKey = [
	...agentHoursKey,
	"organizationAllotments",
] as const;

const agentHoursUsageKey = [...agentHoursKey, "usage"] as const;

const agentHoursGroupAllotmentsKey = (organizationId: string) =>
	[...agentHoursKey, "groupAllotments", organizationId] as const;

const agentHoursOrganizationUsageKey = (organizationId: string) =>
	[...agentHoursKey, "organizationUsage", organizationId] as const;

// Other admins and other tabs change allotments and delete their targets, so
// these refetch on focus despite the global default.
export const agentHoursOrganizationAllotments = () => ({
	queryKey: agentHoursOrganizationAllotmentsKey,
	queryFn: API.getAgentHoursOrganizationAllotments,
	refetchOnWindowFocus: true,
});

export const agentHoursGroupAllotments = (organizationId: string) => ({
	queryKey: agentHoursGroupAllotmentsKey(organizationId),
	queryFn: () => API.getAgentHoursGroupAllotments(organizationId),
	refetchOnWindowFocus: true,
});

export const agentHoursUsage = () => ({
	queryKey: agentHoursUsageKey,
	queryFn: API.getAgentHoursUsage,
});

export const organizationAgentHoursUsage = (organizationId: string) => ({
	queryKey: agentHoursOrganizationUsageKey(organizationId),
	queryFn: () => API.getOrganizationAgentHoursUsage(organizationId),
});

type OrganizationAllotmentChange = {
	organizationId: string;
	allotmentBps: number;
};

// Group views derive their hours from their organization's share, and a 409
// means another admin's change made cached totals stale, so every write,
// including a failed one, refreshes all Agent Hours queries.
const invalidateAgentHours = (queryClient: QueryClient) =>
	queryClient.invalidateQueries({ queryKey: agentHoursKey });

export const upsertAgentHoursOrganizationAllotment = (
	queryClient: QueryClient,
) => ({
	mutationFn: ({ organizationId, allotmentBps }: OrganizationAllotmentChange) =>
		API.upsertAgentHoursOrganizationAllotment(organizationId, {
			allotment_bps: allotmentBps,
		}),
	onSettled: () => invalidateAgentHours(queryClient),
});

export const deleteAgentHoursOrganizationAllotment = (
	queryClient: QueryClient,
) => ({
	mutationFn: (organizationId: string) =>
		API.deleteAgentHoursOrganizationAllotment(organizationId),
	onSettled: () => invalidateAgentHours(queryClient),
});

type GroupAllotmentChange = {
	groupId: string;
	allotmentBps: number;
};

export const upsertAgentHoursGroupAllotment = (queryClient: QueryClient) => ({
	mutationFn: ({ groupId, allotmentBps }: GroupAllotmentChange) =>
		API.upsertAgentHoursGroupAllotment(groupId, {
			allotment_bps: allotmentBps,
		}),
	onSettled: () => invalidateAgentHours(queryClient),
});

export const deleteAgentHoursGroupAllotment = (queryClient: QueryClient) => ({
	mutationFn: (groupId: string) => API.deleteAgentHoursGroupAllotment(groupId),
	onSettled: () => invalidateAgentHours(queryClient),
});

// Group allotment writes authorize on updating groups, so this lists the
// organizations whose group allotments the user can manage.
export const agentHoursAllotmentOrganizations = () =>
	permittedOrganizations({
		object: { resource_type: "group" },
		action: "update",
	});

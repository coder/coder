import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import { permittedOrganizations } from "./organizations";

const agentHoursKey = ["agentHours"] as const;

const agentHoursOrganizationAllotmentsKey = [
	...agentHoursKey,
	"organizationAllotments",
] as const;

const agentHoursGroupAllotmentsKey = (organizationId: string) =>
	[...agentHoursKey, "groupAllotments", organizationId] as const;

export const agentHoursOrganizationAllotments = () => ({
	queryKey: agentHoursOrganizationAllotmentsKey,
	queryFn: API.getAgentHoursOrganizationAllotments,
});

export const agentHoursGroupAllotments = (organizationId: string) => ({
	queryKey: agentHoursGroupAllotmentsKey(organizationId),
	queryFn: () => API.getAgentHoursGroupAllotments(organizationId),
});

type OrganizationAllotmentChange = {
	organizationId: string;
	allotmentBps: number;
};

// An organization's own share is also part of its group allotments response,
// so organization-tier changes invalidate both views.
const invalidateOrganizationAllotment = (
	queryClient: QueryClient,
	organizationId: string,
) =>
	Promise.all([
		queryClient.invalidateQueries({
			queryKey: agentHoursOrganizationAllotmentsKey,
		}),
		queryClient.invalidateQueries({
			queryKey: agentHoursGroupAllotmentsKey(organizationId),
		}),
	]);

export const upsertAgentHoursOrganizationAllotment = (
	queryClient: QueryClient,
) => ({
	mutationFn: ({ organizationId, allotmentBps }: OrganizationAllotmentChange) =>
		API.upsertAgentHoursOrganizationAllotment(organizationId, {
			allotment_bps: allotmentBps,
		}),
	onSuccess: (_: unknown, { organizationId }: OrganizationAllotmentChange) =>
		invalidateOrganizationAllotment(queryClient, organizationId),
});

export const deleteAgentHoursOrganizationAllotment = (
	queryClient: QueryClient,
) => ({
	mutationFn: (organizationId: string) =>
		API.deleteAgentHoursOrganizationAllotment(organizationId),
	onSuccess: (_: unknown, organizationId: string) =>
		invalidateOrganizationAllotment(queryClient, organizationId),
});

type GroupAllotmentChange = {
	groupId: string;
	allotmentBps: number;
};

export const upsertAgentHoursGroupAllotment = (
	queryClient: QueryClient,
	organizationId: string,
) => ({
	mutationFn: ({ groupId, allotmentBps }: GroupAllotmentChange) =>
		API.upsertAgentHoursGroupAllotment(groupId, {
			allotment_bps: allotmentBps,
		}),
	onSuccess: () =>
		queryClient.invalidateQueries({
			queryKey: agentHoursGroupAllotmentsKey(organizationId),
		}),
});

export const deleteAgentHoursGroupAllotment = (
	queryClient: QueryClient,
	organizationId: string,
) => ({
	mutationFn: (groupId: string) => API.deleteAgentHoursGroupAllotment(groupId),
	onSuccess: () =>
		queryClient.invalidateQueries({
			queryKey: agentHoursGroupAllotmentsKey(organizationId),
		}),
});

// Group allotment writes authorize on updating groups, so this lists the
// organizations whose group allotments the user can manage.
export const agentHoursAllotmentOrganizations = () =>
	permittedOrganizations({
		object: { resource_type: "group" },
		action: "update",
	});

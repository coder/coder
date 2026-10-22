import { API } from "#/api/api";

export const workspaceQuotaKey = ["workspaceQuota"] as const;

export const getWorkspaceQuotaQueryKey = (
	organizationName: string,
	username: string,
) => {
	return [...workspaceQuotaKey, organizationName, username];
};

export const workspaceQuota = (organizationName: string, username: string) => {
	return {
		queryKey: getWorkspaceQuotaQueryKey(organizationName, username),
		queryFn: () => API.getWorkspaceQuota(organizationName, username),
	};
};

export const getWorkspaceResolveAutostartQueryKey = (workspaceId: string) => [
	workspaceId,
	"workspaceResolveAutostart",
];

export const workspaceResolveAutostart = (workspaceId: string) => {
	return {
		queryKey: getWorkspaceResolveAutostartQueryKey(workspaceId),
		queryFn: () => API.getWorkspaceResolveAutostart(workspaceId),
	};
};

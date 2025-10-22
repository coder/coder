import { API } from "api/api";

export const getWorkspaceQuotaQueryKey = (
	organizationName: string,
	username: string,
) => {
	return ["workspaceQuota", organizationName, username];
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

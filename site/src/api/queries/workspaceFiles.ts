import type { QueryClient, UseQueryOptions } from "react-query";
import {
	API,
	type WorkspaceAgentFileContents,
	type WorkspaceAgentFindFilesResponse,
	type WorkspaceAgentListDirectoryResponse,
} from "#/api/api";

const workspaceAgentFilesKey = (agentId: string) =>
	["workspaceAgentFiles", agentId] as const;

export const workspaceAgentDirectoryKey = (agentId: string, path: string) =>
	[...workspaceAgentFilesKey(agentId), "directory", path] as const;

export const workspaceAgentDirectory = (agentId: string, path: string) => {
	return {
		queryKey: workspaceAgentDirectoryKey(agentId, path),
		queryFn: () => API.listWorkspaceAgentDirectory(agentId, path),
	} satisfies UseQueryOptions<WorkspaceAgentListDirectoryResponse>;
};

export const workspaceAgentFileKey = (agentId: string, path: string) =>
	[...workspaceAgentFilesKey(agentId), "file", path] as const;

export const workspaceAgentFile = (agentId: string, path: string) => {
	return {
		queryKey: workspaceAgentFileKey(agentId, path),
		queryFn: () => API.readWorkspaceAgentFile(agentId, path),
	} satisfies UseQueryOptions<WorkspaceAgentFileContents>;
};

export const workspaceAgentFindFiles = (
	agentId: string,
	roots: readonly string[],
	query: string,
) => {
	return {
		queryKey: [...workspaceAgentFilesKey(agentId), "find", roots, query],
		queryFn: () => API.findWorkspaceAgentFiles(agentId, roots, query),
	} satisfies UseQueryOptions<WorkspaceAgentFindFilesResponse>;
};

type WriteWorkspaceAgentFileVariables = {
	path: string;
	content: string;
	ifMatch?: string;
};

export const writeWorkspaceAgentFile = (
	queryClient: QueryClient,
	agentId: string,
) => {
	return {
		mutationFn: ({
			path,
			content,
			ifMatch,
		}: WriteWorkspaceAgentFileVariables) =>
			API.writeWorkspaceAgentFile(agentId, path, content, ifMatch),
		onSuccess: (
			{ etag }: { etag: string },
			{ path, content }: WriteWorkspaceAgentFileVariables,
		) => {
			queryClient.setQueryData<WorkspaceAgentFileContents>(
				workspaceAgentFileKey(agentId, path),
				(previous) => ({
					data: new TextEncoder().encode(content).buffer,
					contentType: previous?.contentType ?? "",
					etag,
				}),
			);
		},
	};
};

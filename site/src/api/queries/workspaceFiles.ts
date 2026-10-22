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
		queryFn: ({ signal }) =>
			API.listWorkspaceAgentDirectory(agentId, path, signal),
	} satisfies UseQueryOptions<WorkspaceAgentListDirectoryResponse>;
};

export const workspaceAgentFileKey = (agentId: string, path: string) =>
	[...workspaceAgentFilesKey(agentId), "file", path] as const;

/** Text files above this size are not opened in the editor. */
export const workspaceAgentFileMaxTextBytes = 10 << 20;

// Git treats a file as binary when its first 8000 bytes contain a NUL.
const sniffBytes = 8 << 10;

type WorkspaceAgentFileQueryData = WorkspaceAgentFileContents & {
	/**
	 * Set when `data` holds only the first bytes, because the file is binary
	 * or a text file over workspaceAgentFileMaxTextBytes.
	 */
	partial?: "binary" | "too-large";
};

const looksBinary = (head: ArrayBuffer): boolean => {
	const bytes = new Uint8Array(head);
	if (bytes.includes(0)) {
		return true;
	}
	try {
		// stream: a multibyte character cut at the end of the slice is fine.
		new TextDecoder("utf-8", { fatal: true }).decode(bytes, { stream: true });
		return false;
	} catch {
		return true;
	}
};

/**
 * Reads the first bytes before the whole file, so binary and oversized text
 * files are never downloaded in full. Most files fit in the first read.
 */
export const workspaceAgentFile = (agentId: string, path: string) => {
	return {
		queryKey: workspaceAgentFileKey(agentId, path),
		queryFn: async (): Promise<WorkspaceAgentFileQueryData> => {
			const head = await API.readWorkspaceAgentFile(agentId, path, {
				limit: sniffBytes,
			});
			if (head.data.byteLength < sniffBytes) {
				return head;
			}
			if (head.contentType.startsWith("image/")) {
				return API.readWorkspaceAgentFile(agentId, path);
			}
			if (looksBinary(head.data)) {
				return { ...head, partial: "binary" };
			}
			const pastCap = await API.readWorkspaceAgentFile(agentId, path, {
				offset: workspaceAgentFileMaxTextBytes,
				limit: 1,
			});
			if (pastCap.data.byteLength > 0) {
				return { ...head, partial: "too-large" };
			}
			// The file can grow past the cap after the check above.
			const file = await API.readWorkspaceAgentFile(agentId, path, {
				limit: workspaceAgentFileMaxTextBytes + 1,
			});
			if (file.data.byteLength > workspaceAgentFileMaxTextBytes) {
				return {
					...file,
					data: file.data.slice(0, sniffBytes),
					partial: "too-large",
				};
			}
			return file;
		},
	} satisfies UseQueryOptions<WorkspaceAgentFileQueryData>;
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

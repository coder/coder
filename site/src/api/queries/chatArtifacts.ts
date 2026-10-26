import { type QueryClient, queryOptions } from "react-query";
import { API } from "#/api/api";
import type { ChatRole } from "#/api/typesGenerated";

const chatArtifactsKey = ["chat-artifacts"] as const;

/** Query key of one artifact with its version metadata. */
export const chatArtifactKey = (artifactId: string) =>
	[...chatArtifactsKey, artifactId] as const;

/** Query key of the text of one artifact version. */
export const chatArtifactVersionTextKey = (
	artifactId: string,
	version: number,
) => [...chatArtifactKey(artifactId), "versions", version, "text"] as const;

/**
 * Lists the artifacts the user owns or sees through a chat project, across
 * all organizations, newest update first. List entries carry no versions.
 */
export const chatArtifacts = () =>
	queryOptions({
		queryKey: [...chatArtifactsKey, "list"],
		queryFn: () => API.experimental.listChatArtifacts(),
	});

/** One artifact with the metadata of every version, newest first. */
export const chatArtifact = (artifactId: string) =>
	queryOptions({
		queryKey: chatArtifactKey(artifactId),
		queryFn: () => API.experimental.getChatArtifact(artifactId),
	});

/** The text of one version. Published versions never change. */
export const chatArtifactVersionText = (artifactId: string, version: number) =>
	queryOptions({
		queryKey: chatArtifactVersionTextKey(artifactId, version),
		queryFn: () =>
			API.experimental.getChatArtifactVersionText(artifactId, version),
		staleTime: Number.POSITIVE_INFINITY,
	});

/** Query key of the users and groups an artifact is shared with. */
export const chatArtifactACLKey = (artifactId: string) =>
	[...chatArtifactKey(artifactId), "acl"] as const;

/** The users and groups an artifact is shared with. */
export const chatArtifactACL = (artifactId: string) =>
	queryOptions({
		queryKey: chatArtifactACLKey(artifactId),
		queryFn: () => API.experimental.getChatArtifactACL(artifactId),
	});

/** Whether an artifact is shared with a user or with a group. */
export type ChatArtifactShareeKind = "user" | "group";

type SetChatArtifactRoleVariables = {
	artifactId: string;
	/** A user ID or a group ID, as `kind` says. */
	id: string;
	kind: ChatArtifactShareeKind;
	/** "read" shares the artifact, "" stops sharing it. */
	role: ChatRole;
};

/** Shares an artifact with one user or group, or stops sharing it. */
export const setChatArtifactRole = (queryClient: QueryClient) => ({
	mutationFn: ({ artifactId, id, kind, role }: SetChatArtifactRoleVariables) =>
		API.experimental.updateChatArtifactACL(
			artifactId,
			kind === "user"
				? { user_roles: { [id]: role } }
				: { group_roles: { [id]: role } },
		),
	onSuccess: async (
		_data: unknown,
		{ artifactId }: SetChatArtifactRoleVariables,
	) => {
		await queryClient.invalidateQueries({
			queryKey: chatArtifactACLKey(artifactId),
			exact: true,
		});
	},
});

import { queryOptions } from "react-query";
import { API } from "#/api/api";

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

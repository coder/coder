import type {
	ChatArtifactVersion,
	ChatMessage,
	ChatMessagePart,
	ChatToolResultPart,
} from "#/api/typesGenerated";
import { formatDateTime } from "#/utils/time";
import { isRasterImageMediaType } from "../../utils/chatAttachments";
import { asNumber, asRecord, asString } from "../ChatElements/runtimeTypeUtils";

type ArtifactToolResult = { artifactId: string; version: number };

const isArtifactToolResult = (
	part: ChatMessagePart,
): part is ChatToolResultPart =>
	part.type === "tool-result" &&
	!part.is_error &&
	(part.tool_name === "publish_artifact" || part.tool_name === "read_artifact");

/** Artifact tool results in message order, oldest first. */
const artifactToolResults = (
	messages: readonly ChatMessage[],
): ArtifactToolResult[] =>
	messages
		.flatMap((message) => message.content ?? [])
		.filter(isArtifactToolResult)
		.flatMap((part) => {
			const rec = asRecord(part.result);
			const artifactId = asString(rec?.artifact_id);
			return artifactId
				? [{ artifactId, version: asNumber(rec?.version) ?? 0 }]
				: [];
		});

/** Artifact IDs that this chat published or read, newest first. */
export const deriveArtifactIds = (
	messages: readonly ChatMessage[],
): string[] => {
	const ids: string[] = [];
	for (const { artifactId } of artifactToolResults(messages)) {
		if (!ids.includes(artifactId)) {
			ids.push(artifactId);
		}
	}
	return ids.reverse();
};

/**
 * The highest version that a tool result in this chat names, per artifact.
 * A cached artifact older than this misses a version that the chat published.
 */
export const deriveSeenArtifactVersions = (
	messages: readonly ChatMessage[],
): Map<string, number> => {
	const versions = new Map<string, number>();
	for (const { artifactId, version } of artifactToolResults(messages)) {
		versions.set(artifactId, Math.max(versions.get(artifactId) ?? 0, version));
	}
	return versions;
};

export type ArtifactKind =
	| "html"
	| "mermaid"
	| "markdown"
	| "image"
	| "text"
	| "download";

/** Picks the renderer for a version from its file name and media type. */
export const kindOf = (name: string, mediaType: string): ArtifactKind => {
	const ext = name.includes(".") ? name.split(".").pop()?.toLowerCase() : "";
	if (ext === "html" || ext === "htm") return "html";
	if (ext === "mmd" || ext === "mermaid") return "mermaid";
	if (ext === "md" || mediaType === "text/markdown") return "markdown";
	if (mediaType === "image/svg+xml" || isRasterImageMediaType(mediaType)) {
		return "image";
	}
	if (mediaType.startsWith("text/") || mediaType === "application/json") {
		return "text";
	}
	return "download";
};

export const getArtifactPagePath = (artifactId: string) =>
	`/agents/artifacts/${encodeURIComponent(artifactId)}`;

/** An absolute link to the artifact page, for "Copy link". */
export const getArtifactLink = (artifactId: string) =>
	`${window.location.origin}${getArtifactPagePath(artifactId)}`;

/**
 * Describes one version for the version menu: "v2 · label · this chat ·
 * Mar 3, 14:32". The chat title is set only when the viewer owns the chat.
 */
export const describeArtifactVersion = (
	version: ChatArtifactVersion,
	currentChatId: string | undefined,
): string => {
	const source =
		currentChatId !== undefined && version.chat_id === currentChatId
			? "this chat"
			: version.chat_title || "another chat";
	return [
		`v${version.version}`,
		version.label,
		source,
		formatDateTime(version.created_at, "MMM D, HH:mm"),
	]
		.filter(Boolean)
		.join(" · ");
};

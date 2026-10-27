import {
	CodeXmlIcon,
	FileCodeIcon,
	FileIcon,
	FileTextIcon,
	ImageIcon,
	WorkflowIcon,
} from "lucide-react";
import type {
	AuthorizationRequest,
	ChatArtifact,
	ChatArtifactVersion,
	ChatMessage,
	ChatMessagePart,
	ChatToolResultPart,
} from "#/api/typesGenerated";
import { formatDateTime } from "#/utils/time";
import { isRasterImageMediaType } from "../../utils/chatAttachments";
import { buildAgentChatPath } from "../../utils/navigation";
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

/** The renderer that kindOf picks for a version. */
type ArtifactKind =
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

/**
 * The icon of each kind. Index it during render with `kindOf`, so that
 * `react(static-components)` sees a static component.
 */
export const artifactKindIcons: Record<
	ArtifactKind,
	React.FC<{ className?: string }>
> = {
	html: CodeXmlIcon,
	mermaid: WorkflowIcon,
	markdown: FileTextIcon,
	image: ImageIcon,
	text: FileCodeIcon,
	download: FileIcon,
};

/** The route of the artifact gallery. Artifact pages live below it. */
export const artifactsGalleryPath = "/agents/artifacts";

/** The artifact page, at one version when `version` is set. */
export const getArtifactPagePath = (artifactId: string, version?: number) => {
	const path = `${artifactsGalleryPath}/${encodeURIComponent(artifactId)}`;
	if (version === undefined) {
		return path;
	}
	return `${path}?version=${version}`;
};

/**
 * An absolute link to the artifact page, for "Copy link". Without
 * `version` it opens the latest version.
 */
export const getArtifactLink = (artifactId: string, version?: number) =>
	`${window.location.origin}${getArtifactPagePath(artifactId, version)}`;

/** Asks whether the viewer may share the artifact, as `canShare`. */
export const getArtifactShareCheck = (
	artifact: ChatArtifact,
): AuthorizationRequest => ({
	checks: {
		canShare: {
			object: {
				resource_type: "chat_artifact",
				owner_id: artifact.owner_id,
				organization_id: artifact.organization_id,
			},
			action: "share",
		},
	},
});

/**
 * The version that an artifact view shows: the selected version, or the
 * latest one when the selection is unset or unknown.
 */
export const findShownVersion = (
	artifact: ChatArtifact,
	selectedVersion: number | undefined,
): ChatArtifactVersion | undefined => {
	const versions = artifact.versions ?? [];
	return versions.find((v) => v.version === selectedVersion) ?? versions[0];
};

/**
 * The path of the chat that published a version, when the viewer can open
 * it. The chat title is set only when the viewer owns the chat.
 */
export const getVersionChatPath = (
	version: ChatArtifactVersion,
): string | undefined =>
	version.chat_id && version.chat_title
		? buildAgentChatPath({ chatId: version.chat_id })
		: undefined;

/**
 * Names where a version came from. The chat title is set only when the
 * viewer owns the chat, so a version from another user's chat names that
 * user.
 */
const describeVersionSource = (
	version: ChatArtifactVersion,
	currentChatId: string | undefined,
): string => {
	if (currentChatId !== undefined && version.chat_id === currentChatId) {
		return "this chat";
	}
	if (version.chat_title) {
		return `your chat “${version.chat_title}”`;
	}
	if (version.created_by_username) {
		return `by ${version.created_by_username}`;
	}
	return "another chat";
};

/**
 * Describes one version for the version menu: "v2 · label · this chat ·
 * Mar 3, 14:32".
 */
export const describeArtifactVersion = (
	version: ChatArtifactVersion,
	currentChatId: string | undefined,
): string => {
	return [
		`v${version.version}`,
		version.label,
		describeVersionSource(version, currentChatId),
		formatDateTime(version.created_at, "MMM D, HH:mm"),
	]
		.filter(Boolean)
		.join(" · ");
};

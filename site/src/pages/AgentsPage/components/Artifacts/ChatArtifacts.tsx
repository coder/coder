import { PanelRightOpenIcon } from "lucide-react";
import type { ChatMessage } from "#/api/typesGenerated";
import { SectionHeader } from "../SectionHeader";
import { useArtifactTabs } from "./ArtifactTabsContext";
import {
	artifactKindIcons,
	deriveArtifactIds,
	deriveSeenArtifactVersions,
	kindOf,
} from "./artifactUtils";
import { useChatArtifact } from "./useChatArtifact";

type ChatArtifactsProps = {
	/** The loaded messages. Only their artifacts show. */
	messages: readonly ChatMessage[];
	/** Gates the artifact reads on the Summary tab being visible. */
	isVisible: boolean;
};

/**
 * The chat's artifacts in the Summary tab. A row opens the artifact in its
 * own right panel tab. Chats without artifacts show no section.
 */
export const ChatArtifacts: React.FC<ChatArtifactsProps> = ({
	messages,
	isVisible,
}) => {
	const artifactIds = deriveArtifactIds(messages);
	const seenVersions = deriveSeenArtifactVersions(messages);
	if (artifactIds.length === 0) {
		return null;
	}
	return (
		<section className="flex flex-col gap-3">
			<SectionHeader
				level="section"
				label={`Artifacts (${artifactIds.length.toLocaleString("en-US")})`}
			/>
			<ul className="m-0 flex list-none flex-col gap-2 p-0">
				{artifactIds.map((artifactId) => (
					<ChatArtifactRow
						key={artifactId}
						artifactId={artifactId}
						seenVersion={seenVersions.get(artifactId) ?? 0}
						isVisible={isVisible}
					/>
				))}
			</ul>
		</section>
	);
};

type ChatArtifactRowProps = {
	artifactId: string;
	/** The highest version that a tool result in the chat names. */
	seenVersion: number;
	isVisible: boolean;
};

const ChatArtifactRow: React.FC<ChatArtifactRowProps> = ({
	artifactId,
	seenVersion,
	isVisible,
}) => {
	const { onOpenArtifact } = useArtifactTabs();
	const { data, isError } = useChatArtifact(artifactId, seenVersion, {
		enabled: isVisible,
	});

	if (!data) {
		return (
			<li className="flex h-16 items-center rounded-md border border-solid border-border px-3 text-sm text-content-secondary">
				{isError ? "Unavailable artifact" : "Loading…"}
			</li>
		);
	}

	const title = data.title || "Untitled artifact";
	const Icon =
		artifactKindIcons[kindOf(data.latest_name, data.latest_media_type)];
	return (
		<li>
			<button
				type="button"
				onClick={() => onOpenArtifact?.(artifactId, title)}
				disabled={!onOpenArtifact}
				className="flex h-16 w-full items-center gap-3 rounded-md border border-solid border-border bg-surface-tertiary px-3 py-2 text-left transition-colors hover:bg-surface-quaternary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link disabled:hover:bg-surface-tertiary"
			>
				<span className="flex size-10 shrink-0 items-center justify-center rounded-md bg-surface-secondary text-content-secondary">
					<Icon className="size-4" />
				</span>
				<span className="flex min-w-0 flex-1 flex-col">
					<span className="truncate text-sm text-content-primary">{title}</span>
					<span className="truncate text-xs text-content-secondary">
						{data.latest_name} · v{data.latest_version}
					</span>
				</span>
				<PanelRightOpenIcon
					aria-hidden
					className="size-4 shrink-0 text-content-secondary"
				/>
			</button>
		</li>
	);
};

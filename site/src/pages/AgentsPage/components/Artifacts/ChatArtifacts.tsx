import { PanelRightOpenIcon } from "lucide-react";
import { useQuery } from "react-query";
import { chatArtifact } from "#/api/queries/chatArtifacts";
import { SectionHeader } from "../SectionHeader";
import { useArtifactsPanel } from "./ArtifactsPanelContext";
import { artifactIcon } from "./artifactUtils";

type ChatArtifactsProps = {
	/** The artifacts that the loaded messages published or read, newest first. */
	artifactIds: readonly string[];
	/** Gates the artifact reads on the Summary tab being visible. */
	isVisible: boolean;
};

/**
 * The chat's artifacts in the Summary tab. A row opens the artifact in its
 * own right panel tab. Chats without artifacts show no section.
 */
export const ChatArtifacts: React.FC<ChatArtifactsProps> = ({
	artifactIds,
	isVisible,
}) => {
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
						isVisible={isVisible}
					/>
				))}
			</ul>
		</section>
	);
};

type ChatArtifactRowProps = {
	artifactId: string;
	isVisible: boolean;
};

const ChatArtifactRow: React.FC<ChatArtifactRowProps> = ({
	artifactId,
	isVisible,
}) => {
	const { onOpenArtifact } = useArtifactsPanel();
	const { data, isError } = useQuery({
		...chatArtifact(artifactId),
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
	const Icon = artifactIcon(data);
	return (
		<li>
			<button
				type="button"
				onClick={() => onOpenArtifact?.(artifactId, title)}
				disabled={!onOpenArtifact}
				className="flex h-16 w-full cursor-pointer items-center gap-3 rounded-md border border-solid border-border bg-surface-tertiary px-3 py-2 text-left transition-colors hover:bg-surface-quaternary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
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

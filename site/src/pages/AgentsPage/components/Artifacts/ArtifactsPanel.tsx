import { cn } from "cn";
import { useEffect, useRef } from "react";
import { useQuery } from "react-query";
import { chatArtifact } from "#/api/queries/chatArtifacts";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { type ChatStore, useChatMessages } from "../ChatConversation/chatStore";
import { ArtifactView } from "./ArtifactView";
import { deriveArtifactIds, deriveSeenArtifactVersions } from "./artifactUtils";

export type ArtifactSelection = {
	artifactId: string;
	version?: number;
};

type ArtifactsPanelProps = {
	chatId: string;
	store: ChatStore;
	/** The user's choice. Without one, the newest artifact shows. */
	selection: ArtifactSelection | null;
	onSelect: (selection: ArtifactSelection) => void;
	onPrompt?: (text: string) => void;
};

/**
 * The Artifacts tab: the artifacts that this chat published or read, and one
 * selected artifact. It finds IDs only in loaded messages.
 */
export const ArtifactsPanel: React.FC<ArtifactsPanelProps> = ({
	chatId,
	store,
	selection,
	onSelect,
	onPrompt,
}) => {
	const messages = useChatMessages(store);
	const artifactIds = deriveArtifactIds(messages);
	const seenVersions = deriveSeenArtifactVersions(messages);
	const selectedId = selection?.artifactId ?? artifactIds[0];

	if (artifactIds.length === 0 || selectedId === undefined) {
		return (
			<div className="flex h-full min-h-0 items-center justify-center px-6 text-center text-xs text-content-secondary">
				No artifacts yet. Artifacts that the agent publishes or reads in this
				chat show here.
			</div>
		);
	}

	return (
		<div className="flex h-full min-h-0 flex-col">
			<ul
				aria-label="Artifacts in this chat"
				className="m-0 flex max-h-40 shrink-0 list-none flex-col gap-0.5 overflow-y-auto border-0 border-b border-solid border-border p-2"
			>
				{artifactIds.map((artifactId) => (
					<ArtifactListItem
						key={artifactId}
						artifactId={artifactId}
						seenVersion={seenVersions.get(artifactId) ?? 0}
						isSelected={artifactId === selectedId}
						onSelect={() => onSelect({ artifactId })}
					/>
				))}
			</ul>
			<SelectedArtifact
				artifactId={selectedId}
				version={
					selection?.artifactId === selectedId ? selection.version : undefined
				}
				chatId={chatId}
				onSelectVersion={(version) =>
					onSelect({ artifactId: selectedId, version })
				}
				onPrompt={onPrompt}
			/>
		</div>
	);
};

type ArtifactListItemProps = {
	artifactId: string;
	seenVersion: number;
	isSelected: boolean;
	onSelect: () => void;
};

const ArtifactListItem: React.FC<ArtifactListItemProps> = ({
	artifactId,
	seenVersion,
	isSelected,
	onSelect,
}) => {
	const artifactQuery = useQuery(chatArtifact(artifactId));
	const { data, refetch } = artifactQuery;
	const refetchedForVersionRef = useRef(0);

	// A tool result that names a newer version than the cache means the
	// chat published again. Refetch once for each such version.
	useEffect(() => {
		if (
			data &&
			seenVersion > data.latest_version &&
			refetchedForVersionRef.current < seenVersion
		) {
			refetchedForVersionRef.current = seenVersion;
			void refetch();
		}
	}, [data, seenVersion, refetch]);

	let label = "Loading…";
	if (data) {
		label = data.title || "Untitled artifact";
	} else if (artifactQuery.isError) {
		label = "Unavailable artifact";
	}

	return (
		<li>
			<button
				type="button"
				aria-current={isSelected ? "true" : undefined}
				onClick={onSelect}
				className={cn(
					"flex w-full min-w-0 cursor-pointer items-center gap-2 rounded-md border-0 bg-transparent px-2 py-1 text-left text-xs text-content-secondary hover:bg-surface-secondary hover:text-content-primary",
					isSelected && "bg-surface-secondary text-content-primary",
				)}
			>
				<span className="min-w-0 flex-1 truncate">{label}</span>
				{data && (
					<span className="shrink-0 text-content-secondary">
						v{data.latest_version}
					</span>
				)}
			</button>
		</li>
	);
};

type SelectedArtifactProps = {
	artifactId: string;
	version: number | undefined;
	chatId: string;
	onSelectVersion: (version: number) => void;
	onPrompt?: (text: string) => void;
};

const SelectedArtifact: React.FC<SelectedArtifactProps> = ({
	artifactId,
	version,
	chatId,
	onSelectVersion,
	onPrompt,
}) => {
	const artifactQuery = useQuery(chatArtifact(artifactId));
	// A failed background refetch keeps the cached artifact on screen.
	if (artifactQuery.data) {
		return (
			<ArtifactView
				artifact={artifactQuery.data}
				selectedVersion={version}
				onSelectVersion={onSelectVersion}
				currentChatId={chatId}
				onPrompt={onPrompt}
			/>
		);
	}
	if (artifactQuery.isError) {
		return (
			<div className="p-4">
				<ErrorAlert error={artifactQuery.error} />
			</div>
		);
	}
	return <Loader className="flex-1" label="Loading artifact" />;
};

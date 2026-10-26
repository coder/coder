import { CheckIcon, ChevronDownIcon, ShapesIcon } from "lucide-react";
import { useEffect, useRef } from "react";
import { useQuery } from "react-query";
import { Link } from "react-router";
import { chatArtifact } from "#/api/queries/chatArtifacts";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { Loader } from "#/components/Loader/Loader";
import { type ChatStore, useChatMessages } from "../ChatConversation/chatStore";
import { ArtifactView } from "./ArtifactView";
import {
	artifactsGalleryPath,
	deriveArtifactIds,
	deriveSeenArtifactVersions,
} from "./artifactUtils";

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
	/** The viewer, for the Share popover. */
	currentUserId: string;
};

/**
 * The Artifacts tab: one artifact that this chat published or read. The title
 * opens a menu that switches to the chat's other artifacts. It finds IDs only
 * in loaded messages.
 */
export const ArtifactsPanel: React.FC<ArtifactsPanelProps> = ({
	chatId,
	store,
	selection,
	onSelect,
	onPrompt,
	currentUserId,
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
			<SelectedArtifact
				artifactId={selectedId}
				seenVersion={seenVersions.get(selectedId) ?? 0}
				version={
					selection?.artifactId === selectedId ? selection.version : undefined
				}
				chatId={chatId}
				onSelectVersion={(version) =>
					onSelect({ artifactId: selectedId, version })
				}
				onPrompt={onPrompt}
				currentUserId={currentUserId}
				renderTitle={(title) => (
					<ArtifactSwitcher
						title={title}
						artifactIds={artifactIds}
						seenVersions={seenVersions}
						selectedId={selectedId}
						onSelect={(artifactId) => onSelect({ artifactId })}
					/>
				)}
			/>
		</div>
	);
};

/**
 * Loads an artifact and refetches it once for each newer version that a tool
 * result in the chat names, because that means the chat published again.
 */
const useChatArtifact = (artifactId: string, seenVersion: number) => {
	const artifactQuery = useQuery(chatArtifact(artifactId));
	const { data, refetch } = artifactQuery;
	const refetchedForVersionRef = useRef(0);

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

	return artifactQuery;
};

type ArtifactSwitcherProps = {
	/** The title of the artifact that shows. */
	title: string;
	artifactIds: readonly string[];
	seenVersions: ReadonlyMap<string, number>;
	selectedId: string;
	onSelect: (artifactId: string) => void;
};

/** The artifact title as a menu of the chat's artifacts and the gallery. */
const ArtifactSwitcher: React.FC<ArtifactSwitcherProps> = ({
	title,
	artifactIds,
	seenVersions,
	selectedId,
	onSelect,
}) => {
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<button
					type="button"
					className="inline-flex min-w-0 max-w-full cursor-pointer items-center gap-1 rounded-md border-0 bg-transparent px-1 py-0.5 text-left text-sm font-medium text-content-primary hover:bg-surface-secondary"
				>
					<span className="min-w-0 truncate">{title}</span>
					<ChevronDownIcon className="size-3.5 shrink-0 text-content-secondary" />
				</button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="start" className="w-72 max-w-[90vw]">
				<DropdownMenuLabel>
					In this chat ({artifactIds.length})
				</DropdownMenuLabel>
				<div className="max-h-72 overflow-y-auto">
					{artifactIds.map((artifactId) => (
						<ArtifactSwitcherItem
							key={artifactId}
							artifactId={artifactId}
							seenVersion={seenVersions.get(artifactId) ?? 0}
							isSelected={artifactId === selectedId}
							onSelect={() => onSelect(artifactId)}
						/>
					))}
				</div>
				<DropdownMenuSeparator />
				<DropdownMenuItem asChild>
					<Link to={artifactsGalleryPath}>
						<ShapesIcon />
						All artifacts
					</Link>
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

type ArtifactSwitcherItemProps = {
	artifactId: string;
	seenVersion: number;
	isSelected: boolean;
	onSelect: () => void;
};

const ArtifactSwitcherItem: React.FC<ArtifactSwitcherItemProps> = ({
	artifactId,
	seenVersion,
	isSelected,
	onSelect,
}) => {
	const artifactQuery = useChatArtifact(artifactId, seenVersion);
	const { data } = artifactQuery;

	let label = "Loading…";
	if (data) {
		label = data.title || "Untitled artifact";
	} else if (artifactQuery.isError) {
		label = "Unavailable artifact";
	}

	// The shown artifact stays enabled because Radix keeps disabled items
	// out of keyboard focus.
	return (
		<DropdownMenuItem
			aria-current={isSelected ? "true" : undefined}
			onSelect={isSelected ? undefined : onSelect}
		>
			<span className="min-w-0 flex-1 truncate">{label}</span>
			{data && (
				<span className="shrink-0 text-xs text-content-secondary">
					v{data.latest_version}
				</span>
			)}
			{isSelected ? (
				<CheckIcon className="text-content-primary" />
			) : (
				<span aria-hidden className="size-icon-sm shrink-0" />
			)}
		</DropdownMenuItem>
	);
};

type SelectedArtifactProps = {
	artifactId: string;
	seenVersion: number;
	version: number | undefined;
	chatId: string;
	onSelectVersion: (version: number) => void;
	onPrompt?: (text: string) => void;
	currentUserId: string;
	/** Renders the title, such as the artifact switcher. */
	renderTitle: (title: string) => React.ReactNode;
};

const SelectedArtifact: React.FC<SelectedArtifactProps> = ({
	artifactId,
	seenVersion,
	version,
	chatId,
	onSelectVersion,
	onPrompt,
	currentUserId,
	renderTitle,
}) => {
	const artifactQuery = useChatArtifact(artifactId, seenVersion);
	// A failed background refetch keeps the cached artifact on screen.
	if (artifactQuery.data) {
		return (
			<ArtifactView
				artifact={artifactQuery.data}
				selectedVersion={version}
				onSelectVersion={onSelectVersion}
				currentChatId={chatId}
				onPrompt={onPrompt}
				currentUserId={currentUserId}
				renderTitle={renderTitle}
				showPageLink
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

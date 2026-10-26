import { ShapesIcon } from "lucide-react";
import { useEffect, useRef } from "react";
import { type UseQueryResult, useQuery } from "react-query";
import { Link } from "react-router";
import { chatArtifact } from "#/api/queries/chatArtifacts";
import type { ChatArtifact } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
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
				artifactIds={artifactIds}
				chatId={chatId}
				onSelect={onSelect}
				onPrompt={onPrompt}
				currentUserId={currentUserId}
			/>
		</div>
	);
};

/** The name of an artifact in the tab, also while it loads or fails. */
const artifactLabel = (artifactQuery: UseQueryResult<ChatArtifact>) => {
	if (artifactQuery.data) {
		return artifactQuery.data.title || "Untitled artifact";
	}
	return artifactQuery.isError ? "Unavailable artifact" : "Loading…";
};

type ArtifactSwitcherProps = {
	/** The name of the artifact that shows. */
	label: string;
	artifactIds: readonly string[];
	selectedId: string;
	onSelect: (artifactId: string) => void;
};

/** The artifact title as a menu of the chat's artifacts and the gallery. */
const ArtifactSwitcher: React.FC<ArtifactSwitcherProps> = ({
	label,
	artifactIds,
	selectedId,
	onSelect,
}) => {
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					variant="subtle"
					size="sm"
					className="h-7 min-w-0 max-w-full justify-start px-1.5 text-sm text-content-primary"
				>
					<span className="min-w-0 truncate">{label}</span>
					<ChevronDownIcon className="text-content-secondary" />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="start" className="w-72 max-w-[90vw]">
				<DropdownMenuLabel>
					In this chat ({artifactIds.length})
				</DropdownMenuLabel>
				<DropdownMenuRadioGroup
					value={selectedId}
					onValueChange={onSelect}
					className="max-h-72 overflow-y-auto"
				>
					{artifactIds.map((artifactId) => (
						<ArtifactSwitcherItem key={artifactId} artifactId={artifactId} />
					))}
				</DropdownMenuRadioGroup>
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
};

const ArtifactSwitcherItem: React.FC<ArtifactSwitcherItemProps> = ({
	artifactId,
}) => {
	// Items mount when the menu opens, and the query refetches then, so the
	// version is current without the seen-version check.
	const artifactQuery = useQuery(chatArtifact(artifactId));
	return (
		<DropdownMenuRadioItem value={artifactId} className="gap-2">
			<span className="min-w-0 flex-1 truncate">
				{artifactLabel(artifactQuery)}
			</span>
			{artifactQuery.data && (
				<span className="shrink-0 text-xs text-content-secondary">
					v{artifactQuery.data.latest_version}
				</span>
			)}
		</DropdownMenuRadioItem>
	);
};

type SelectedArtifactProps = {
	artifactId: string;
	/** The highest version that a tool result in the chat names. */
	seenVersion: number;
	version: number | undefined;
	/** Every artifact of the chat, for the switcher. */
	artifactIds: readonly string[];
	chatId: string;
	onSelect: (selection: ArtifactSelection) => void;
	onPrompt?: (text: string) => void;
	currentUserId: string;
};

const SelectedArtifact: React.FC<SelectedArtifactProps> = ({
	artifactId,
	seenVersion,
	version,
	artifactIds,
	chatId,
	onSelect,
	onPrompt,
	currentUserId,
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

	const switcher = (
		<ArtifactSwitcher
			label={artifactLabel(artifactQuery)}
			artifactIds={artifactIds}
			selectedId={artifactId}
			onSelect={(id) => onSelect({ artifactId: id })}
		/>
	);
	// A failed background refetch keeps the cached artifact on screen.
	if (data) {
		return (
			<ArtifactView
				artifact={data}
				selectedVersion={version}
				onSelectVersion={(v) => onSelect({ artifactId, version: v })}
				currentChatId={chatId}
				onPrompt={onPrompt}
				currentUserId={currentUserId}
				title={switcher}
				showPageLink
			/>
		);
	}
	// The switcher stays while the artifact loads or fails, so the chat's
	// other artifacts stay reachable.
	return (
		<div className="flex min-h-0 flex-1 flex-col">
			<h3 className="m-0 flex shrink-0 border-0 border-b border-solid border-border px-3 py-2 text-sm font-medium">
				{switcher}
			</h3>
			{artifactQuery.isError ? (
				<div className="p-4">
					<ErrorAlert error={artifactQuery.error} />
				</div>
			) : (
				<Loader className="flex-1" label="Loading artifact" />
			)}
		</div>
	);
};

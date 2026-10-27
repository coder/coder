import { useId } from "react";
import type { Location } from "react-router";
import { CHAT_SEARCH_LIMIT } from "#/api/queries/chats";
import type { Chat, ChatProject } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	ChatSearchResultRow,
	ChatSearchResultsSkeleton,
	ProjectSearchResultRow,
	ShowAllProjectsRow,
} from "./ChatSearchResultRows";

/** How many recent chats the default view offers. */
export const RECENT_CHATS_COUNT = 10;

const COLLAPSED_PROJECT_COUNT = 3;

/**
 * Splits the projects to offer into the visible ones and the count behind
 * the "Show all" row.
 */
export const getVisibleProjects = (
	projects: readonly ChatProject[],
	isExpanded: boolean,
): { visible: readonly ChatProject[]; hiddenCount: number } => {
	const visible = isExpanded
		? projects
		: projects.slice(0, COLLAPSED_PROJECT_COUNT);
	return { visible, hiddenCount: projects.length - visible.length };
};

/**
 * Options are ordered as the visible projects, the "Show all" row when some
 * projects are hidden, then the chats. Returns the index of the first chat.
 */
export const getFirstChatOptionIndex = (
	visibleProjectCount: number,
	hiddenProjectCount: number,
): number => visibleProjectCount + (hiddenProjectCount > 0 ? 1 : 0);

/** The element ID of the option at `index`, for aria-activedescendant. */
export const getChatSearchOptionId = (listboxId: string, index: number) =>
	`${listboxId}-option-${index}`;

type ProjectResults = {
	readonly visible: readonly ChatProject[];
	/** Matching projects behind the "Show all" row. */
	readonly hiddenCount: number;
	readonly organizationLabels: ReadonlyMap<string, string> | undefined;
	readonly onShowAll: () => void;
};

type ChatSearchResultsProps = {
	readonly chats: readonly Chat[] | undefined;
	readonly recentChats: readonly Chat[];
	readonly projectResults: ProjectResults;
	/**
	 * True when projects are enabled and the query has text but no filter,
	 * so the empty state can say that projects were searched too.
	 */
	readonly searchesProjects: boolean;
	readonly error: unknown;
	readonly hasQuery: boolean;
	readonly hasSearchText: boolean;
	readonly location: Location;
	readonly listboxId: string;
	readonly selectedIndex: number | undefined;
	readonly showLoading: boolean;
	readonly isRefreshing: boolean;
	readonly onDismiss: () => void;
};

// !block overrides Radix ScrollArea viewport's display:table so truncated text can shrink.
const SCROLL_AREA_PROPS = {
	className:
		"h-[300px] w-full [&_[data-radix-scroll-area-viewport]>div]:block!",
	scrollBarClassName: "w-1.5",
	viewportClassName: "pr-3",
	viewportTabIndex: -1,
};

const ChatSearchErrorAlert: React.FC<{ readonly error: unknown }> = ({
	error,
}) => (
	<ErrorAlert
		error={error}
		className="max-h-[340px] overflow-y-auto [&_pre]:whitespace-pre-wrap [&_pre]:break-all [&_pre]:w-auto [&_pre]:min-w-0"
	/>
);

const pluralize = (count: number, noun: string) =>
	`${noun}${count === 1 ? "" : "s"}`;

export const ChatSearchResults: React.FC<ChatSearchResultsProps> = ({
	chats,
	recentChats,
	projectResults,
	searchesProjects,
	error,
	hasQuery,
	hasSearchText,
	location,
	listboxId,
	selectedIndex,
	showLoading,
	isRefreshing,
	onDismiss,
}) => {
	const projectCount =
		projectResults.visible.length + projectResults.hiddenCount;

	// Matching projects stay usable when only the chat search fails.
	if (error && projectCount === 0) {
		return (
			<div className="min-h-[260px]">
				<ChatSearchErrorAlert error={error} />
			</div>
		);
	}

	if (!hasQuery) {
		return (
			<DefaultView
				recentChats={recentChats}
				projectResults={projectResults}
				location={location}
				listboxId={listboxId}
				selectedIndex={selectedIndex}
				onDismiss={onDismiss}
			/>
		);
	}

	const chatCount = chats?.length ?? 0;
	const isChatLimitReached = chatCount === CHAT_SEARCH_LIMIT;
	let resultSummary: React.ReactNode;
	if (projectCount > 0) {
		// The chat count is unknown until the chat search succeeds.
		const showsChatCount = !showLoading && !error;
		resultSummary = (
			<>
				<span className="text-content-primary">{projectCount}</span>{" "}
				{pluralize(projectCount, "project")}
				{showsChatCount && (
					<>
						{" "}
						and {isChatLimitReached && "the first "}
						<span className="text-content-primary">{chatCount}</span>{" "}
						{pluralize(chatCount, "chat")}
					</>
				)}
			</>
		);
	} else if (isChatLimitReached) {
		resultSummary = (
			<>
				Showing first{" "}
				<span className="text-content-primary">{CHAT_SEARCH_LIMIT}</span>{" "}
				results.
			</>
		);
	} else {
		resultSummary = (
			<>
				<span className="text-content-primary">{chatCount}</span>{" "}
				{pluralize(chatCount, "result")}
			</>
		);
	}

	return (
		<div className="min-h-[260px]">
			<div className="space-y-3">
				<p className="m-0 text-sm text-content-secondary">
					<span>{resultSummary}</span>
					{isRefreshing && (
						<Spinner
							loading
							size="sm"
							className="ml-1.5 inline-block align-text-bottom text-content-secondary"
							aria-label="Searching chats"
						/>
					)}
				</p>
				<ScrollArea {...SCROLL_AREA_PROPS}>
					<ChatSearchResultsList
						chats={chats}
						projectResults={projectResults}
						searchesProjects={searchesProjects}
						error={error}
						hasSearchText={hasSearchText}
						location={location}
						listboxId={listboxId}
						selectedIndex={selectedIndex}
						showLoading={showLoading}
						onDismiss={onDismiss}
					/>
				</ScrollArea>
			</div>
		</div>
	);
};

// ---------------------------------------------------------------------------
// Option groups shared by the default view and the results list.
// ---------------------------------------------------------------------------

type ChatSearchResultsGroupProps = {
	readonly heading: string;
	readonly children: React.ReactNode;
};

const ChatSearchResultsGroup: React.FC<ChatSearchResultsGroupProps> = ({
	heading,
	children,
}) => {
	const headingId = useId();
	// A listbox may only contain groups and options, so the visible heading
	// is hidden from the accessibility tree and labels the group instead.
	return (
		<div role="presentation">
			<div
				id={headingId}
				aria-hidden="true"
				className="mb-3 text-sm font-medium text-content-secondary"
			>
				{heading}
			</div>
			<div role="group" aria-labelledby={headingId} className="space-y-1">
				{children}
			</div>
		</div>
	);
};

type ProjectOptionsProps = {
	readonly projectResults: ProjectResults;
	readonly location: Location;
	readonly listboxId: string;
	readonly selectedIndex: number | undefined;
	readonly onDismiss: () => void;
};

const ProjectOptions: React.FC<ProjectOptionsProps> = ({
	projectResults,
	location,
	listboxId,
	selectedIndex,
	onDismiss,
}) => {
	const showAllIndex = projectResults.visible.length;
	return (
		<ChatSearchResultsGroup heading="Projects">
			{projectResults.visible.map((project, index) => (
				<ProjectSearchResultRow
					key={project.id}
					project={project}
					organizationLabel={projectResults.organizationLabels?.get(project.id)}
					id={getChatSearchOptionId(listboxId, index)}
					isSelected={selectedIndex === index}
					location={location}
					onSelect={onDismiss}
				/>
			))}
			{projectResults.hiddenCount > 0 && (
				<ShowAllProjectsRow
					projectCount={
						projectResults.visible.length + projectResults.hiddenCount
					}
					id={getChatSearchOptionId(listboxId, showAllIndex)}
					isSelected={selectedIndex === showAllIndex}
					onShowAll={projectResults.onShowAll}
				/>
			)}
		</ChatSearchResultsGroup>
	);
};

type ChatOptionsProps = {
	readonly chats: readonly Chat[];
	readonly firstIndex: number;
	readonly location: Location;
	readonly listboxId: string;
	readonly selectedIndex: number | undefined;
	readonly onDismiss: () => void;
};

const ChatOptions: React.FC<ChatOptionsProps> = ({
	chats,
	firstIndex,
	location,
	listboxId,
	selectedIndex,
	onDismiss,
}) =>
	chats.map((chat, chatIndex) => {
		const index = firstIndex + chatIndex;
		return (
			<ChatSearchResultRow
				key={chat.id}
				chat={chat}
				id={getChatSearchOptionId(listboxId, index)}
				isSelected={selectedIndex === index}
				location={location}
				onSelect={onDismiss}
			/>
		);
	});

// ---------------------------------------------------------------------------
// Default view: projects and recent chats (shown when no query is active).
// ---------------------------------------------------------------------------

type DefaultViewProps = {
	readonly recentChats: readonly Chat[];
	readonly projectResults: ProjectResults;
	readonly location: Location;
	readonly listboxId: string;
	readonly selectedIndex: number | undefined;
	readonly onDismiss: () => void;
};

const DefaultView: React.FC<DefaultViewProps> = ({
	recentChats,
	projectResults,
	location,
	listboxId,
	selectedIndex,
	onDismiss,
}) => {
	const visibleRecentChats = recentChats.slice(0, RECENT_CHATS_COUNT);
	const hasProjects = projectResults.visible.length > 0;

	return (
		<div className="min-h-[260px]">
			{(hasProjects || visibleRecentChats.length > 0) && (
				<ScrollArea {...SCROLL_AREA_PROPS}>
					<div
						id={listboxId}
						role="listbox"
						aria-label={
							hasProjects ? "Projects and recent chats" : "Recent chats"
						}
						className="space-y-4"
					>
						{hasProjects && (
							<ProjectOptions
								projectResults={projectResults}
								location={location}
								listboxId={listboxId}
								selectedIndex={selectedIndex}
								onDismiss={onDismiss}
							/>
						)}
						{visibleRecentChats.length > 0 && (
							<ChatSearchResultsGroup heading="Recent chats">
								<ChatOptions
									chats={visibleRecentChats}
									firstIndex={getFirstChatOptionIndex(
										projectResults.visible.length,
										projectResults.hiddenCount,
									)}
									location={location}
									listboxId={listboxId}
									selectedIndex={selectedIndex}
									onDismiss={onDismiss}
								/>
							</ChatSearchResultsGroup>
						)}
					</div>
				</ScrollArea>
			)}
		</div>
	);
};

// ---------------------------------------------------------------------------
// Results list and row components.
// ---------------------------------------------------------------------------

type ChatSearchResultsListProps = {
	readonly chats: readonly Chat[] | undefined;
	readonly projectResults: ProjectResults;
	readonly searchesProjects: boolean;
	readonly error: unknown;
	readonly hasSearchText: boolean;
	readonly location: Location;
	readonly listboxId: string;
	readonly selectedIndex: number | undefined;
	readonly showLoading: boolean;
	readonly onDismiss: () => void;
};

const ChatSearchResultsList: React.FC<ChatSearchResultsListProps> = ({
	chats = [],
	projectResults,
	searchesProjects,
	error,
	hasSearchText,
	location,
	listboxId,
	selectedIndex,
	showLoading,
	onDismiss,
}) => {
	const hasProjects = projectResults.visible.length > 0;

	if (!hasProjects && showLoading) {
		return <ChatSearchResultsSkeleton />;
	}

	if (!hasProjects && chats.length === 0) {
		return (
			<div className="flex h-[300px] items-center justify-center px-6 text-center">
				<p className="text-sm text-content-secondary">
					{searchesProjects
						? "No matching chats or projects."
						: "No matching chats."}
					{hasSearchText && (
						<>
							{" "}
							Message content is indexed periodically, so very recent messages
							may not be searchable yet.
						</>
					)}
				</p>
			</div>
		);
	}

	if (!hasProjects) {
		return (
			<div
				id={listboxId}
				role="listbox"
				aria-label="Chat search results"
				className="space-y-1"
			>
				<ChatOptions
					chats={chats}
					firstIndex={0}
					location={location}
					listboxId={listboxId}
					selectedIndex={selectedIndex}
					onDismiss={onDismiss}
				/>
			</div>
		);
	}

	return (
		<div className="space-y-4">
			<div
				id={listboxId}
				role="listbox"
				aria-label="Search results"
				className="space-y-4"
			>
				<ProjectOptions
					projectResults={projectResults}
					location={location}
					listboxId={listboxId}
					selectedIndex={selectedIndex}
					onDismiss={onDismiss}
				/>
				{!error && !showLoading && chats.length > 0 && (
					<ChatSearchResultsGroup heading="Chats">
						<ChatOptions
							chats={chats}
							firstIndex={getFirstChatOptionIndex(
								projectResults.visible.length,
								projectResults.hiddenCount,
							)}
							location={location}
							listboxId={listboxId}
							selectedIndex={selectedIndex}
							onDismiss={onDismiss}
						/>
					</ChatSearchResultsGroup>
				)}
			</div>
			{error ? (
				<ChatSearchErrorAlert error={error} />
			) : (
				showLoading && <ChatSearchResultsSkeleton />
			)}
		</div>
	);
};

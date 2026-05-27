import { SettingsIcon, SquarePenIcon } from "lucide-react";
import { type FC, useEffect, useRef } from "react";
import { Link, type Location } from "react-router";
import { CHAT_SEARCH_LIMIT } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import { cn } from "#/utils/cn";
import { shortRelativeTime } from "#/utils/time";
import { getChatDisplayConfig } from "../tree/statusConfig";

// `!block` overrides the Radix ScrollArea viewport wrapper's inline
// `display: table` so that descendants using `truncate` can shrink
// inside the scroll container instead of forcing the table to grow.
const SCROLL_AREA_CLASS =
	"min-h-0 flex-1 w-full [&_[data-radix-scroll-area-viewport]>div]:!block";
const SCROLL_BAR_CLASS = "w-1.5";
const VIEWPORT_CLASS = "pr-3";

type ChatSearchResultsProps = {
	readonly chats: readonly Chat[] | undefined;
	readonly recentChats: readonly Chat[];
	readonly error: unknown;
	readonly hasQuery: boolean;
	readonly location: Location;
	readonly listboxId: string;
	readonly selectedChatIndex: number | undefined;
	readonly showLoading: boolean;
	readonly isRefreshing: boolean;
	readonly onDismiss: () => void;
	readonly onNewChat?: () => void;
};

const RECENT_CHATS_COUNT = 10;

export const ChatSearchResults: FC<ChatSearchResultsProps> = ({
	chats,
	recentChats,
	error,
	hasQuery,
	location,
	listboxId,
	selectedChatIndex,
	showLoading,
	isRefreshing,
	onDismiss,
	onNewChat,
}) => {
	if (error) {
		return (
			<div className="h-full">
				<ErrorAlert error={error} />
			</div>
		);
	}

	if (!hasQuery) {
		return (
			<DefaultView
				recentChats={recentChats}
				location={location}
				listboxId={listboxId}
				selectedChatIndex={selectedChatIndex}
				onDismiss={onDismiss}
				onNewChat={onNewChat}
			/>
		);
	}

	const resultCount = chats?.length ?? 0;
	const resultSummary =
		resultCount === CHAT_SEARCH_LIMIT ? (
			<>
				Showing first{" "}
				<span className="text-content-primary">{CHAT_SEARCH_LIMIT}</span>{" "}
				results.
			</>
		) : (
			<>
				<span className="text-content-primary">{resultCount}</span>{" "}
				{resultCount === 1 ? "result" : "results"}
			</>
		);

	return (
		<div className="flex h-full flex-col">
			<div className="flex min-h-0 flex-1 flex-col gap-3">
				<p className="inline-flex shrink-0 items-center gap-1.5 text-sm text-content-secondary">
					<span>{resultSummary}</span>
					{isRefreshing && (
						<Spinner
							loading
							size="sm"
							className="text-content-secondary"
							aria-label="Searching chats"
						/>
					)}
				</p>
				<ScrollArea
					className={SCROLL_AREA_CLASS}
					scrollBarClassName={SCROLL_BAR_CLASS}
					viewportClassName={VIEWPORT_CLASS}
					viewportTabIndex={-1}
				>
					<ChatSearchResultsList
						chats={chats}
						location={location}
						listboxId={listboxId}
						selectedChatIndex={selectedChatIndex}
						showLoading={showLoading}
						onDismiss={onDismiss}
					/>
				</ScrollArea>
			</div>
		</div>
	);
};

// ---------------------------------------------------------------------------
// Default view: quick actions + recent chats (shown when no query is active).
// ---------------------------------------------------------------------------

type DefaultViewProps = {
	readonly recentChats: readonly Chat[];
	readonly location: Location;
	readonly listboxId: string;
	readonly selectedChatIndex: number | undefined;
	readonly onDismiss: () => void;
	readonly onNewChat?: () => void;
};

const DefaultView: FC<DefaultViewProps> = ({
	recentChats,
	location,
	listboxId,
	selectedChatIndex,
	onDismiss,
	onNewChat,
}) => {
	const visibleRecentChats = recentChats.slice(0, RECENT_CHATS_COUNT);

	return (
		<div className="flex h-full flex-col gap-4">
			<div className="shrink-0">
				<h3 className="m-0 mb-2 text-xs font-medium text-content-secondary">
					Quick actions
				</h3>
				<div className="flex flex-wrap gap-2">
					{onNewChat && (
						<Button variant="outline" size="sm" onClick={onNewChat}>
							<SquarePenIcon className="size-4" />
							New agent
						</Button>
					)}
					<Button asChild variant="outline" size="sm">
						<Link to="/agents/settings" onClick={onDismiss}>
							<SettingsIcon className="size-4" />
							Settings
						</Link>
					</Button>
				</div>
			</div>
			{visibleRecentChats.length > 0 && (
				<div className="flex min-h-0 flex-1 flex-col">
					<h3 className="m-0 mb-2 shrink-0 text-xs font-medium text-content-secondary">
						Chats
					</h3>
					<ScrollArea
						className={SCROLL_AREA_CLASS}
						scrollBarClassName={SCROLL_BAR_CLASS}
						viewportClassName={VIEWPORT_CLASS}
						viewportTabIndex={-1}
					>
						<div id={listboxId} className="space-y-1">
							{visibleRecentChats.map((chat, index) => (
								<ChatSearchResultRow
									key={chat.id}
									chat={chat}
									id={`${listboxId}-option-${index}`}
									isSelected={selectedChatIndex === index}
									location={location}
									onSelect={onDismiss}
								/>
							))}
						</div>
					</ScrollArea>
				</div>
			)}
		</div>
	);
};

// ---------------------------------------------------------------------------
// Results list and row components.
// ---------------------------------------------------------------------------

type ChatSearchResultsListProps = {
	readonly chats: readonly Chat[] | undefined;
	readonly location: Location;
	readonly listboxId: string;
	readonly selectedChatIndex: number | undefined;
	readonly showLoading: boolean;
	readonly onDismiss: () => void;
};

const ChatSearchResultsList: FC<ChatSearchResultsListProps> = ({
	chats,
	location,
	listboxId,
	selectedChatIndex,
	showLoading,
	onDismiss,
}) => {
	if (showLoading) {
		return <ChatSearchResultsSkeleton />;
	}

	if ((chats?.length ?? 0) === 0) {
		return (
			<div className="flex h-full items-center justify-center">
				<p className="text-sm text-content-secondary">No matching chats</p>
			</div>
		);
	}

	return (
		<div
			id={listboxId}
			role="listbox"
			aria-label="Chat search results"
			className="space-y-1"
		>
			{chats?.map((chat, index) => (
				<ChatSearchResultRow
					key={chat.id}
					chat={chat}
					id={`${listboxId}-option-${index}`}
					isSelected={selectedChatIndex === index}
					location={location}
					onSelect={onDismiss}
				/>
			))}
		</div>
	);
};

type ChatSearchResultRowProps = {
	readonly chat: Chat;
	readonly id: string;
	readonly isSelected: boolean;
	readonly location: Location;
	readonly onSelect: () => void;
};

const ChatSearchResultRow: FC<ChatSearchResultRowProps> = ({
	chat,
	id,
	isSelected,
	location,
	onSelect,
}) => {
	const rowRef = useRef<HTMLAnchorElement | null>(null);
	const {
		icon: StatusIcon,
		className: statusClassName,
		diffStatus,
	} = getChatDisplayConfig(chat);
	const additions = diffStatus?.additions ?? 0;
	const deletions = diffStatus?.deletions ?? 0;
	const changedFiles = diffStatus?.changed_files ?? 0;
	const hasLineStats =
		Boolean(diffStatus?.url) &&
		(additions > 0 || deletions > 0 || changedFiles > 0);
	const subtitle = chat.last_turn_summary?.trim() || "No summary available";

	useEffect(() => {
		if (isSelected) {
			rowRef.current?.scrollIntoView({ block: "nearest" });
		}
	}, [isSelected]);

	return (
		<Link
			ref={rowRef}
			id={id}
			role="option"
			aria-selected={isSelected}
			tabIndex={-1}
			to={{ pathname: `/agents/${chat.id}`, search: location.search }}
			onClick={onSelect}
			className={cn(
				"grid w-full min-w-0 grid-cols-[auto_minmax(0,1fr)_auto] items-start gap-2 rounded-md px-1.5 py-1 text-content-secondary no-underline hover:bg-surface-tertiary/40 hover:text-content-primary",
				isSelected && "bg-surface-tertiary/40 text-content-primary",
			)}
		>
			<StatusIcon className={cn("mt-1 size-3.5 shrink-0", statusClassName)} />
			<div className="min-w-0">
				<div className="truncate text-sm text-content-primary">
					{chat.title}
				</div>
				<div className="flex min-w-0 items-center gap-1.5 text-xs">
					{hasLineStats && (
						<span className="inline-flex shrink-0 items-center gap-0.5 tabular-nums">
							<span className="text-git-added-bright">+{additions}</span>
							<span className="text-git-deleted-bright">
								&minus;{deletions}
							</span>
						</span>
					)}
					<span className="min-w-0 flex-1 truncate text-content-secondary">
						{subtitle}
					</span>
				</div>
			</div>
			<span className="inline-flex shrink-0 items-center gap-1.5 pt-0.5 text-xs text-content-secondary">
				{chat.has_unread && (
					<span
						className="size-1.5 shrink-0 rounded-full bg-content-link"
						aria-hidden="true"
					/>
				)}
				{shortRelativeTime(chat.updated_at)}
			</span>
		</Link>
	);
};

const ChatSearchResultsSkeleton: FC = () => (
	<div className="space-y-1.5">
		{Array.from({ length: 6 }, (_, index) => (
			<div
				key={`search-skeleton-${index}`}
				className="flex items-start gap-2 rounded-md px-1.5 py-1"
			>
				<Skeleton className="mt-1 size-3.5 shrink-0 rounded-full" />
				<div className="min-w-0 flex-1 space-y-1.5">
					<Skeleton
						className="h-3.5"
						style={{ width: `${60 + ((index * 11) % 30)}%` }}
					/>
					<Skeleton
						className="h-3"
						style={{ width: `${50 + ((index * 13) % 35)}%` }}
					/>
				</div>
				<Skeleton className="h-3 w-6" />
			</div>
		))}
	</div>
);

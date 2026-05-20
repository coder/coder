import type { LucideIcon } from "lucide-react";
import { type FC, useEffect, useRef } from "react";
import { Link, type Location } from "react-router";
import type { Chat } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { cn } from "#/utils/cn";
import { shortRelativeTime } from "#/utils/time";
import {
	getChatDiffStatus,
	getPRIconConfig,
	getStatusConfig,
} from "../tree/statusConfig";

type ChatSearchResultsProps = {
	readonly chats: readonly Chat[] | undefined;
	readonly error: unknown;
	readonly hasQuery: boolean;
	readonly location: Location;
	readonly listboxId: string;
	readonly selectedChatIndex: number | undefined;
	readonly showLoading: boolean;
	readonly onSelectChat: () => void;
};

export const ChatSearchResults: FC<ChatSearchResultsProps> = ({
	chats,
	error,
	hasQuery,
	location,
	listboxId,
	selectedChatIndex,
	showLoading,
	onSelectChat,
}) => {
	if (error) {
		return (
			<div className="min-h-[260px]">
				<ErrorAlert error={error} />
			</div>
		);
	}

	if (!hasQuery) {
		return (
			<div className="min-h-[260px]">
				<div className="pt-2 text-sm text-content-secondary">
					Type to search by title, or use filters like `has_unread:true`,
					`archived:true`, `pr_status:open`, or `diff_url:"..."`.
				</div>
			</div>
		);
	}

	const resultCount = chats?.length ?? 0;

	return (
		<div className="min-h-[260px]">
			<div className="space-y-3">
				<p className="text-sm text-content-secondary">
					<span className="text-content-primary">{resultCount}</span> results
				</p>
				<ScrollArea
					className="h-[300px]"
					scrollBarClassName="w-[0.375rem]"
					viewportClassName="pr-3"
					viewportTabIndex={-1}
				>
					<ChatSearchResultsList
						chats={chats}
						location={location}
						listboxId={listboxId}
						selectedChatIndex={selectedChatIndex}
						showLoading={showLoading}
						onSelectChat={onSelectChat}
					/>
				</ScrollArea>
			</div>
		</div>
	);
};

type ChatSearchResultsListProps = {
	readonly chats: readonly Chat[] | undefined;
	readonly location: Location;
	readonly listboxId: string;
	readonly selectedChatIndex: number | undefined;
	readonly showLoading: boolean;
	readonly onSelectChat: () => void;
};

const ChatSearchResultsList: FC<ChatSearchResultsListProps> = ({
	chats,
	location,
	listboxId,
	selectedChatIndex,
	showLoading,
	onSelectChat,
}) => {
	if (showLoading) {
		return <ChatSearchResultsSkeleton />;
	}

	if ((chats?.length ?? 0) === 0) {
		return (
			<p className="px-1.5 py-2 text-sm text-content-secondary">
				No matching chats
			</p>
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
					onSelect={onSelectChat}
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
	const diffStatus = getChatDiffStatus(chat);
	const baseConfig = getStatusConfig(chat.status);
	const prConfig =
		chat.status === "waiting" || chat.status === "completed"
			? getPRIconConfig(diffStatus)
			: undefined;
	const config = prConfig ?? baseConfig;
	const StatusIcon = config.icon as LucideIcon;
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
				"flex items-start gap-2 rounded-md px-1.5 py-1 text-content-secondary no-underline hover:bg-surface-tertiary/40 hover:text-content-primary",
				isSelected && "bg-surface-tertiary/40 text-content-primary",
			)}
		>
			<StatusIcon
				className={cn("mt-1 h-3.5 w-3.5 shrink-0", config.className)}
			/>
			<div className="min-w-0 flex-1">
				<div className="flex items-center gap-1.5">
					<span className="truncate text-sm text-content-primary">
						{chat.title}
					</span>
				</div>
				<div className="flex items-center gap-1.5 text-xs">
					{hasLineStats && (
						<span className="inline-flex shrink-0 items-center gap-0.5 tabular-nums">
							<span className="text-git-added-bright">+{additions}</span>
							<span className="text-git-deleted-bright">
								&minus;{deletions}
							</span>
						</span>
					)}
					<span className="truncate text-content-secondary">{subtitle}</span>
				</div>
			</div>
			<span className="inline-flex shrink-0 items-center gap-1.5 pt-0.5 text-xs text-content-secondary">
				{chat.has_unread && (
					<span
						className="h-1.5 w-1.5 shrink-0 rounded-full bg-content-link"
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
				<Skeleton className="mt-1 h-3.5 w-3.5 shrink-0 rounded-full" />
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

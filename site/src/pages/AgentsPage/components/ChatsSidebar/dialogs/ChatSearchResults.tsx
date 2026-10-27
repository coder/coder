import type { Location } from "react-router";
import { CHAT_SEARCH_LIMIT } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	ChatSearchResultRow,
	ChatSearchResultsSkeleton,
} from "./ChatSearchResultRows";

type ChatSearchResultsProps = {
	readonly chats: readonly Chat[] | undefined;
	readonly recentChats: readonly Chat[];
	readonly error: unknown;
	readonly hasQuery: boolean;
	readonly hasSearchText: boolean;
	readonly location: Location;
	readonly listboxId: string;
	readonly selectedChatIndex: number | undefined;
	readonly showLoading: boolean;
	readonly isRefreshing: boolean;
	readonly onDismiss: () => void;
};

const RECENT_CHATS_COUNT = 10;

// !block overrides Radix ScrollArea viewport's display:table so truncated text can shrink.
const SCROLL_AREA_PROPS = {
	className:
		"h-[300px] w-full [&_[data-radix-scroll-area-viewport]>div]:block!",
	scrollBarClassName: "w-1.5",
	viewportClassName: "pr-3",
	viewportTabIndex: -1,
};

export const ChatSearchResults: React.FC<ChatSearchResultsProps> = ({
	chats,
	recentChats,
	error,
	hasQuery,
	hasSearchText,
	location,
	listboxId,
	selectedChatIndex,
	showLoading,
	isRefreshing,
	onDismiss,
}) => {
	if (error) {
		return (
			<div className="min-h-[260px]">
				<ErrorAlert
					error={error}
					className="max-h-[340px] overflow-y-auto [&_pre]:whitespace-pre-wrap [&_pre]:break-all [&_pre]:w-auto [&_pre]:min-w-0"
				/>
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
						hasSearchText={hasSearchText}
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
// Default view: recent chats (shown when no query is active).
// ---------------------------------------------------------------------------

type DefaultViewProps = {
	readonly recentChats: readonly Chat[];
	readonly location: Location;
	readonly listboxId: string;
	readonly selectedChatIndex: number | undefined;
	readonly onDismiss: () => void;
};

const DefaultView: React.FC<DefaultViewProps> = ({
	recentChats,
	location,
	listboxId,
	selectedChatIndex,
	onDismiss,
}) => {
	const visibleRecentChats = recentChats.slice(0, RECENT_CHATS_COUNT);

	return (
		<div className="min-h-[260px]">
			<div className="space-y-3">
				{visibleRecentChats.length > 0 && (
					<div>
						<h3 className="m-0 mb-3 text-sm font-medium text-content-secondary">
							Recent chats
						</h3>
						<ScrollArea {...SCROLL_AREA_PROPS}>
							<div
								id={listboxId}
								role="listbox"
								aria-label="Recent chats"
								className="space-y-1"
							>
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
		</div>
	);
};

// ---------------------------------------------------------------------------
// Results list and row components.
// ---------------------------------------------------------------------------

type ChatSearchResultsListProps = {
	readonly chats: readonly Chat[] | undefined;
	readonly hasSearchText: boolean;
	readonly location: Location;
	readonly listboxId: string;
	readonly selectedChatIndex: number | undefined;
	readonly showLoading: boolean;
	readonly onDismiss: () => void;
};

const ChatSearchResultsList: React.FC<ChatSearchResultsListProps> = ({
	chats,
	hasSearchText,
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
			<div className="flex h-[300px] items-center justify-center px-6 text-center">
				<p className="text-sm text-content-secondary">
					No matching chats.
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

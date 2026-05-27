import { ChevronRightIcon } from "lucide-react";
import type { FC, RefObject } from "react";
import { type KeyboardEventHandler, useId, useRef, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { type Location, useNavigate } from "react-router";
import { chatSearch } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { Dialog, DialogContent, DialogTitle } from "#/components/Dialog/Dialog";
import { useDebouncedValue } from "#/hooks/debounce";
import { ChatSearchInput } from "./ChatSearchInput";
import { ChatSearchResults } from "./ChatSearchResults";
import { normalizeChatSearchInput } from "./searchQuery";

type ChatSearchDialogProps = {
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	readonly location: Location;
	readonly recentChats?: readonly Chat[];
	readonly onNewChat?: () => void;
};

const SEARCH_DEBOUNCE_MS = 500;

// Height of the tips + results area below the search input. Derived from
// p-6 (48px padding) + input (40px) + gap-4 (16px) = 104px of chrome,
// leaving ~370px for content in the ~480px dialog.
const CONTENT_AREA_HEIGHT = "h-[370px]";

export const ChatSearchDialog: FC<ChatSearchDialogProps> = ({
	open,
	onOpenChange,
	location,
	recentChats = [],
	onNewChat,
}) => {
	const inputRef = useRef<HTMLInputElement | null>(null);

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent
				// `top` is pinned so the dialog doesn't re-center when its
				// content height changes between states. The max(1rem, ...)
				// clamp keeps the dialog fully visible on short viewports.
				className="top-[max(1rem,_calc(50%_-_218px))] w-[calc(100vw-2rem)] max-w-[560px] translate-y-0 gap-4 border-border-default bg-surface-primary p-6 sm:p-6"
				// Suppress the open/close animation. The `animate-in`/`animate-out`
				// rules applied via CVA in `dialogVariants` outrank Tailwind class
				// overrides, so we disable them with an inline style to avoid the
				// dialog resizing visibly as results stream in.
				style={{ animation: "none", transition: "none" }}
				aria-describedby={undefined}
				onOpenAutoFocus={(event) => {
					event.preventDefault();
					requestAnimationFrame(() => {
						inputRef.current?.focus();
					});
				}}
			>
				<ChatSearchDialogContent
					open={open}
					onOpenChange={onOpenChange}
					location={location}
					inputRef={inputRef}
					recentChats={recentChats}
					onNewChat={onNewChat}
				/>
			</DialogContent>
		</Dialog>
	);
};

type ChatSearchDialogContentProps = ChatSearchDialogProps & {
	readonly inputRef: RefObject<HTMLInputElement | null>;
};

const ChatSearchDialogContent: FC<ChatSearchDialogContentProps> = ({
	open,
	onOpenChange,
	location,
	inputRef,
	recentChats = [],
	onNewChat,
}) => {
	const navigate = useNavigate();
	const [inputValue, setInputValue] = useState("");
	const [selectedChatIndex, setSelectedChatIndex] = useState<
		number | undefined
	>(undefined);
	const listboxId = useId();
	const debouncedInput = useDebouncedValue(inputValue, SEARCH_DEBOUNCE_MS);
	const normalizedQuery = normalizeChatSearchInput(debouncedInput);
	const hasQuery = inputValue.trim() !== "" && normalizedQuery !== undefined;

	const searchQuery = useQuery({
		...chatSearch(normalizedQuery ?? ""),
		enabled: open && hasQuery,
		placeholderData: keepPreviousData,
	});

	// Guard against stale keepPreviousData: only count results when an
	// active query exists. Without this, arrow+Enter after clearing the
	// input would navigate to a stale search result while the DOM shows
	// the recent chats default view.
	const resultCount = hasQuery ? (searchQuery.data?.length ?? 0) : 0;
	const safeSelectedChatIndex =
		selectedChatIndex !== undefined && selectedChatIndex < resultCount
			? selectedChatIndex
			: undefined;
	const selectedChat =
		safeSelectedChatIndex !== undefined
			? searchQuery.data?.[safeSelectedChatIndex]
			: undefined;
	const activeResultId =
		safeSelectedChatIndex !== undefined
			? `${listboxId}-option-${safeSelectedChatIndex}`
			: undefined;
	const closeDialog = () => onOpenChange(false);

	const showResultsLoading =
		hasQuery &&
		(searchQuery.isLoading ||
			(searchQuery.isFetching && (searchQuery.data?.length ?? 0) === 0));
	const isRefreshing =
		hasQuery &&
		searchQuery.isFetching &&
		searchQuery.isPlaceholderData &&
		!showResultsLoading;
	const handleInputKeyDown: KeyboardEventHandler<HTMLInputElement> = (
		event,
	) => {
		if (event.key === "ArrowDown" || event.key === "ArrowUp") {
			if (resultCount === 0) {
				return;
			}

			event.preventDefault();
			setSelectedChatIndex((previousIndex) => {
				if (previousIndex === undefined || previousIndex >= resultCount) {
					return event.key === "ArrowUp" ? resultCount - 1 : 0;
				}

				if (event.key === "ArrowDown") {
					return Math.min(previousIndex + 1, resultCount - 1);
				}

				return Math.max(previousIndex - 1, 0);
			});
			return;
		}

		if (event.key === "Enter" && selectedChat) {
			event.preventDefault();
			navigate({
				pathname: `/agents/${selectedChat.id}`,
				search: location.search,
			});
			closeDialog();
		}
	};

	const handleNewChat = onNewChat
		? () => {
				onNewChat();
				closeDialog();
			}
		: undefined;

	return (
		<>
			<DialogTitle className="sr-only">Search chats</DialogTitle>
			<ChatSearchInput
				activeResultId={activeResultId}
				hasResults={resultCount > 0}
				inputRef={inputRef}
				listboxId={listboxId}
				value={inputValue}
				onChange={(event) => {
					setInputValue(event.target.value);
					setSelectedChatIndex(undefined);
				}}
				onKeyDown={handleInputKeyDown}
			/>

			{/* Fixed-height wrapper so expanding SearchTips shrinks the
			   results scroll area instead of growing the dialog. */}
			<div className={`flex ${CONTENT_AREA_HEIGHT} flex-col gap-3`}>
				<SearchTips />
				<div className="min-h-0 flex-1">
					<ChatSearchResults
						chats={searchQuery.data}
						recentChats={recentChats}
						error={searchQuery.error}
						hasQuery={hasQuery}
						location={location}
						listboxId={listboxId}
						selectedChatIndex={safeSelectedChatIndex}
						showLoading={showResultsLoading}
						isRefreshing={isRefreshing}
						onDismiss={closeDialog}
						onNewChat={handleNewChat}
					/>
				</div>
			</div>
		</>
	);
};

// Collapsible search tips using the shared Radix Collapsible primitive.
const SearchTips: FC = () => (
	<Collapsible>
		<CollapsibleTrigger className="group inline-flex cursor-pointer items-center gap-1 border-none bg-transparent p-0 text-xs text-content-secondary hover:text-content-primary">
			Search tips
			<ChevronRightIcon className="size-3 transition-transform group-data-[state=open]:rotate-90" />
		</CollapsibleTrigger>
		<CollapsibleContent>
			<div className="mt-2 text-sm text-content-secondary">
				<p className="mb-1">Type to search chat titles, or use filters like:</p>
				<ul className="m-0 list-inside list-disc space-y-0.5 pl-1 text-xs">
					<li>
						<code>has_unread:true</code>
					</li>
					<li>
						<code>archived:true</code>
					</li>
					<li>
						<code>pr_status:open</code>
					</li>
					<li>
						<code>diff_url:"..."</code>
					</li>
				</ul>
			</div>
		</CollapsibleContent>
	</Collapsible>
);

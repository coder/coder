import type { FC, RefObject } from "react";
import { type KeyboardEventHandler, useId, useRef, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { type Location, useNavigate } from "react-router";
import { chatSearch } from "#/api/queries/chats";
import { Dialog, DialogContent, DialogTitle } from "#/components/Dialog/Dialog";
import { useDebouncedValue } from "#/hooks/debounce";
import { ChatSearchInput } from "./ChatSearchInput";
import { ChatSearchResults } from "./ChatSearchResults";
import { normalizeChatSearchInput } from "./searchQuery";

type ChatSearchDialogProps = {
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	readonly location: Location;
};

const SEARCH_DEBOUNCE_MS = 500;

export const ChatSearchDialog: FC<ChatSearchDialogProps> = ({
	open,
	onOpenChange,
	location,
}) => {
	const inputRef = useRef<HTMLInputElement | null>(null);

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent
				// `top` is pinned (rather than the default `top-1/2 -translate-y-1/2`)
				// so the dialog doesn't re-center when its content height changes
				// between the empty hint, loading skeleton, and results states.
				// 218px is half of the tallest content height (~436px: p-6 padding +
				// input + gap-4 + summary + space-y-3 + the 300px scroll area in
				// ChatSearchResults). The max(1rem, ...) clamp keeps the dialog
				// fully visible on short viewports.
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

	const resultCount = searchQuery.data?.length ?? 0;
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

			<ChatSearchResults
				chats={searchQuery.data}
				error={searchQuery.error}
				hasQuery={hasQuery}
				location={location}
				listboxId={listboxId}
				selectedChatIndex={safeSelectedChatIndex}
				showLoading={showResultsLoading}
				isRefreshing={isRefreshing}
				onSelectChat={closeDialog}
			/>
		</>
	);
};

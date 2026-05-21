import type { FC } from "react";
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
	const navigate = useNavigate();
	const [inputValue, setInputValue] = useState("");
	const [selectedChatIndex, setSelectedChatIndex] = useState<
		number | undefined
	>(undefined);
	const inputRef = useRef<HTMLInputElement | null>(null);
	const listboxId = useId();
	const debouncedInput = useDebouncedValue(inputValue, SEARCH_DEBOUNCE_MS);
	const normalizedQuery = normalizeChatSearchInput(debouncedInput);
	const hasQuery = inputValue.trim() !== "" && normalizedQuery !== undefined;

	const searchQuery = useQuery({
		...chatSearch(normalizedQuery ?? ""),
		enabled: open && hasQuery,
		// Keep the previous results visible while debouncing the next query.
		// Without this, every debounced keystroke would briefly flash the loading
		// skeleton between results sets.
		placeholderData: keepPreviousData,
	});

	const resultCount = searchQuery.data?.length ?? 0;
	// `selectedChatIndex` may reference an option that no longer exists if the
	// result set has shrunk since the user pressed an arrow key. Clamp to the
	// current results so stale state does not appear selected in the UI.
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
	const handleOpenChange = (nextOpen: boolean) => {
		if (!nextOpen) {
			setInputValue("");
			setSelectedChatIndex(undefined);
		}
		onOpenChange(nextOpen);
	};
	const closeDialog = () => handleOpenChange(false);

	const showResultsLoading =
		hasQuery &&
		(searchQuery.isLoading ||
			(searchQuery.isFetching && (searchQuery.data?.length ?? 0) === 0));
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
		<Dialog open={open} onOpenChange={handleOpenChange}>
			<DialogContent
				className="max-w-[560px] gap-4 border-border-default bg-surface-primary p-6 sm:p-6"
				aria-describedby={undefined}
				onOpenAutoFocus={(event) => {
					event.preventDefault();
					requestAnimationFrame(() => {
						inputRef.current?.focus();
					});
				}}
			>
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
					onSelectChat={closeDialog}
				/>
			</DialogContent>
		</Dialog>
	);
};

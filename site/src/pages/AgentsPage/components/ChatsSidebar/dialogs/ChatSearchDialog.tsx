import type { FC } from "react";
import {
	type KeyboardEventHandler,
	useEffect,
	useId,
	useRef,
	useState,
} from "react";
import { useQuery } from "react-query";
import { type Location, useNavigate } from "react-router";
import { chatSearch } from "#/api/queries/chats";
import { Dialog, DialogContent } from "#/components/Dialog/Dialog";
import { useDebouncedValue } from "#/hooks/debounce";
import { ChatSearchInput } from "./ChatSearchInput";
import { ChatSearchResults } from "./ChatSearchResults";
import { normalizeChatSearchInput } from "./searchQuery";

type ChatSearchDialogProps = {
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	readonly location: Location;
	readonly onBeforeNewAgent?: () => void;
};

export const ChatSearchDialog: FC<ChatSearchDialogProps> = ({
	open,
	onOpenChange,
	location,
	onBeforeNewAgent,
}) => {
	const navigate = useNavigate();
	const [inputValue, setInputValue] = useState("");
	const [highlightedChatIndex, setHighlightedChatIndex] = useState<
		number | undefined
	>(undefined);
	const inputRef = useRef<HTMLInputElement | null>(null);
	const listboxId = useId();
	const debouncedInput = useDebouncedValue(inputValue, 500);
	const normalizedQuery = normalizeChatSearchInput(debouncedInput);
	const hasQuery = normalizedQuery !== undefined;

	const searchQuery = useQuery({
		...chatSearch(normalizedQuery ?? ""),
		enabled: open && hasQuery,
	});

	const resultCount = searchQuery.data?.length ?? 0;
	const selectedChatIndex =
		highlightedChatIndex !== undefined && highlightedChatIndex < resultCount
			? highlightedChatIndex
			: undefined;
	const selectedChat =
		selectedChatIndex !== undefined
			? searchQuery.data?.[selectedChatIndex]
			: undefined;
	const activeResultId =
		selectedChatIndex !== undefined
			? `${listboxId}-option-${selectedChatIndex}`
			: undefined;
	const closeDialog = () => onOpenChange(false);

	useEffect(() => {
		if (open) {
			return;
		}
		setInputValue("");
		setHighlightedChatIndex(undefined);
	}, [open]);

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
			setHighlightedChatIndex((previousIndex) => {
				if (previousIndex === undefined || previousIndex >= resultCount) {
					return 0;
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
		<Dialog open={open} onOpenChange={onOpenChange}>
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
				<ChatSearchInput
					activeResultId={activeResultId}
					hasResults={resultCount > 0}
					inputRef={inputRef}
					listboxId={listboxId}
					value={inputValue}
					onChange={(event) => {
						setInputValue(event.target.value);
						setHighlightedChatIndex(undefined);
					}}
					onKeyDown={handleInputKeyDown}
				/>

				<ChatSearchResults
					chats={searchQuery.data}
					error={searchQuery.error}
					hasQuery={hasQuery}
					location={location}
					listboxId={listboxId}
					selectedChatIndex={selectedChatIndex}
					showLoading={showResultsLoading}
					onSelectChat={closeDialog}
				/>
			</DialogContent>
		</Dialog>
	);
};

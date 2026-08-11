import {
	ArchiveIcon,
	CircleDotIcon,
	FileTextIcon,
	LinkIcon,
} from "lucide-react";
import type { FC, RefObject } from "react";
import { type KeyboardEventHandler, useId, useRef, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { type Location, useNavigate } from "react-router";
import { chatSearch } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Dialog, DialogContent, DialogTitle } from "#/components/Dialog/Dialog";
import { useDebouncedValue } from "#/hooks/debounce";
import { ChatSearchInput, type SearchFilter } from "./ChatSearchInput";
import { ChatSearchResults } from "./ChatSearchResults";
import {
	buildChatSearchQuery,
	CHAT_SEARCH_FILTER_KEYS,
	CHAT_SEARCH_KNOWN_FILTER_KEYS,
	type ChatSearchFilterKey,
	extractTypedFilters,
	isValidChatSearchFilterValue,
} from "./searchQuery";

// Filter definitions. Filters with a defaultValue are inserted as complete
// pills (e.g. has_unread:true). Filters without one are inserted as
// incomplete pills so the user can type the value.
type FilterDefinition = {
	readonly key: ChatSearchFilterKey;
	readonly label: string;
	readonly icon: FC<{ className?: string }>;
	readonly defaultValue: string | null;
	readonly validate: (value: string) => boolean;
};

const FILTER_DEFINITIONS_BY_KEY: Readonly<
	Record<ChatSearchFilterKey, Omit<FilterDefinition, "key">>
> = {
	has_unread: {
		label: "Unread",
		icon: CircleDotIcon,
		defaultValue: "true",
		validate: (value) => isValidChatSearchFilterValue("has_unread", value),
	},
	archived: {
		label: "Archived",
		icon: ArchiveIcon,
		defaultValue: "true",
		validate: (value) => isValidChatSearchFilterValue("archived", value),
	},
	pr_status: {
		label: "PR status",
		icon: FileTextIcon,
		defaultValue: null,
		validate: (value) => isValidChatSearchFilterValue("pr_status", value),
	},
	diff_url: {
		label: "Diff URL",
		icon: LinkIcon,
		defaultValue: null,
		validate: (value) => isValidChatSearchFilterValue("diff_url", value),
	},
};

const FILTER_DEFINITIONS: readonly FilterDefinition[] =
	CHAT_SEARCH_FILTER_KEYS.map((key) => ({
		key,
		...FILTER_DEFINITIONS_BY_KEY[key],
	}));

type ChatSearchDialogProps = {
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	readonly focusInputOnOpen?: boolean;
	readonly location: Location;
	readonly recentChats?: readonly Chat[];
};

const SEARCH_DEBOUNCE_MS = 500;

export const ChatSearchDialog: FC<ChatSearchDialogProps> = ({
	open,
	onOpenChange,
	focusInputOnOpen = true,
	location,
	recentChats = [],
}) => {
	const contentRef = useRef<HTMLDivElement | null>(null);
	const inputRef = useRef<HTMLInputElement | null>(null);

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent
				ref={contentRef}
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
				tabIndex={-1}
				// When opened from the mobile sidebar button, skip autofocusing
				// the input so the virtual keyboard doesn't push the dialog
				// off-screen. Focus the dialog container instead to keep the
				// element in the accessibility tree.
				onOpenAutoFocus={(event) => {
					if (focusInputOnOpen) {
						return;
					}
					event.preventDefault();
					contentRef.current?.focus({ preventScroll: true });
				}}
			>
				<ChatSearchDialogContent
					open={open}
					onOpenChange={onOpenChange}
					location={location}
					inputRef={inputRef}
					recentChats={recentChats}
				/>
			</DialogContent>
		</Dialog>
	);
};

type ChatSearchDialogContentProps = Omit<
	ChatSearchDialogProps,
	"focusInputOnOpen"
> & {
	readonly inputRef: RefObject<HTMLInputElement | null>;
};

const ChatSearchDialogContent: FC<ChatSearchDialogContentProps> = ({
	open,
	onOpenChange,
	location,
	inputRef,
	recentChats = [],
}) => {
	const navigate = useNavigate();
	const [filters, setFilters] = useState<SearchFilter[]>([]);
	const [freeText, setFreeText] = useState("");
	// Tracks the key of a parameterized filter being typed (e.g. "pr_status").
	// While set, freeText holds the in-progress value and the pill shows as
	// incomplete (dashed border). Space or Enter commits the value.
	const [incompleteFilterKey, setIncompleteFilterKey] = useState<string | null>(
		null,
	);
	const [isDropdownOpen, setIsDropdownOpen] = useState(false);
	const [selectedChatIndex, setSelectedChatIndex] = useState<
		number | undefined
	>(undefined);
	const listboxId = useId();

	// Debounce as one snapshot so a committed incomplete-filter value cannot
	// reappear as full-text search.
	const queryFilters =
		incompleteFilterKey && freeText.trim()
			? [...filters, { key: incompleteFilterKey, value: freeText.trim() }]
			: filters;
	const queryFreeText = incompleteFilterKey ? "" : freeText;
	const currentQuery = buildChatSearchQuery(queryFilters, queryFreeText);
	const hasActiveSearch =
		queryFilters.length > 0 || queryFreeText.trim() !== "";
	const querySnapshot = JSON.stringify(currentQuery);
	const debouncedQuerySnapshot = useDebouncedValue(
		querySnapshot,
		SEARCH_DEBOUNCE_MS,
	);
	const debouncedQueryResult: ReturnType<typeof buildChatSearchQuery> =
		JSON.parse(debouncedQuerySnapshot);
	const { query: debouncedQuery, hasSearchText } = debouncedQueryResult;
	const hasQuery = hasActiveSearch && debouncedQuery !== undefined;

	const searchQuery = useQuery({
		...chatSearch({ q: debouncedQuery ?? "" }),
		enabled: open && hasQuery,
		placeholderData: keepPreviousData,
	});

	// Use search results count when a query is active, otherwise count
	// recent chats so keyboard navigation works in the default view too.
	const recentChatsSlice = (recentChats ?? []).slice(0, 10);
	const resultCount = hasQuery
		? (searchQuery.data?.length ?? 0)
		: recentChatsSlice.length;
	const safeSelectedChatIndex =
		selectedChatIndex !== undefined && selectedChatIndex < resultCount
			? selectedChatIndex
			: undefined;
	const selectedChat =
		safeSelectedChatIndex !== undefined
			? hasQuery
				? searchQuery.data?.[safeSelectedChatIndex]
				: recentChatsSlice[safeSelectedChatIndex]
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

	const commitIncompleteFilter = () => {
		const value = freeText.trim();
		const definition = FILTER_DEFINITIONS.find(
			(def) => def.key === incompleteFilterKey,
		);
		if (incompleteFilterKey && definition?.validate(value)) {
			setFilters((previous) => [
				...previous.filter((filter) => filter.key !== incompleteFilterKey),
				{ key: incompleteFilterKey, value },
			]);
			setFreeText("");
			setIncompleteFilterKey(null);
		}
	};

	const addFilter = (def: FilterDefinition) => {
		if (
			filters.some((f) => f.key === def.key) ||
			incompleteFilterKey === def.key
		) {
			return;
		}
		commitIncompleteFilter();

		if (def.defaultValue !== null) {
			setFilters((prev) => [
				...prev,
				{ key: def.key, value: def.defaultValue },
			]);
		} else {
			setIncompleteFilterKey(def.key);
			setFreeText("");
		}
		setIsDropdownOpen(false);
		setSelectedChatIndex(undefined);
		requestAnimationFrame(() => inputRef.current?.focus());
	};

	const removeFilter = (key: string) => {
		if (incompleteFilterKey === key) {
			setIncompleteFilterKey(null);
			setFreeText("");
		} else {
			setFilters((prev) => prev.filter((f) => f.key !== key));
		}
		setSelectedChatIndex(undefined);
		requestAnimationFrame(() => inputRef.current?.focus());
	};

	const handleInputChange = (value: string) => {
		setFreeText(value);
		setSelectedChatIndex(undefined);
	};

	// Build the display filters for ChatSearchInput: completed filters plus
	// the incomplete one (shown with dashed border).
	const displayFilters: SearchFilter[] = incompleteFilterKey
		? [...filters, { key: incompleteFilterKey, value: null }]
		: filters;

	const handleInputKeyDown: KeyboardEventHandler<HTMLInputElement> = (
		event,
	) => {
		if (
			(event.key === " " || event.key === "Enter") &&
			incompleteFilterKey &&
			freeText.trim()
		) {
			event.preventDefault();
			commitIncompleteFilter();
			return;
		}

		if (
			(event.key === " " || event.key === "Enter") &&
			!incompleteFilterKey &&
			freeText.trim() &&
			event.currentTarget.selectionStart === freeText.length &&
			event.currentTarget.selectionEnd === freeText.length
		) {
			const extracted = extractTypedFilters(
				freeText,
				CHAT_SEARCH_KNOWN_FILTER_KEYS,
				filters,
			);
			if (extracted.consumed) {
				event.preventDefault();
				setFilters((previous) => {
					const replacements = new Map(
						extracted.filters.map((filter) => [filter.key, filter]),
					);
					const next = previous.map(
						(filter) => replacements.get(filter.key) ?? filter,
					);
					for (const filter of extracted.filters) {
						if (!previous.some((existing) => existing.key === filter.key)) {
							next.push(filter);
						}
					}
					return next;
				});
				setFreeText(
					event.key === " " && extracted.remainingText
						? `${extracted.remainingText.trimEnd()} `
						: extracted.remainingText.trimEnd(),
				);
				return;
			}
		}

		if (event.key === "Backspace" && freeText === "") {
			if (incompleteFilterKey) {
				setIncompleteFilterKey(null);
				return;
			}
			if (filters.length > 0) {
				const lastFilter = filters[filters.length - 1];
				removeFilter(lastFilter.key);
				return;
			}
		}

		if (event.key === "Escape" && isDropdownOpen) {
			setIsDropdownOpen(false);
			event.stopPropagation();
			return;
		}

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
			{/* Wrap input + dropdown so onBlur on the container closes
				   the dropdown, but clicks within the dropdown (which is
				   inside the same container) don't trigger blur. */}
			<div
				className="relative w-full min-w-0 max-w-full"
				onBlur={(e) => {
					if (!e.currentTarget.contains(e.relatedTarget)) {
						setIsDropdownOpen(false);
					}
				}}
			>
				<ChatSearchInput
					activeResultId={activeResultId}
					hasResults={resultCount > 0}
					inputRef={inputRef}
					listboxId={listboxId}
					filters={displayFilters}
					value={freeText}
					onChange={(event) => handleInputChange(event.target.value)}
					onKeyDown={handleInputKeyDown}
					onRemoveFilter={removeFilter}
					isDropdownOpen={isDropdownOpen}
					onToggleDropdown={() => setIsDropdownOpen((prev) => !prev)}
				/>
				{isDropdownOpen && (
					<FilterDropdown filters={displayFilters} onSelectFilter={addFilter} />
				)}
			</div>

			<ChatSearchResults
				chats={searchQuery.data}
				recentChats={recentChats}
				error={hasQuery ? searchQuery.error : undefined}
				hasQuery={hasQuery}
				hasSearchText={hasSearchText}
				location={location}
				listboxId={listboxId}
				selectedChatIndex={safeSelectedChatIndex}
				showLoading={showResultsLoading}
				isRefreshing={isRefreshing}
				onDismiss={closeDialog}
			/>
		</>
	);
};

// ---------------------------------------------------------------------------
// Filter dropdown: appears on focus, shows clickable filter chips.
// ---------------------------------------------------------------------------

const FilterDropdown: FC<{
	readonly filters: readonly SearchFilter[];
	readonly onSelectFilter: (def: FilterDefinition) => void;
}> = ({ filters, onSelectFilter }) => {
	const activeKeys = new Set(filters.map((f) => f.key));

	return (
		<div className="absolute left-0 right-0 top-full z-10 mt-1 rounded-md border border-solid border-border bg-surface-primary p-3 shadow-md">
			<h3 className="m-0 mb-2 text-xs font-medium text-content-secondary">
				Filter by
			</h3>
			<div className="flex flex-wrap gap-2">
				{FILTER_DEFINITIONS.map((def) => {
					const Icon = def.icon;
					const isActive = activeKeys.has(def.key);
					return (
						<Button
							key={def.key}
							variant="outline"
							size="sm"
							disabled={isActive}
							onClick={() => onSelectFilter(def)}
						>
							<Icon className="size-4" />
							{def.label}
						</Button>
					);
				})}
			</div>
		</div>
	);
};

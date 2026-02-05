import { Command as CommandPrimitive } from "cmdk";
import { Badge } from "components/Badge/Badge";
import {
	Command,
	CommandGroup,
	CommandItem,
	CommandList,
} from "components/Command/Command";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { SlidersHorizontal, Search, X, ChevronDown } from "lucide-react";
import {
	type FC,
	type KeyboardEvent,
	type ReactNode,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { cn } from "utils/cn";

// Types for filter tokens
export interface FilterOption {
	value: string;
	label: string;
	icon?: ReactNode;
}

export interface FilterDefinition {
	key: string;
	label: string;
	options: FilterOption[];
	/** Allows typing custom values not in options list */
	allowCustom?: boolean;
}

export interface FilterToken {
	key: string;
	value: string;
	label: string;
}

interface TokenSearchProps {
	/** Available filter definitions */
	filters: FilterDefinition[];
	/** Currently applied filter tokens */
	tokens: FilterToken[];
	/** Callback when tokens change */
	onTokensChange: (tokens: FilterToken[]) => void;
	/** Placeholder text for the search input */
	placeholder?: string;
	/** Optional free-text search value */
	searchValue?: string;
	/** Callback for free-text search changes */
	onSearchChange?: (value: string) => void;
	/** Custom class name */
	className?: string;
}

type SuggestionMode = "filters" | "values";

export const TokenSearch: FC<TokenSearchProps> = ({
	filters,
	tokens,
	onTokensChange,
	placeholder = "Search...",
	searchValue = "",
	onSearchChange,
	className,
}) => {
	const inputRef = useRef<HTMLInputElement>(null);
	const containerRef = useRef<HTMLDivElement>(null);
	const [inputValue, setInputValue] = useState("");
	const [isOpen, setIsOpen] = useState(false);
	const [suggestionMode, setSuggestionMode] = useState<SuggestionMode>("filters");
	const [activeFilterKey, setActiveFilterKey] = useState<string | null>(null);

	// Parse input to detect if user is typing a filter key
	const parseInput = useCallback((value: string) => {
		const colonIndex = value.indexOf(":");
		if (colonIndex === -1) {
			return { filterKey: null, filterValue: value };
		}
		return {
			filterKey: value.slice(0, colonIndex).toLowerCase(),
			filterValue: value.slice(colonIndex + 1),
		};
	}, []);

	// Get current parsing state
	const { filterKey, filterValue } = useMemo(
		() => parseInput(inputValue),
		[inputValue, parseInput],
	);

	// Find matching filter definition
	const matchedFilter = useMemo(() => {
		if (!filterKey) return null;
		return filters.find(
			(f) =>
				f.key.toLowerCase() === filterKey ||
				f.label.toLowerCase() === filterKey,
		);
	}, [filterKey, filters]);

	// Update suggestion mode based on input
	useEffect(() => {
		if (matchedFilter) {
			setSuggestionMode("values");
			setActiveFilterKey(matchedFilter.key);
		} else if (filterKey && filterKey.length > 0) {
			// Typing a potential filter key, show matching filters
			setSuggestionMode("filters");
			setActiveFilterKey(null);
		} else {
			setSuggestionMode("filters");
			setActiveFilterKey(null);
		}
	}, [matchedFilter, filterKey]);

	// Filter suggestions based on current mode and input
	const suggestions = useMemo((): FilterOption[] => {
		if (suggestionMode === "values" && matchedFilter) {
			const searchTerm = filterValue.toLowerCase();
			return matchedFilter.options.filter(
				(opt) =>
					opt.label.toLowerCase().includes(searchTerm) ||
					opt.value.toLowerCase().includes(searchTerm),
			);
		}

		// Show filter keys
		const searchTerm = inputValue.toLowerCase();
		return filters
			.filter(
				(f) =>
					f.key.toLowerCase().includes(searchTerm) ||
					f.label.toLowerCase().includes(searchTerm),
			)
			.map((f): FilterOption => ({
				value: f.key,
				label: f.label,
			}));
	}, [suggestionMode, matchedFilter, filterValue, inputValue, filters]);

	// Handle selecting a filter key from dropdown
	const handleFilterSelect = useCallback((filter: FilterDefinition) => {
		setInputValue(`${filter.key}:`);
		setSuggestionMode("values");
		setActiveFilterKey(filter.key);
		setIsOpen(true);
		inputRef.current?.focus();
	}, []);

	// Handle selecting a value (completes the token)
	const handleValueSelect = useCallback(
		(option: FilterOption) => {
			if (!activeFilterKey) return;

			const filter = filters.find((f) => f.key === activeFilterKey);
			if (!filter) return;

			const newToken: FilterToken = {
				key: activeFilterKey,
				value: option.value,
				label: option.label,
			};

			// Replace existing token with same key or add new one
			const existingIndex = tokens.findIndex((t) => t.key === activeFilterKey);
			let newTokens: FilterToken[];
			if (existingIndex >= 0) {
				newTokens = [...tokens];
				newTokens[existingIndex] = newToken;
			} else {
				newTokens = [...tokens, newToken];
			}

			onTokensChange(newTokens);
			setInputValue("");
			setSuggestionMode("filters");
			setActiveFilterKey(null);
			setIsOpen(false);
			inputRef.current?.focus();
		},
		[activeFilterKey, filters, tokens, onTokensChange],
	);

	// Handle selecting a suggestion (could be filter or value)
	const handleSuggestionSelect = useCallback(
		(suggestion: FilterOption) => {
			if (suggestionMode === "filters") {
				const filter = filters.find((f) => f.key === suggestion.value);
				if (filter) {
					handleFilterSelect(filter);
				}
			} else {
				handleValueSelect(suggestion);
			}
		},
		[suggestionMode, filters, handleFilterSelect, handleValueSelect],
	);

	// Remove a token
	const handleRemoveToken = useCallback(
		(tokenKey: string) => {
			onTokensChange(tokens.filter((t) => t.key !== tokenKey));
			inputRef.current?.focus();
		},
		[tokens, onTokensChange],
	);

	// Handle keyboard events
	const handleKeyDown = useCallback(
		(e: KeyboardEvent<HTMLInputElement>) => {
			// Backspace on empty input removes last token
			if (e.key === "Backspace" && inputValue === "" && tokens.length > 0) {
				const lastToken = tokens[tokens.length - 1];
				handleRemoveToken(lastToken.key);
				return;
			}

			// Tab to autocomplete current suggestion
			if (e.key === "Tab" && suggestions.length > 0 && isOpen) {
				e.preventDefault();
				handleSuggestionSelect(suggestions[0]);
				return;
			}

			// Escape to close suggestions
			if (e.key === "Escape") {
				setIsOpen(false);
				return;
			}
		},
		[inputValue, tokens, suggestions, isOpen, handleRemoveToken, handleSuggestionSelect],
	);

	// Handle input change
	const handleInputChange = useCallback((value: string) => {
		setInputValue(value);
		setIsOpen(true);
	}, []);

	// Handle focus
	const handleFocus = useCallback(() => {
		setIsOpen(true);
	}, []);

	// Handle blur (with delay to allow click on suggestions)
	const handleBlur = useCallback(() => {
		setTimeout(() => {
			setIsOpen(false);
		}, 150);
	}, []);

	return (
		<div className={cn("relative w-full", className)} ref={containerRef}>
			{/* Main search container */}
			<div
				className={cn(
					"flex items-center rounded-md border border-solid border-border bg-surface-primary",
					"focus-within:border-border-hover",
					"transition-colors",
				)}
				onClick={() => inputRef.current?.focus()}
			>
				{/* Left section: search icon + tokens + input */}
				<div className="flex items-center gap-2 flex-1 px-3 py-2">
					<Search className="h-4 w-4 shrink-0 text-content-secondary" />

					{/* Tokens */}
					{tokens.map((token) => (
						<Badge
							key={token.key}
							variant="default"
							className="gap-1.5 pr-1.5 font-normal bg-surface-secondary text-content-primary text-sm"
						>
							<span>{token.key}:{token.value}</span>
							<button
								type="button"
								className="rounded-sm hover:bg-surface-tertiary transition-colors p-0.5"
								onClick={(e) => {
									e.stopPropagation();
									handleRemoveToken(token.key);
								}}
								aria-label={`Remove ${token.key} filter`}
							>
								<X className="h-3 w-3" />
							</button>
						</Badge>
					))}

					{/* Input */}
					<input
						ref={inputRef}
						value={inputValue}
						onChange={(e) => handleInputChange(e.target.value)}
						onKeyDown={handleKeyDown}
						onFocus={handleFocus}
						onBlur={handleBlur}
						placeholder={tokens.length === 0 ? placeholder : ""}
						className="flex-1 min-w-[100px] bg-transparent border-none outline-none text-sm text-content-primary placeholder:text-content-secondary"
						aria-label="Search with filters"
					/>
				</div>

				{/* Right section: Filter dropdown button */}
				<div className="border-l border-solid border-border">
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<button
								type="button"
								className={cn(
									"flex items-center gap-2 px-3 py-2 h-full",
									"text-content-secondary hover:text-content-primary hover:bg-surface-secondary",
									"transition-colors text-sm",
								)}
							>
								<SlidersHorizontal className="h-4 w-4" />
								<span>Filter</span>
								<ChevronDown className="h-3 w-3" />
							</button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end" className="min-w-[140px]">
							{filters.map((filter) => (
								<DropdownMenuItem
									key={filter.key}
									onClick={() => handleFilterSelect(filter)}
									className="text-sm"
								>
									{filter.label}
								</DropdownMenuItem>
							))}
						</DropdownMenuContent>
					</DropdownMenu>
				</div>
			</div>

			{/* Suggestions dropdown - full width below search box */}
			{isOpen && suggestions.length > 0 && suggestionMode === "values" && (
				<div
					className={cn(
						"absolute left-0 right-0 top-full mt-1 z-50",
						"rounded-md border border-solid border-border bg-surface-primary shadow-lg",
						"overflow-hidden",
					)}
				>
					<Command shouldFilter={false}>
						<CommandList className="max-h-[280px] overflow-y-auto py-2">
							{suggestions.map((suggestion) => (
								<CommandItem
									key={suggestion.value}
									onSelect={() => handleSuggestionSelect(suggestion)}
									className="cursor-pointer px-4 py-2.5 text-sm text-content-primary hover:bg-surface-secondary mx-0 rounded-none"
								>
									{suggestion.icon}
									<span>{suggestion.label}</span>
								</CommandItem>
							))}
						</CommandList>
					</Command>
				</div>
			)}
		</div>
	);
};

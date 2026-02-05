import { Command as CommandPrimitive } from "cmdk";
import { Badge } from "components/Badge/Badge";
import {
	Command,
	CommandEmpty,
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
		}, 200);
	}, []);

	// Get the filter label for a token
	const getFilterLabel = useCallback(
		(key: string) => {
			const filter = filters.find((f) => f.key === key);
			return filter?.label || key;
		},
		[filters],
	);

	return (
		<div className={cn("relative w-full", className)}>
			<div
				className={cn(
					"flex items-center gap-2 px-3 py-2 rounded-lg border border-border bg-surface-primary",
					"focus-within:ring-2 focus-within:ring-ring focus-within:border-transparent",
					"transition-all",
				)}
				onClick={() => inputRef.current?.focus()}
			>
				<Search className="h-4 w-4 shrink-0 text-content-secondary" />

				{/* Active tokens */}
				<div className="flex flex-wrap items-center gap-1.5 flex-1">
					{tokens.map((token) => (
						<Badge
							key={token.key}
							variant="default"
							className="gap-1 pr-1 font-normal"
						>
							<span className="text-content-secondary">
								{getFilterLabel(token.key)}:
							</span>
							<span>{token.label}</span>
							<button
								type="button"
								className="ml-0.5 rounded-full p-0.5 hover:bg-surface-tertiary transition-colors"
								onClick={(e) => {
									e.stopPropagation();
									handleRemoveToken(token.key);
								}}
								aria-label={`Remove ${getFilterLabel(token.key)} filter`}
							>
								<X className="h-3 w-3" />
							</button>
						</Badge>
					))}

					{/* Input */}
					<CommandPrimitive
						className="flex-1 min-w-[120px]"
						shouldFilter={false}
					>
						<CommandPrimitive.Input
							ref={inputRef}
							value={inputValue}
							onValueChange={handleInputChange}
							onKeyDown={handleKeyDown}
							onFocus={handleFocus}
							onBlur={handleBlur}
							placeholder={tokens.length === 0 ? placeholder : "Add filter..."}
							className="w-full bg-transparent border-none outline-none text-sm text-content-primary placeholder:text-content-secondary"
							aria-label="Search with filters"
						/>
					</CommandPrimitive>
				</div>

				{/* Filter dropdown button */}
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<button
							type="button"
							className={cn(
								"flex items-center gap-1 px-2 py-1 rounded text-sm",
								"text-content-secondary hover:text-content-primary hover:bg-surface-secondary",
								"transition-colors",
							)}
						>
							<SlidersHorizontal className="h-4 w-4" />
							<span>Filter</span>
							<ChevronDown className="h-3 w-3" />
						</button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end" className="w-48">
						{filters.map((filter) => (
							<DropdownMenuItem
								key={filter.key}
								onClick={() => handleFilterSelect(filter)}
							>
								{filter.label}
							</DropdownMenuItem>
						))}
					</DropdownMenuContent>
				</DropdownMenu>
			</div>

			{/* Suggestions dropdown */}
			{isOpen && suggestions.length > 0 && (
				<div
					className={cn(
						"absolute left-0 right-0 top-full mt-1 z-50",
						"rounded-lg border border-border bg-surface-primary shadow-lg",
						"overflow-hidden",
					)}
				>
					<Command shouldFilter={false}>
						<CommandList className="max-h-64">
							<CommandGroup
								heading={
									suggestionMode === "filters"
										? "Filter by"
										: `${getFilterLabel(activeFilterKey || "")} options`
								}
							>
								{suggestions.map((suggestion) => (
									<CommandItem
										key={suggestion.value}
										onSelect={() => handleSuggestionSelect(suggestion)}
										className="cursor-pointer"
									>
										{suggestion.icon}
										<span>{suggestion.label}</span>
										{suggestionMode === "filters" && (
											<span className="ml-auto text-xs text-content-secondary">
												{suggestion.value}:
											</span>
										)}
									</CommandItem>
								))}
							</CommandGroup>
						</CommandList>
					</Command>
				</div>
			)}
		</div>
	);
};

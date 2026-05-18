import { CheckIcon, FilterIcon, SearchIcon, XIcon } from "lucide-react";
import { type FC, useEffect, useRef } from "react";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { cn } from "#/utils/cn";

type ArchivedFilter = "active" | "archived";

interface FilterDropdownProps {
	readonly archivedFilter: ArchivedFilter;
	readonly onArchivedFilterChange?: (filter: ArchivedFilter) => void;
}

export const FilterDropdown: FC<FilterDropdownProps> = ({
	archivedFilter,
	onArchivedFilterChange,
}) => (
	<DropdownMenu>
		<DropdownMenuTrigger asChild>
			<Button
				variant="subtle"
				size="icon"
				aria-label="Filter agents"
				className={cn(
					"h-7 w-7 min-w-0 justify-end rounded-none px-0 text-content-secondary hover:text-content-primary",
					archivedFilter === "archived" && "text-content-primary",
				)}
			>
				<FilterIcon />
			</Button>
		</DropdownMenuTrigger>
		<DropdownMenuContent
			align="end"
			className="mobile-full-width-dropdown mobile-full-width-dropdown-top-below-header [&_[role=menuitem]]:text-[13px]"
		>
			<DropdownMenuItem onSelect={() => onArchivedFilterChange?.("active")}>
				Active
				{archivedFilter === "active" && (
					<CheckIcon className="ml-auto h-3.5 w-3.5" />
				)}
			</DropdownMenuItem>
			<DropdownMenuItem onSelect={() => onArchivedFilterChange?.("archived")}>
				Archived
				{archivedFilter === "archived" && (
					<CheckIcon className="ml-auto h-3.5 w-3.5" />
				)}
			</DropdownMenuItem>
		</DropdownMenuContent>
	</DropdownMenu>
);

interface SearchBarProps {
	readonly isOpen: boolean;
	readonly onToggle: () => void;
	readonly searchQuery: string;
	readonly onSearchChange: (query: string) => void;
	readonly resultCount: number;
	readonly totalCount: number;
}

export const SearchBar: FC<SearchBarProps> = ({
	isOpen,
	onToggle,
	searchQuery,
	onSearchChange,
	resultCount,
	totalCount,
}) => {
	const inputRef = useRef<HTMLInputElement>(null);

	useEffect(() => {
		if (isOpen) {
			// Small delay to allow animation to start before focusing.
			const id = window.setTimeout(() => inputRef.current?.focus(), 50);
			return () => window.clearTimeout(id);
		}
	}, [isOpen]);

	useEffect(() => {
		if (!isOpen) return;
		const handler = (e: KeyboardEvent) => {
			if (e.key === "Escape") {
				onSearchChange("");
				onToggle();
			}
		};
		document.addEventListener("keydown", handler);
		return () => document.removeEventListener("keydown", handler);
	}, [isOpen, onToggle, onSearchChange]);

	if (!isOpen) {
		return (
			<Button
				variant="subtle"
				size="icon"
				aria-label="Search agents"
				className="h-7 w-7 min-w-0 rounded-none px-0 text-content-secondary hover:text-content-primary"
				onClick={onToggle}
			>
				<SearchIcon />
			</Button>
		);
	}

	return (
		<div className="flex w-full flex-col gap-1">
			<div className="flex items-center gap-1">
				<div className="flex min-w-0 flex-1 items-center gap-1.5 rounded-md border border-solid border-border-default bg-surface-primary px-2 py-1 focus-within:ring-2 focus-within:ring-content-link">
					<SearchIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
					<input
						ref={inputRef}
						type="text"
						value={searchQuery}
						onChange={(e) => onSearchChange(e.target.value)}
						placeholder="Search agents..."
						aria-label="Search agents"
						className="min-w-0 flex-1 border-none bg-transparent p-0 text-[13px] text-content-primary outline-none placeholder:text-content-secondary"
					/>
					{searchQuery && (
						<button
							type="button"
							aria-label="Clear search"
							className="flex h-4 w-4 shrink-0 cursor-pointer items-center justify-center rounded-sm border-none bg-transparent p-0 text-content-secondary hover:text-content-primary"
							onClick={() => onSearchChange("")}
						>
							<XIcon className="h-3 w-3" />
						</button>
					)}
				</div>
			</div>
			{searchQuery && (
				<span className="text-[11px] text-content-secondary">
					<strong>{resultCount}</strong> of {totalCount} agents
				</span>
			)}
		</div>
	);
};

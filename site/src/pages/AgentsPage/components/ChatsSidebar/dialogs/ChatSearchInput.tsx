import { ListFilterIcon, SearchIcon, XIcon } from "lucide-react";
import type {
	ChangeEventHandler,
	FC,
	KeyboardEventHandler,
	RefObject,
} from "react";
import { cn } from "#/utils/cn";

export type SearchFilter = {
	readonly key: string;
	readonly value: string | null;
};

type ChatSearchInputProps = {
	readonly activeResultId: string | undefined;
	readonly hasResults: boolean;
	readonly inputRef: RefObject<HTMLInputElement | null>;
	readonly listboxId: string;
	readonly filters: readonly SearchFilter[];
	readonly value: string;
	readonly onChange: ChangeEventHandler<HTMLInputElement>;
	readonly onKeyDown: KeyboardEventHandler<HTMLInputElement>;
	readonly onRemoveFilter: (key: string) => void;
	readonly isDropdownOpen: boolean;
	readonly onToggleDropdown: () => void;
};

export const ChatSearchInput: FC<ChatSearchInputProps> = ({
	activeResultId,
	hasResults,
	inputRef,
	listboxId,
	filters,
	value,
	onChange,
	onKeyDown,
	onRemoveFilter,
	isDropdownOpen,
	onToggleDropdown,
}) => {
	const completedFilters = filters.filter((f) => f.value !== null);
	const incompleteFilter = filters.find((f) => f.value === null);

	return (
		<div
			className={cn(
				"flex min-h-10 w-full items-center gap-1.5 rounded-md border border-solid border-border-default bg-surface-primary px-3",
				"focus-within:ring-2 focus-within:ring-content-link",
			)}
		>
			<SearchIcon className="size-4 shrink-0 text-content-secondary" />
			{completedFilters.map((f) => (
				<span
					key={f.key}
					className="inline-flex shrink-0 items-center gap-1 rounded-md border border-solid border-border bg-surface-secondary px-2 py-0.5 text-xs text-content-secondary"
				>
					<span>
						{f.key}:{f.value}
					</span>
					<button
						type="button"
						onClick={(e) => {
							e.stopPropagation();
							onRemoveFilter(f.key);
						}}
						className="inline-flex cursor-pointer items-center border-none bg-transparent p-0 text-content-secondary hover:text-content-primary"
						aria-label={`Remove ${f.key} filter`}
					>
						<XIcon className="size-3" />
					</button>
				</span>
			))}
			{incompleteFilter && (
				<span className="inline-flex shrink-0 items-center rounded-md border border-dashed border-border bg-surface-secondary px-2 py-0.5 text-xs text-content-secondary">
					{incompleteFilter.key}:
				</span>
			)}
			<input
				ref={inputRef}
				value={value}
				onChange={onChange}
				onKeyDown={onKeyDown}
				placeholder={filters.length > 0 ? "" : "Search chats..."}
				className="min-w-[60px] flex-1 border-none bg-transparent py-2 text-sm text-content-primary outline-none placeholder:text-content-disabled"
				aria-label="Search chats"
				role="combobox"
				aria-controls={hasResults ? listboxId : undefined}
				aria-expanded={hasResults}
				aria-haspopup="listbox"
				aria-activedescendant={activeResultId}
			/>
			<button
				type="button"
				onClick={onToggleDropdown}
				className={cn(
					"inline-flex shrink-0 cursor-pointer items-center border-none bg-transparent p-0 text-content-secondary hover:text-content-primary",
					isDropdownOpen && "text-content-primary",
				)}
				aria-label="Toggle filters"
				aria-expanded={isDropdownOpen}
			>
				<ListFilterIcon className="size-4" />
			</button>
		</div>
	);
};

import { CheckIcon, FilterIcon } from "lucide-react";
import type { FC } from "react";
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
					"size-7 min-w-0 justify-end rounded-none px-0 text-content-secondary hover:text-content-primary",
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
					<CheckIcon className="ml-auto size-3.5" />
				)}
			</DropdownMenuItem>
			<DropdownMenuItem onSelect={() => onArchivedFilterChange?.("archived")}>
				Archived
				{archivedFilter === "archived" && (
					<CheckIcon className="ml-auto size-3.5" />
				)}
			</DropdownMenuItem>
		</DropdownMenuContent>
	</DropdownMenu>
);

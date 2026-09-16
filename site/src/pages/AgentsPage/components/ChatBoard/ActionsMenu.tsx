import { EllipsisIcon, Trash2Icon } from "lucide-react";
import type { FC } from "react";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";

interface ActionsMenuProps {
	readonly label: string;
	readonly onDelete: { readonly label: string; readonly run: () => void };
}

/**
 * Destructive actions only; renaming and coloring happen in place. A hover
 * "..." with a reserved slot, so revealing it never moves the line it sits
 * on. Revealed by the `group/column` hover group.
 */
export const ActionsMenu: FC<ActionsMenuProps> = ({ label, onDelete }) => (
	<DropdownMenu>
		<DropdownMenuTrigger asChild>
			<button
				type="button"
				aria-label={`Actions for ${label}`}
				className="grid size-4 shrink-0 place-items-center rounded border-0 bg-transparent p-0 text-content-secondary/70 opacity-0 hover:bg-content-primary/5 hover:text-content-primary focus-visible:opacity-100 group-hover/column:opacity-100 data-[state=open]:opacity-100"
				onPointerDown={(e) => e.stopPropagation()}
			>
				<EllipsisIcon className="size-3.5" />
			</button>
		</DropdownMenuTrigger>
		<DropdownMenuContent align="end" className="min-w-40 text-xs">
			<DropdownMenuItem
				className="text-content-destructive"
				onSelect={onDelete.run}
			>
				<Trash2Icon className="size-3.5" />
				{onDelete.label}
			</DropdownMenuItem>
		</DropdownMenuContent>
	</DropdownMenu>
);

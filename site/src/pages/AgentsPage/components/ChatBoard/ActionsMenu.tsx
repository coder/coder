import { cn } from "cn";
import { EllipsisIcon, PencilIcon, Trash2Icon } from "lucide-react";
import type { FC } from "react";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { CARD_COLORS, type CardColor } from "./boardLabels";

const SWATCH_CLASS: Record<CardColor, string> = {
	green: "bg-highlight-green",
	orange: "bg-highlight-orange",
	sky: "bg-highlight-sky",
	red: "bg-highlight-red",
	purple: "bg-highlight-purple",
	magenta: "bg-highlight-magenta",
};

interface ActionsMenuProps {
	readonly label: string;
	/** Tailwind group whose hover reveals the trigger. */
	readonly revealOn: "card" | "row" | "column";
	readonly onRename: () => void;
	readonly color?: {
		readonly value: CardColor | undefined;
		readonly onChange: (color: CardColor | undefined) => void;
	};
	readonly onDelete?: { readonly label: string; readonly run: () => void };
}

/**
 * The one place a card, row, or column exposes its actions: a hover "..."
 * with a reserved slot, so revealing it never moves the line it sits on.
 */
export const ActionsMenu: FC<ActionsMenuProps> = ({
	label,
	revealOn,
	onRename,
	color,
	onDelete,
}) => (
	<DropdownMenu>
		<DropdownMenuTrigger asChild>
			<button
				type="button"
				aria-label={`Actions for ${label}`}
				className={cn(
					"grid size-4 shrink-0 place-items-center rounded border-0 bg-transparent p-0 text-content-secondary/70 opacity-0 hover:bg-content-primary/5 hover:text-content-primary focus-visible:opacity-100 data-[state=open]:opacity-100",
					revealOn === "card" && "group-hover/card:opacity-100",
					revealOn === "row" && "group-hover/row:opacity-100",
					revealOn === "column" && "group-hover/column:opacity-100",
				)}
				onPointerDown={(e) => e.stopPropagation()}
			>
				<EllipsisIcon className="size-3.5" />
			</button>
		</DropdownMenuTrigger>
		<DropdownMenuContent align="end" className="min-w-40 text-xs">
			<DropdownMenuItem onSelect={onRename}>
				<PencilIcon className="size-3.5" />
				Rename
			</DropdownMenuItem>
			{color && (
				<>
					<DropdownMenuSeparator />
					<div className="flex items-center gap-1.5 px-2 py-1.5">
						<Swatch
							label="No color"
							selected={color.value === undefined}
							className="bg-surface-secondary"
							onClick={() => color.onChange(undefined)}
						/>
						{CARD_COLORS.map((name) => (
							<Swatch
								key={name}
								label={name}
								selected={color.value === name}
								className={SWATCH_CLASS[name]}
								onClick={() => color.onChange(name)}
							/>
						))}
					</div>
				</>
			)}
			{onDelete && (
				<>
					<DropdownMenuSeparator />
					<DropdownMenuItem
						className="text-content-destructive"
						onSelect={onDelete.run}
					>
						<Trash2Icon className="size-3.5" />
						{onDelete.label}
					</DropdownMenuItem>
				</>
			)}
		</DropdownMenuContent>
	</DropdownMenu>
);

const Swatch: FC<{
	readonly label: string;
	readonly selected: boolean;
	readonly className: string;
	readonly onClick: () => void;
}> = ({ label, selected, className, onClick }) => (
	<button
		type="button"
		aria-label={label}
		aria-pressed={selected}
		className={cn(
			"size-5 rounded-md border border-border",
			className,
			selected && "ring-2 ring-content-link ring-offset-1",
		)}
		onClick={onClick}
	/>
);

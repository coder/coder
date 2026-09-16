import { cn } from "cn";
import {
	EllipsisIcon,
	EllipsisVerticalIcon,
	type LucideIcon,
} from "lucide-react";
import type { FC } from "react";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";

interface MenuAction {
	readonly label: string;
	readonly icon: LucideIcon;
	readonly onSelect: () => void;
	readonly destructive?: boolean;
}

interface ActionsMenuProps {
	readonly label: string;
	readonly items: readonly MenuAction[];
	/**
	 * Cards show a permanent vertical "⋮" so nothing shifts on hover;
	 * columns reveal a horizontal "..." from the `group/column` hover group.
	 */
	readonly permanent?: boolean;
}

/** The menu for actions that have no in-place gesture. */
export const ActionsMenu: FC<ActionsMenuProps> = ({
	label,
	items,
	permanent = false,
}) => {
	const Icon = permanent ? EllipsisVerticalIcon : EllipsisIcon;
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<button
					type="button"
					aria-label={`Actions for ${label}`}
					className={cn(
						"relative z-[1] grid size-4 shrink-0 place-items-center rounded border-0 bg-transparent p-0 text-content-secondary/60 hover:text-content-primary focus-visible:opacity-100 data-[state=open]:text-content-primary",
						!permanent &&
							"opacity-0 group-hover/column:opacity-100 data-[state=open]:opacity-100",
					)}
					onPointerDown={(e) => e.stopPropagation()}
				>
					<Icon className="size-3.5" />
				</button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end" className="min-w-40 text-xs">
				{items.map((item) => (
					<DropdownMenuItem
						key={item.label}
						className={cn(item.destructive && "text-content-destructive")}
						onSelect={item.onSelect}
					>
						<item.icon className="size-3.5" />
						{item.label}
					</DropdownMenuItem>
				))}
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

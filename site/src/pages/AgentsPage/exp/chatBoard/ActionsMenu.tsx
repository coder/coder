import { cn } from "cn";
import {
	EllipsisIcon,
	EllipsisVerticalIcon,
	type LucideIcon,
} from "lucide-react";
import type { FC, ReactNode } from "react";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSub,
	DropdownMenuSubContent,
	DropdownMenuSubTrigger,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";

/** An item runs an action, or opens a sub menu holding `children`. */
type MenuAction = Readonly<{ label: string; icon: LucideIcon }> &
	(
		| Readonly<{ onSelect: () => void; destructive?: boolean }>
		| Readonly<{ children: ReactNode }>
	);

type ActionsMenuProps = {
	readonly label: string;
	readonly items: readonly MenuAction[];
	/**
	 * Cards show a permanent vertical "⋮" so nothing shifts on hover;
	 * columns reveal a horizontal "..." from the `group/column` hover group.
	 */
	readonly permanent?: boolean;
};

/** The menu for actions that have no in-place gesture. */
export const ActionsMenu: React.FC<ActionsMenuProps> = ({
	label,
	items,
	permanent = false,
}) => {
	const Icon = permanent ? EllipsisVerticalIcon : EllipsisIcon;
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					aria-label={`Actions for ${label}`}
					// Card and column headers are drag handles: the press must not start
					// a drag, and z-[1] keeps it above a card's open-chat surface.
					onPointerDown={(e) => e.stopPropagation()}
					className={cn(
						"relative z-[1] size-4 min-w-0 rounded p-0 text-content-secondary/60 [&>svg]:size-3.5! [&>svg]:p-0 focus-visible:opacity-100 data-[state=open]:text-content-primary",
						!permanent &&
							"opacity-0 group-hover/column:opacity-100 data-[state=open]:opacity-100",
					)}
				>
					<Icon />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end" className="min-w-40 text-xs">
				{items.map((item) =>
					"children" in item ? (
						<DropdownMenuSub key={item.label}>
							<DropdownMenuSubTrigger>
								<item.icon className="size-3.5" />
								{item.label}
							</DropdownMenuSubTrigger>
							<DropdownMenuSubContent className="min-w-44 text-xs">
								{item.children}
							</DropdownMenuSubContent>
						</DropdownMenuSub>
					) : (
						<DropdownMenuItem
							key={item.label}
							className={cn(item.destructive && "text-content-destructive")}
							onSelect={item.onSelect}
						>
							<item.icon className="size-3.5" />
							{item.label}
						</DropdownMenuItem>
					),
				)}
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

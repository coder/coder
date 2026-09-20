import type { FC, ReactNode } from "react";
import { Badge } from "#/components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type DimensionBadgeItem = {
	key: string;
	label: string;
	icon: ReactNode;
};

type DimensionBadgeProps = {
	items: readonly DimensionBadgeItem[];
	/** Plural label for the count badge, such as "providers". */
	noun: string;
};

/**
 * One item renders as a labeled badge; several collapse into a count badge
 * whose tooltip lists them.
 */
export const DimensionBadge: FC<DimensionBadgeProps> = ({ items, noun }) => {
	if (items.length === 0) {
		return null;
	}
	if (items.length > 1) {
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<Badge asChild hover className="max-w-full">
						<button
							type="button"
							// Opening the list must not also open the clickable session row.
							onClick={(event) => event.stopPropagation()}
						>
							{items.length} {noun}
						</button>
					</Badge>
				</TooltipTrigger>
				<TooltipContent side="top" align="start">
					<ul className="m-0 flex list-none flex-col gap-1 p-0">
						{items.map((item) => (
							<li key={item.key} className="flex items-center gap-1.5">
								{item.icon}
								{item.label}
							</li>
						))}
					</ul>
				</TooltipContent>
			</Tooltip>
		);
	}
	const [item] = items;
	return (
		<Badge className="gap-1.5 max-w-full">
			<div className="shrink-0 flex items-center">{item.icon}</div>
			<span className="truncate min-w-0" title={item.label}>
				{item.label}
			</span>
		</Badge>
	);
};

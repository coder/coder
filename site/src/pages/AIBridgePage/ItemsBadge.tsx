import { cn } from "cn";
import type { FC, ReactNode } from "react";
import { Badge } from "#/components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type ItemsBadgeItem = {
	key: string;
	label: string;
	icon: ReactNode;
};

type ItemsBadgeProps = {
	items: readonly ItemsBadgeItem[];
	/** Plural label for the count badge, such as "providers". */
	noun: string;
	/** Heading above the tooltip list; when set, list rows render compact. */
	tooltipHeading?: { title: string; subtitle?: string };
	/** Caps how many items the tooltip lists. The badge still counts all. */
	maxTooltipItems?: number;
};

/**
 * One item renders as a labeled badge; several collapse into a count badge
 * whose tooltip lists them.
 */
export const ItemsBadge: FC<ItemsBadgeProps> = ({
	items,
	noun,
	tooltipHeading,
	maxTooltipItems,
}) => {
	if (items.length === 0) {
		return null;
	}
	if (items.length > 1) {
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<Badge asChild hover className="max-w-full">
						{/* Radix opens tooltips on hover or focus only, so a click or tap
						must keep reaching the clickable session row. */}
						<button type="button">
							{items.length} {noun}
						</button>
					</Badge>
				</TooltipTrigger>
				<TooltipContent
					side="top"
					align="start"
					collisionPadding={16}
					// Long lists scroll inside the viewport instead of overflowing it.
					className="max-h-(--radix-popper-available-height) overflow-y-auto"
					// The portal still bubbles React clicks up to the session row.
					onClick={(event) => event.stopPropagation()}
				>
					{tooltipHeading && (
						<div className="mb-1.5">
							<div className="text-sm text-content-primary">
								{tooltipHeading.title}
							</div>
							{tooltipHeading.subtitle && (
								<div className="text-xs text-content-secondary">
									{tooltipHeading.subtitle}
								</div>
							)}
						</div>
					)}
					<ul className="m-0 flex list-none flex-col gap-1 p-0">
						{items.slice(0, maxTooltipItems).map((item) => (
							<li
								key={item.key}
								className={cn(
									"flex items-center gap-1.5",
									tooltipHeading && "text-xs text-content-secondary",
								)}
							>
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

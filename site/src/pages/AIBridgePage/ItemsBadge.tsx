import { Badge } from "#/components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type ItemsBadgeItem = {
	key: string;
	label: string;
	icon: React.ReactNode;
};

type ItemsBadgeProps = {
	items: readonly ItemsBadgeItem[];
	/** Plural label for the count badge, such as "providers". */
	noun: string;
};

/**
 * One item renders as a labeled badge; several collapse into a count badge
 * whose tooltip lists them. The tooltip is interactive so a long list can be
 * scrolled from the keyboard; a click or tap pins it.
 */
export const ItemsBadge: React.FC<ItemsBadgeProps> = ({ items, noun }) => {
	if (items.length === 0) {
		return null;
	}
	if (items.length > 1) {
		return (
			<Tooltip interactive>
				<TooltipTrigger className="max-w-full rounded-md">
					<Badge asChild hover className="max-w-full">
						<span>
							{items.length} {noun}
						</span>
					</Badge>
				</TooltipTrigger>
				<TooltipContent side="top" align="start" collisionPadding={16}>
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

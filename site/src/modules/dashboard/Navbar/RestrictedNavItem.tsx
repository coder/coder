import type { FC, ReactNode } from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";

// TODO(PLAT-460): placeholder copy, replace once the final upgrade message is
// available.
export const restrictedNavTooltip =
	"Workspaces are not available for your account. Contact your administrator to request access.";

type RestrictedNavItemProps = {
	className?: string;
	/**
	 * Dimmed labels are not links, so they carry no keyboard affordance of their
	 * own. Keeping the wrapper focusable makes the tooltip reachable without a
	 * pointer. Set to -1 inside menus that already manage focus on the parent
	 * item.
	 */
	tabIndex?: number;
	children: ReactNode;
};

/**
 * A navigation label rendered as dimmed, non-interactive text with a tooltip
 * describing why it cannot be opened.
 */
export const RestrictedNavItem: FC<RestrictedNavItemProps> = ({
	className,
	tabIndex = 0,
	children,
}) => {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<span
					aria-disabled="true"
					tabIndex={tabIndex}
					className={cn("opacity-50 cursor-default", className)}
				>
					{children}
				</span>
			</TooltipTrigger>
			<TooltipContent>{restrictedNavTooltip}</TooltipContent>
		</Tooltip>
	);
};

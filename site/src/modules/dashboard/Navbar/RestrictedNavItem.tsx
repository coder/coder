import type { FC, ReactNode } from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";

// TODO(PLAT-460): placeholder copy, replace once the final messages are
// available.
export const restrictedNavTooltips = {
	workspaces:
		"Workspaces are not available for your account. Contact your administrator to request access.",
	templates:
		"Templates are not available for your account. Contact your administrator to request access.",
	tasks:
		"Tasks are not available for your account. Contact your administrator to request access.",
	agents:
		"Agents need permission to create workspaces. Contact your administrator to request access.",
} as const;

type RestrictedNavItemProps = {
	className?: string;
	message: string;
	children: ReactNode;
};

/**
 * A dimmed, inert navigation label. The message is rendered as screen reader
 * text and shown in a tooltip on hover or focus.
 *
 * The label is a button so that it is focusable and announced as unavailable.
 * It performs no action. Callers supply the dimmed text color, so the focus
 * ring stays at full strength.
 */
export const RestrictedNavItem: FC<RestrictedNavItemProps> = ({
	className,
	message,
	children,
}) => {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<button
					type="button"
					aria-disabled="true"
					className={cn(
						"bg-transparent border-0 p-0 font-inherit cursor-default",
						"focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link",
						className,
					)}
				>
					<span>{children}</span>
					<span className="sr-only"> {message}</span>
				</button>
			</TooltipTrigger>
			<TooltipContent>{message}</TooltipContent>
		</Tooltip>
	);
};

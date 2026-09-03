import type { FC } from "react";
import { Link, NavLink } from "react-router";
import { useSidebarContext } from "#/components/Sidebar/SidebarContext";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";

interface SidebarTopLevelNavItemProps {
	label: string;
	href: string;
	icon: FC<{ className?: string }>;
	active: boolean;
	/** Match the route exactly instead of by prefix (NavLink end). */
	end?: boolean;
}

/**
 * A flat icon+label link for collapsible settings sidebars. In
 * collapsed mode it renders as an icon with a tooltip; clicking it
 * navigates and re-expands the sidebar.
 */
export const SidebarTopLevelNavItem: FC<SidebarTopLevelNavItemProps> = ({
	label,
	href,
	icon: Icon,
	active,
	end,
}) => {
	const { collapsed, expand } = useSidebarContext();

	if (collapsed) {
		return (
			<TooltipProvider>
				<Tooltip delayDuration={0}>
					<TooltipTrigger asChild>
						<Link
							to={href}
							onClick={expand}
							className="flex items-center justify-center w-10 h-10 rounded-md no-underline hover:bg-surface-secondary"
						>
							<Icon
								className={cn(
									"size-4 shrink-0 text-content-secondary",
									active && "text-content-primary",
								)}
							/>
						</Link>
					</TooltipTrigger>
					<TooltipContent side="right">{label}</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		);
	}

	return (
		<NavLink
			to={href}
			end={end}
			className={({ isActive }) =>
				cn(
					"flex items-center gap-2 -mx-3 pl-4 pr-3 h-10 rounded-md no-underline text-sm font-medium text-content-secondary hover:bg-surface-secondary transition-colors",
					isActive && "text-content-primary font-semibold",
				)
			}
		>
			<Icon className="size-4 shrink-0" />
			{label}
		</NavLink>
	);
};

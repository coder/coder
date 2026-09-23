import { cn } from "cn";
import type { FC, ReactNode } from "react";
import { NavLink } from "react-router";

interface SidebarNavLinkProps {
	href: string;
	children: ReactNode;
	/** Match the route exactly instead of by prefix. */
	end?: boolean;
}

/**
 * Leaf link inside a SidebarGroup, hanging off its connecting line.
 * Active links use the primary text color, a semibold weight, and mark
 * their spot on the line; hovered links lift to the primary text color.
 */
export const SidebarNavLink: FC<SidebarNavLinkProps> = ({
	href,
	children,
	end,
}) => {
	return (
		<NavLink
			to={href}
			end={end}
			className={({ isActive }) =>
				cn(
					// Rows keep 8px of inner padding, start 4px right of the
					// connecting line so the hover surface never touches it, and
					// stop 8px short of the sidebar edge.
					"relative flex items-center h-8 px-2 -mr-1 text-sm rounded-md font-medium text-content-secondary no-underline hover:bg-surface-secondary hover:text-content-primary transition-colors",
					// The marker is a short 2px bar over the connecting line, 5px
					// left of the row (4px gap plus the 1px line), centered on the
					// row.
					isActive &&
						"font-semibold text-content-primary before:absolute before:-left-[5px] before:top-1/2 before:h-5 before:w-0.5 before:-translate-y-1/2 before:rounded-full before:bg-content-primary",
				)
			}
		>
			{children}
		</NavLink>
	);
};

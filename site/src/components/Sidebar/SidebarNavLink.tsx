import { cn } from "cn";
import type { FC, ReactNode } from "react";
import { NavLink } from "react-router";

type SidebarNavLinkProps = {
	href: string;
	children: ReactNode;
	/** Match the route exactly instead of by prefix. */
	end?: boolean;
};

/** Link inside a SidebarGroup. The active link is marked on the group's line. */
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
					"relative flex items-center h-8 px-2 -mr-1 text-sm rounded-md font-medium text-content-secondary no-underline hover:bg-surface-secondary hover:text-content-primary transition-colors",
					// `before:` draws the active marker over the group's line.
					isActive &&
						"font-semibold text-content-primary before:absolute before:-left-[5px] before:top-1/2 before:h-5 before:w-0.5 before:-translate-y-1/2 before:rounded-full before:bg-content-primary",
				)
			}
		>
			{children}
		</NavLink>
	);
};

import type { FC, ReactNode } from "react";
import { Link, NavLink } from "react-router";
import { cn } from "#/utils/cn";

interface SidebarNavLinkProps {
	href: string;
	children: ReactNode;
	/** Match the route exactly instead of by prefix. */
	end?: boolean;
	/**
	 * Items under a nested accordion use tighter 32px rows; items directly
	 * under an icon section keep 40px rows.
	 */
	nested?: boolean;
	/** Overrides NavLink matching for pages reachable from several URLs. */
	activeOverride?: boolean;
}

/**
 * Leaf link for the collapsible settings sidebars. Active links use the
 * primary text color and a semibold weight.
 */
export const SidebarNavLink: FC<SidebarNavLinkProps> = ({
	href,
	children,
	end,
	nested = false,
	activeOverride,
}) => {
	const sizeClass = nested ? "h-8 px-2" : "h-10 px-2 -mx-2";
	const baseClass = cn(
		"flex items-center rounded-md text-sm font-medium text-content-secondary no-underline hover:bg-surface-secondary transition-colors",
		sizeClass,
	);
	const activeClass = "font-semibold text-content-primary";

	if (activeOverride !== undefined) {
		return (
			<Link
				to={href}
				aria-current={activeOverride ? "page" : undefined}
				className={cn(baseClass, activeOverride && activeClass)}
			>
				{children}
			</Link>
		);
	}

	return (
		<NavLink
			to={href}
			end={end}
			className={({ isActive }) => cn(baseClass, isActive && activeClass)}
		>
			{children}
		</NavLink>
	);
};

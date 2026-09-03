import type { FC, ReactNode } from "react";
import { NavLink } from "react-router";
import { cn } from "#/utils/cn";

interface SidebarProps {
	children?: ReactNode;
	className?: string;
}

export const Sidebar: FC<SidebarProps> = ({ className, children }) => {
	return (
		<nav className={cn("w-full lg:w-60 shrink-0", className)}>{children}</nav>
	);
};

interface SettingsSidebarNavItemProps {
	children?: ReactNode;
	href: string;
	end?: boolean;
}

export const SettingsSidebarNavItem: FC<SettingsSidebarNavItemProps> = ({
	children,
	href,
	end,
}) => {
	return (
		<NavLink
			end={end}
			to={href}
			className={({ isActive }) =>
				cn(
					"relative text-sm text-content-secondary no-underline font-medium py-2 px-3 hover:bg-surface-secondary rounded-md transition ease-in-out duration-150",
					isActive && "font-semibold text-content-primary",
				)
			}
		>
			{children}
		</NavLink>
	);
};

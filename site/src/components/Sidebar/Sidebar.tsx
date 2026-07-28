import type { LucideIcon } from "lucide-react";
import { Children, type FC, isValidElement, type ReactNode } from "react";
import { Link, matchPath, NavLink, useLocation } from "react-router";
import { cn } from "#/utils/cn";

interface SidebarProps {
	children?: ReactNode;
	className?: string;
}

export const Sidebar: FC<SidebarProps> = ({ className, children }) => {
	return (
		<nav className={cn("w-full lg:w-60 flex-shrink-0", className)}>
			{children}
		</nav>
	);
};

interface SidebarHeaderProps {
	avatar: ReactNode;
	title: ReactNode;
	subtitle: ReactNode;
	linkTo?: string;
}

const titleStyles = {
	normal:
		"text-semibold overflow-hidden whitespace-nowrap text-content-primary",
};

export const SidebarHeader: FC<SidebarHeaderProps> = ({
	avatar,
	title,
	subtitle,
	linkTo,
}) => {
	return (
		<div className="flex flex-row gap-2 mb-4">
			{avatar}
			<div className="overflow-hidden flex flex-col">
				{linkTo ? (
					<Link className={cn(titleStyles.normal, "no-underline")} to={linkTo}>
						{title}
					</Link>
				) : (
					<span className={titleStyles.normal}>{title}</span>
				)}
				<span className="text-content-secondary text-sm overflow-hidden overflow-ellipsis">
					{subtitle}
				</span>
			</div>
		</div>
	);
};

interface SidebarNavItemProps {
	children?: ReactNode;
	href: string;
	end?: boolean;
	icon?: LucideIcon;
}

export const SidebarNavItem: FC<SidebarNavItemProps> = ({
	children,
	href,
	end,
	icon: Icon,
}) => {
	return (
		<NavLink
			end={end}
			to={href}
			className={({ isActive }) =>
				cn(
					"relative flex items-center gap-2 text-sm text-content-secondary no-underline font-medium py-2 px-3 hover:bg-surface-secondary rounded-md transition ease-in-out duration-150",
					isActive && "font-semibold text-content-primary",
				)
			}
		>
			{Icon && <Icon className="size-4 flex-shrink-0" />}
			{children}
		</NavLink>
	);
};

interface SidebarGroupProps {
	icon: LucideIcon;
	label: string;
	/** Section overview route the header navigates to. */
	href: string;
	children?: ReactNode;
}

const collectNavHrefs = (children: ReactNode): string[] => {
	return Children.toArray(children).flatMap((child) => {
		if (!isValidElement(child)) {
			return [];
		}
		const { href } = child.props as { href?: unknown };
		return typeof href === "string" ? [href] : [];
	});
};

/**
 * Always-open settings nav group: icon + label header linking to the
 * section overview, with indented `SidebarNavItem` children. The header
 * highlights when the overview or any child route is active.
 */
export const SidebarGroup: FC<SidebarGroupProps> = ({
	icon: Icon,
	label,
	children,
	href,
}) => {
	const location = useLocation();
	const isActive = [href, ...collectNavHrefs(children)].some(
		(path) => matchPath({ path, end: true }, location.pathname) != null,
	);

	return (
		<div className="flex flex-col gap-1">
			<NavLink
				end
				to={href}
				className={cn(
					"flex items-center gap-2 px-3 py-2 h-10 rounded-md text-content-secondary no-underline hover:bg-surface-secondary transition ease-in-out duration-150",
					isActive && "font-semibold text-content-primary",
				)}
			>
				<Icon className="size-4 flex-shrink-0" />
				<span className="text-sm font-medium whitespace-nowrap">{label}</span>
			</NavLink>
			<div className="flex flex-col gap-1 pl-6 ml-0.5">{children}</div>
		</div>
	);
};

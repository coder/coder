import { Button } from "components/Button/Button";
import { Stack } from "components/Stack/Stack";
import { ChevronDownIcon } from "lucide-react";
import { type ElementType, type FC, type ReactNode, useState } from "react";
import { Link, NavLink, useLocation } from "react-router";
import { cn } from "utils/cn";

interface SidebarProps {
	title: string;
	children?: ReactNode;
	className?: string;
}

export const Sidebar: FC<SidebarProps> = ({ title, className, children }) => {
	const location = useLocation();

	return (
		<>
			{/* Mobile collapsible sidebar. Keyed on pathname so
			    state resets (menu closes) on navigation. */}
			<SidebarMobileNavigation title={title} key={location.pathname}>
				{children}
			</SidebarMobileNavigation>

			{/* Desktop sticky sidebar */}
			<nav
				className={cn(
					"hidden md:block w-60 flex-shrink-0 sticky top-[72px] h-[calc(100vh-72px)] py-4",
					className,
				)}
			>
				<div className="max-h-screen overflow-y-auto">{children}</div>
			</nav>
		</>
	);
};

const SidebarMobileNavigation: FC<{ title: string; children: ReactNode }> = ({
	title,
	children,
}) => {
	const [open, setOpen] = useState(false);

	return (
		<div className="md:hidden w-full">
			<Button
				variant="outline"
				aria-label={open ? "Close menu" : "Open menu"}
				onClick={() => setOpen((prev) => !prev)}
				className="justify-between gap-2 w-full"
			>
				{title}
				<ChevronDownIcon />
			</Button>
			{open && <nav className="py-4">{children}</nav>}
		</div>
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
		<Stack direction="row" spacing={1} className="mb-4">
			{avatar}
			<div
				css={{
					overflow: "hidden",
					display: "flex",
					flexDirection: "column",
				}}
			>
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
		</Stack>
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

interface SidebarNavItemProps {
	children?: ReactNode;
	icon: ElementType;
	href: string;
}

export const SidebarNavItem: FC<SidebarNavItemProps> = ({
	children,
	href,
	icon: Icon,
}) => {
	return (
		<NavLink
			end
			to={href}
			className={({ isActive }) =>
				cn(
					"block relative text-sm text-inherit mb-px p-3 pl-4 rounded-sm",
					"transition-colors no-underline hover:bg-surface-secondary",
					isActive &&
						"bg-surface-secondary border-0 border-solid border-l-[3px] border-highlight-sky",
				)
			}
		>
			<Stack alignItems="center" spacing={1.5} direction="row">
				<Icon className="size-4" />
				{children}
			</Stack>
		</NavLink>
	);
};

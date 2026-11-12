import { cn } from "cn";
import { Link, type LinkProps } from "react-router";

export const Sidebar: React.FC<React.ComponentProps<"nav">> = ({
	children,
	...attrs
}) => {
	return (
		<nav
			className={cn("w-64 shrink-0 border-r", "h-full py-2 overflow-y-auto")}
			{...attrs}
		>
			{children}
		</nav>
	);
};

type SidebarLinkProps = LinkProps & {
	active: boolean;
};

export const SidebarLink: React.FC<SidebarLinkProps> = ({
	children,
	active,
	...attrs
}) => {
	return (
		<Link
			aria-current={active ? "page" : undefined}
			className={cn(
				"block py-2.5 px-6 text-sm no-underline hover:bg-surface-tertiary hover:text-content-primary",
				active
					? "text-content-primary pointer-events-none bg-surface-secondary"
					: "text-content-secondary",
			)}
			{...attrs}
		>
			{children}
		</Link>
	);
};

export const SidebarItemSkeleton: React.FC<React.PropsWithChildren> = ({
	children,
}) => {
	return <div className="py-2.5 px-6 text-sm">{children}</div>;
};

export const SidebarCaption: React.FC<React.ComponentProps<"div">> = ({
	children,
	className,
	...attrs
}) => {
	return (
		<div
			className={cn(
				"text-[10px] uppercase font-medium text-content-secondary",
				"px-6 py-3 tracking-[0.5px]",
			)}
			{...attrs}
		>
			{children}
		</div>
	);
};

import { cn } from "cn";

export const Sidebar: React.FC<React.ComponentProps<"nav">> = ({
	children,
	...attrs
}) => {
	return (
		<nav
			className={cn(
				"w-64 shrink-0 border-solid border-0 border-r",
				"h-full py-2 overflow-y-auto",
			)}
			{...attrs}
		>
			{children}
		</nav>
	);
};

type SidebarItemProps = React.ComponentProps<"button"> & {
	active?: boolean;
};

export const SidebarItem: React.FC<SidebarItemProps> = ({
	children,
	active,
	...attrs
}) => {
	return (
		<button
			className={cn(
				"py-2.5 px-6 border-0 text-sm w-full text-left cursor-pointer",
				"hover:bg-surface-tertiary hover:text-content-primary",
				active
					? "text-content-primary pointer-events-none bg-surface-secondary"
					: "text-content-secondary pointer-events-auto bg-transparent",
			)}
			{...attrs}
		>
			{children}
		</button>
	);
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

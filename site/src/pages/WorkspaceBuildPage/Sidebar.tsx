import type { FC, HTMLAttributes } from "react";
import { cn } from "utils/cn";
export const Sidebar: FC<HTMLAttributes<HTMLElement>> = ({
	children,
	...attrs
}) => {
	return (
		<nav
			className="w-64 flex-shrink-0 border-0 border-r border-solid border-zinc-700 h-full overflow-y-auto"
			{...attrs}
		>
			{children}
		</nav>
	);
};

interface SidebarItemProps extends HTMLAttributes<HTMLElement> {
	active?: boolean;
}

export const SidebarItem: FC<SidebarItemProps> = ({
	children,
	active,
	...attrs
}) => {
	return (
		<button
			className={cn(
				"bg-transparent border-none text-sm w-full text-left",
				"cursor-pointer py-2.5 px-6",
				"hover:bg-surface-secondary hover:text-content-primary",
				active && "none text-content-primary",
				!active && "pointer-events-auto text-content-secondary",
			)}
			{...attrs}
		>
			{children}
		</button>
	);
};

export const SidebarCaption: FC<HTMLAttributes<HTMLDivElement>> = ({
	children,
	...attrs
}) => {
	return (
		<div
			className="text-[10px] uppercase font-medium text-content-secondary tracking-widest py-3 px-4"
			{...attrs}
		>
			{children}
		</div>
	);
};

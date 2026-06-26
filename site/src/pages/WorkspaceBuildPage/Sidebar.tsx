import type { FC, HTMLAttributes } from "react";
import { cn } from "#/utils/cn";

export const Sidebar: FC<HTMLAttributes<HTMLElement>> = ({
	children,
	...attrs
}) => {
	return (
		<nav
			className={cn(
				"w-64 flex-shrink-0 border-solid border-0 border-r",
				"h-full py-2 overflow-y-auto",
			)}
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

export const SidebarCaption: FC<HTMLAttributes<HTMLDivElement>> = ({
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

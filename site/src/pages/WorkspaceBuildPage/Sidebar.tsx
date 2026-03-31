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
			css={(theme) => ({
				background: active ? theme.experimental.l2.background : "none",
				border: "none",
				fontSize: 14,
				width: "100%",
				textAlign: "left",
				padding: "0 24px",
				cursor: "pointer",
				pointerEvents: active ? "none" : "auto",
				color: active
					? theme.palette.text.primary
					: theme.palette.text.secondary,
				"&:hover": {
					background: theme.palette.action.hover,
					color: theme.palette.text.primary,
				},
				paddingTop: 10,
				paddingBottom: 10,
			})}
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
			css={(theme) => ({
				fontSize: 10,
				textTransform: "uppercase",
				fontWeight: 500,
				color: theme.palette.text.secondary,
				padding: "12px 24px",
				letterSpacing: "0.5px",
			})}
			{...attrs}
		>
			{children}
		</div>
	);
};

import type { ComponentProps, FC, HTMLAttributes } from "react";
import { Link, type LinkProps } from "react-router";
import { cn } from "utils/cn";
import { TopbarIconButton } from "./Topbar";

export const Sidebar: FC<HTMLAttributes<HTMLDivElement>> = (props) => {
	return (
		<div
			// TODO: Remove extra border classes once MUI is removed
			className="flex flex-col gap-px w-64 border-solid border-0 border-r border-r-border h-full py-2 shrink-0 overflow-y-auto"
			{...props}
		/>
	);
};

export const SidebarLink: FC<LinkProps> = ({ className, ...props }) => {
	return (
		<Link
			className={cn(
				"text-[13px] text-content-primary py-2 px-4 text-left bg-transparent hover:divide-surface-tertiary cursor-pointer border-0 no-underline",
				className,
			)}
			{...props}
		/>
	);
};

interface SidebarItemProps extends HTMLAttributes<HTMLButtonElement> {
	isActive?: boolean;
}

export const SidebarItem: FC<SidebarItemProps> = ({
	isActive,
	className,
	...buttonProps
}) => {
	return (
		<button
			className={cn(
				"text-[13px] text-content-primary py-2 px-4 text-left bg-transparent hover:divide-surface-tertiary opacity-75 hover:opacity-100 cursor-pointer border-0",
				isActive && "opacity-100 bg-surface-tertiary",
				className,
			)}
			{...buttonProps}
		/>
	);
};

export const SidebarCaption: FC<HTMLAttributes<HTMLSpanElement>> = (props) => {
	return (
		<span
			className="text-[10px] leading-tight py-3 px-4 uppercase font-medium text-content-primary tracking-widest"
			{...props}
		/>
	);
};

interface SidebarIconButton extends ComponentProps<typeof TopbarIconButton> {
	isActive: boolean;
}

export const SidebarIconButton: FC<SidebarIconButton> = ({
	isActive,
	className,
	...buttonProps
}) => {
	return (
		<TopbarIconButton
			{...buttonProps}
			className={cn(
				"border-0 border-x-2 border-x-transparent border-solid",
				!isActive && "opacity-75 hover:opacity-100",
				isActive && "opacity-100 relative border-l-sky-400",
				className,
			)}
		/>
	);
};

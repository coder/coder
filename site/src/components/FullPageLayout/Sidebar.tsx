import { cn } from "cn";
import { Link, type LinkProps } from "react-router";
import { TopbarIconButton } from "./Topbar";

export const Sidebar: React.FC<React.ComponentProps<"div">> = (props) => {
	return (
		<div
			className="flex flex-col gap-px w-64 border-r border-r-border h-full py-2 shrink-0 overflow-y-auto"
			{...props}
		/>
	);
};

export const SidebarLink: React.FC<LinkProps> = ({ className, ...props }) => {
	return (
		<Link
			className={cn(
				"text-sm text-content-primary py-2 px-4 text-left bg-transparent hover:divide-surface-tertiary cursor-pointer no-underline",
				className,
			)}
			{...props}
		/>
	);
};

type SidebarItemProps = React.ComponentProps<"button"> & {
	isActive?: boolean;
};

export const SidebarItem: React.FC<SidebarItemProps> = ({
	isActive,
	className,
	...buttonProps
}) => {
	return (
		<button
			className={cn(
				"text-sm text-content-secondary font-medium py-2 pl-3.5 pr-4 text-left bg-transparent cursor-pointer",
				"border-l-2 border-l-transparent",
				"hover:bg-surface-secondary transition ease-in-out duration-150",
				isActive &&
					"font-semibold text-content-primary bg-surface-tertiary hover:bg-surface-tertiary border-l-content-primary",
				className,
			)}
			{...buttonProps}
		/>
	);
};

export const SidebarCaption: React.FC<React.ComponentProps<"span">> = (
	props,
) => {
	return (
		<span
			className="text-[10px] leading-tight py-3 px-4 uppercase font-medium text-content-primary tracking-widest"
			{...props}
		/>
	);
};

type SidebarIconButtonProps = {
	isActive: boolean;
} & React.ComponentProps<typeof TopbarIconButton>;

export const SidebarIconButton: React.FC<SidebarIconButtonProps> = ({
	isActive,
	className,
	...buttonProps
}) => {
	return (
		<TopbarIconButton
			className={cn(
				"opacity-75 hover:opacity-100 border-x-2 border-x-transparent border-solid",
				isActive && "opacity-100 relative border-l-border-pending",
				className,
			)}
			{...buttonProps}
		/>
	);
};

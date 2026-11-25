import { cn } from "cn";
import type React from "react";

type PageHeaderProps = {
	actions?: React.ReactNode;
	className?: string;
	children?: React.ReactNode;
};

export const PageHeader: React.FC<PageHeaderProps> = ({
	children,
	actions,
	className,
}) => {
	return (
		<header
			className={cn(
				"flex items-start flex-col md:flex-row md:items-center",
				"py-12 gap-8",
				className,
			)}
			data-testid="header"
		>
			<hgroup className="flex flex-col gap-2">{children}</hgroup>
			{actions && (
				<div className="flex items-center gap-2 ml-[initial] md:ml-auto w-full md:w-auto">
					{actions}
				</div>
			)}
		</header>
	);
};

type PageHeaderTitleProps = React.ComponentProps<"h1"> & {
	/** Rendered beside the heading so it stays out of the heading's name. */
	tooltip?: React.ReactNode;
};

export const PageHeaderTitle: React.FC<PageHeaderTitleProps> = ({
	children,
	className,
	tooltip,
	...props
}) => {
	const title = (
		<h1
			className={cn(
				"text-3xl font-semibold m-0 flex items-center leading-snug",
				className,
			)}
			{...props}
		>
			{children}
		</h1>
	);
	if (!tooltip) {
		return title;
	}
	return (
		<div className="flex items-center gap-2">
			{title}
			{tooltip}
		</div>
	);
};

type PageHeaderSubtitleProps = React.ComponentProps<"p">;

export const PageHeaderSubtitle: React.FC<PageHeaderSubtitleProps> = ({
	children,
	className,
	...props
}) => {
	return (
		<p
			className={cn(
				"text-sm text-content-secondary font-normal block m-0 leading-snug",
				className,
			)}
			{...props}
		>
			{children}
		</p>
	);
};

type PageHeaderCaptionProps = React.ComponentProps<"span">;

export const PageHeaderCaption: React.FC<PageHeaderCaptionProps> = ({
	children,
	className,
	...props
}) => {
	return (
		<span
			className={cn(
				"text-sm text-content-secondary font-medium uppercase tracking-widest",
				className,
			)}
			{...props}
		>
			{children}
		</span>
	);
};

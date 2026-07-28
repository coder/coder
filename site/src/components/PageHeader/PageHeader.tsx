import type React from "react";
import type { FC, ReactNode } from "react";
import { cn } from "#/utils/cn";

interface PageHeaderProps {
	actions?: ReactNode;
	className?: string;
	children?: ReactNode;
}

export const PageHeader: FC<PageHeaderProps> = ({
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

type PageHeaderTitleProps = React.ComponentPropsWithRef<"h1">;

export const PageHeaderTitle: FC<PageHeaderTitleProps> = ({
	children,
	className,
	...props
}) => {
	return (
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
};

type PageHeaderSubtitleProps = React.ComponentPropsWithRef<"h2">;

export const PageHeaderSubtitle: FC<PageHeaderSubtitleProps> = ({
	children,
	className,
	...props
}) => {
	return (
		<h2
			className={cn(
				"text-sm text-content-secondary font-normal block m-0 leading-snug",
				className,
			)}
			{...props}
		>
			{children}
		</h2>
	);
};

type PageHeaderCaptionProps = React.ComponentPropsWithRef<"span">;

export const PageHeaderCaption: FC<PageHeaderCaptionProps> = ({
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

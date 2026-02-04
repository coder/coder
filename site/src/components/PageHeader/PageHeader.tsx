import type { FC, PropsWithChildren, ReactNode } from "react";
import { cn } from "utils/cn";

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
				"flex flex-start flex-col md:flex-row md:items-center gap-8",
				"py-12 gap-8",
				className,
			)}
			data-testid="header"
		>
			<hgroup className="flex flex-col gap-2">{children}</hgroup>
			{actions && (
				<div className="flex ml-[initial] md:ml-auto w-full md:w-auto">
					{actions}
				</div>
			)}
		</header>
	);
};

export const PageHeaderTitle: FC<PropsWithChildren> = ({ children }) => {
	return (
		<h1
			className={cn(
				"text-3xl font-semibold m-0 flex items-center leading-snug",
			)}
		>
			{children}
		</h1>
	);
};

interface PageHeaderSubtitleProps {
	children?: ReactNode;
	condensed?: boolean;
}

export const PageHeaderSubtitle: FC<PageHeaderSubtitleProps> = ({
	children,
}) => {
	return (
		<h2
			className={cn(
				"text-sm text-content-secondary font-normal block m-0 leading-snug",
			)}
		>
			{children}
		</h2>
	);
};

export const PageHeaderCaption: FC<PropsWithChildren> = ({ children }) => {
	return (
		<span className="text-sm text-content-secondary font-medium uppercase tracking-widest">
			{children}
		</span>
	);
};

interface ResourcePageHeaderProps extends Omit<PageHeaderProps, "children"> {
	displayName?: string;
	name: string;
}

export const ResourcePageHeader: FC<ResourcePageHeaderProps> = ({
	displayName,
	name,
	...props
}) => {
	const title = displayName || name;

	return (
		<PageHeader {...props}>
			<PageHeaderTitle>{title}</PageHeaderTitle>
			{name !== title && <PageHeaderSubtitle>{name}</PageHeaderSubtitle>}
		</PageHeader>
	);
};

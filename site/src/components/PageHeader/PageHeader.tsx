import type { FC, PropsWithChildren, ReactNode } from "react";
import { cn } from "utils/cn";
import { Stack } from "../Stack/Stack";

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
				"flex flex-col lg:flex-row items-start lg:items-center py-12 gap-8",
				className,
			)}
			data-testid="header"
		>
			<hgroup>{children}</hgroup>
			{actions && (
				<Stack direction="row" className="lg:ml-auto">
					{actions}
				</Stack>
			)}
		</header>
	);
};

export const PageHeaderTitle: FC<PropsWithChildren> = ({ children }) => {
	return (
		<h1 className="text-2xl font-normal m-0 flex items-center leading-[1.4]">
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
	condensed,
}) => {
	return (
		<h2
			className={cn(
				"text-base font-normal block mb-0 text-content-secondary leading-[1.4]",
				condensed && "mt-1",
				!condensed && "mt-2",
			)}
		>
			{children}
		</h2>
	);
};

export const PageHeaderCaption: FC<PropsWithChildren> = ({ children }) => {
	return (
		<span className="text-xs text-content-secondary font-semibold uppercase tracking-[0.1em]">
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

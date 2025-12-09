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
			css={(theme) => ({
				[theme.breakpoints.down("md")]: {
					flexDirection: "column",
					alignItems: "flex-start",
				},
			})}
			className={cn("flex items-center py-12 gap-8", className)}
			data-testid="header"
		>
			<hgroup>{children}</hgroup>
			{actions && (
				<Stack
					direction="row"
					css={(theme) => ({
						marginLeft: "auto",

						[theme.breakpoints.down("md")]: {
							marginLeft: "initial",
							width: "100%",
						},
					})}
				>
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
			css={(theme) => ({
				color: theme.palette.text.secondary,
				marginTop: condensed ? 4 : 8,
			})}
			className="text-base font-normal block mb-0 leading-[1.4]"
		>
			{children}
		</h2>
	);
};

export const PageHeaderCaption: FC<PropsWithChildren> = ({ children }) => {
	return (
		<span
			css={(theme) => ({
				fontSize: 12,
				color: theme.palette.text.secondary,
				fontWeight: 600,
				textTransform: "uppercase",
				letterSpacing: "0.1em",
			})}
		>
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

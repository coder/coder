import { type CSSObject, useTheme } from "@emotion/react";
import type { FC, PropsWithChildren, ReactNode } from "react";
import { cn } from "utils/cn";

interface FullWidthPageHeaderProps {
	children?: ReactNode;
	sticky?: boolean;
}

export const FullWidthPageHeader: FC<FullWidthPageHeaderProps> = ({
	children,
	sticky = true,
}) => {
	const theme = useTheme();
	return (
		<header
			data-testid="header"
			css={[
				{
					...(theme.typography.body2 as CSSObject),
					background: theme.palette.background.default,
					borderBottom: `1px solid ${theme.palette.divider}`,
					[theme.breakpoints.down("lg")]: {
						position: "unset",
						alignItems: "flex-start",
					},
					[theme.breakpoints.down("md")]: {
						flexDirection: "column",
					},
				},
			]}
			className={cn(
				"p-6 flex items-center gap-12 z-10 flex-wrap",
				sticky && "sticky top-0",
			)}
		>
			{children}
		</header>
	);
};

const _PageHeaderActions: FC<PropsWithChildren> = ({ children }) => {
	const theme = useTheme();
	return (
		<div
			css={{
				marginLeft: "auto",
				[theme.breakpoints.down("md")]: {
					marginLeft: "unset",
				},
			}}
		>
			{children}
		</div>
	);
};

export const PageHeaderTitle: FC<PropsWithChildren> = ({ children }) => {
	return <h1 className="text-lg leading-6 font-medium m-0">{children}</h1>;
};

export const PageHeaderSubtitle: FC<PropsWithChildren> = ({ children }) => {
	const theme = useTheme();
	return (
		<span
			css={{
				color: theme.palette.text.secondary,
			}}
			className="text-sm leading-none block"
		>
			{children}
		</span>
	);
};

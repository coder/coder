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
			className={cn("p-6 flex gap-12 z-10 flex-wrap", sticky && "sticky top-0")}
		>
			{children}
		</header>
	);
};

const _PageHeaderActions: FC<PropsWithChildren> = ({ children }) => {
	return <div className="ml-[unset] md:ml-auto">{children}</div>;
};

export const PageHeaderTitle: FC<PropsWithChildren> = ({ children }) => {
	return <h1 className="text-lg leading-6 font-medium m-0">{children}</h1>;
};

export const PageHeaderSubtitle: FC<PropsWithChildren> = ({ children }) => {
	return (
		<span className="text-sm text-content-secondary leading-[22.4px] block">
			{children}
		</span>
	);
};

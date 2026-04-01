import type { FC, PropsWithChildren, ReactNode } from "react";
import { cn } from "#/utils/cn";

interface FullWidthPageHeaderProps {
	children?: ReactNode;
	sticky?: boolean;
}

export const FullWidthPageHeader: FC<FullWidthPageHeaderProps> = ({
	children,
	sticky = true,
}) => {
	return (
		<header
			data-testid="header"
			className={cn(
				"bg-surface-primary border-0 border-b border-solid border-border",
				"text-sm p-6 flex items-center gap-12 flex-wrap z-10",
				"lg:items-center flex-col lg:flex-row",
				sticky && "sticky top-0",
			)}
		>
			{children}
		</header>
	);
};

const _PageHeaderActions: FC<PropsWithChildren> = ({ children }) => {
	return <div className="ml-auto md:ml-0">{children}</div>;
};

export const PageHeaderTitle: FC<PropsWithChildren> = ({ children }) => {
	return <h1 className="text-lg font-medium m-0 leading-6">{children}</h1>;
};

export const PageHeaderSubtitle: FC<PropsWithChildren> = ({ children }) => {
	return (
		<span className="text-sm text-content-secondary block">{children}</span>
	);
};

import { cn } from "cn";

type FullWidthPageHeaderProps = {
	children?: React.ReactNode;
	sticky?: boolean;
};

export const FullWidthPageHeader: React.FC<FullWidthPageHeaderProps> = ({
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

const _PageHeaderActions: React.FC<React.PropsWithChildren> = ({
	children,
}) => {
	return <div className="ml-auto md:ml-0">{children}</div>;
};

export const PageHeaderTitle: React.FC<React.PropsWithChildren> = ({
	children,
}) => {
	return <h1 className="text-lg font-medium m-0 leading-6">{children}</h1>;
};

export const PageHeaderSubtitle: React.FC<React.PropsWithChildren> = ({
	children,
}) => {
	return (
		<span className="text-sm text-content-secondary block">{children}</span>
	);
};

import { createContext, type FC, type HTMLAttributes, useContext } from "react";
import { Link, type LinkProps } from "react-router";
import { cn } from "utils/cn";

// Keeping this for now because of a workaround in WorkspaceBUildPageView
export const TAB_PADDING_X = 16;

type TabsContextValue = {
	active: string;
};

const TabsContext = createContext<TabsContextValue | undefined>(undefined);

type TabsProps = HTMLAttributes<HTMLDivElement> & TabsContextValue;

export const Tabs: FC<TabsProps> = ({ className, active, ...htmlProps }) => {
	return (
		<TabsContext.Provider value={{ active }}>
			<div
				// Because the Tailwind preflight is not used, its necessary to set border style to solid and
				// reset all border widths to 0 https://tailwindcss.com/docs/border-width#using-without-preflight
				className={cn(
					"border-0 border-b border-solid border-border",
					className,
				)}
				{...htmlProps}
			/>
		</TabsContext.Provider>
	);
};

type TabsListProps = HTMLAttributes<HTMLDivElement>;

export const TabsList: FC<TabsListProps> = ({ className, ...props }) => {
	return (
		<div
			role="tablist"
			className={cn("flex items-baseline gap-6", className)}
			{...props}
		/>
	);
};

type TabLinkProps = LinkProps & {
	value: string;
};

export const TabLink: FC<TabLinkProps> = ({
	value,
	className,
	...linkProps
}) => {
	const tabsContext = useContext(TabsContext);
	if (!tabsContext) {
		throw new Error("Tab only can be used inside of Tabs");
	}

	const isActive = tabsContext.active === value;

	return (
		<Link
			{...linkProps}
			className={cn(
				`text-sm text-content-secondary no-underline font-medium py-3 px-1 hover:text-content-primary rounded-md
				focus-visible:ring-offset-1 focus-visible:ring-offset-surface-primary
				focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link focus-visible:rounded-sm`,
				{
					"text-content-primary relative before:absolute before:bg-surface-invert-primary before:left-0 before:w-full before:h-px before:-bottom-px before:content-['']":
						isActive,
				},
				className,
			)}
		/>
	);
};

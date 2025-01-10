import { type FC, type HTMLAttributes, createContext, useContext } from "react";
import { Link, type LinkProps } from "react-router-dom";
import { cn } from "utils/cn";

export const TAB_PADDING_Y = 12;
export const TAB_PADDING_X = 16;

type TabsContextValue = {
	active: string;
};

const TabsContext = createContext<TabsContextValue | undefined>(undefined);

type TabsProps = HTMLAttributes<HTMLDivElement> & TabsContextValue;

export const Tabs: FC<TabsProps> = ({ active, ...htmlProps }) => {
	return (
		<TabsContext.Provider value={{ active }}>
			<div
				className="border-b border-solid border-border border-t-0 border-l-0 border-r-0"
				{...htmlProps}
			/>
		</TabsContext.Provider>
	);
};

type TabsListProps = HTMLAttributes<HTMLDivElement>;

export const TabsList: FC<TabsListProps> = (props) => {
	return <div role="tablist" className="flex items-baseline" {...props} />;
};

type TabLinkProps = LinkProps & {
	value: string;
};

export const TabLink: FC<TabLinkProps> = ({ value, ...linkProps }) => {
	const tabsContext = useContext(TabsContext);

	if (!tabsContext) {
		throw new Error("Tab only can be used inside of Tabs");
	}

	const isActive = tabsContext.active === value;

	return (
		<Link
			{...linkProps}
			className={cn(
				"text-sm text-content-secondary no-underline font-medium py-2 px-3 hover:text-content-primary rounded-md",
				{
					"text-content-primary relative before:absolute before:bg-surface-invert-primary before:left-0 before:w-full before:h-px before:-bottom-px before:content-['']":
						isActive,
				},
			)}
		/>
	);
};

import {
	createContext,
	type FC,
	type HTMLAttributes,
	useCallback,
	useContext,
	useEffect,
	useRef,
	useState,
} from "react";
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
	const listRef = useRef<HTMLDivElement | null>(null);
	const [indicatorStyle, setIndicatorStyle] = useState({
		left: 0,
		width: 0,
	});
	const [hasActiveTab, setHasActiveTab] = useState(false);

	const updateIndicator = useCallback(() => {
		if (!listRef.current) return;

		const activeTab = listRef.current.querySelector<HTMLElement>(
			"[data-active='true']",
		);
		if (!activeTab) {
			setHasActiveTab(false);
			return;
		}

		const listRect = listRef.current.getBoundingClientRect();
		const activeRect = activeTab.getBoundingClientRect();

		requestAnimationFrame(() => {
			setIndicatorStyle({
				left: activeRect.left - listRect.left,
				width: activeRect.width,
			});
			setHasActiveTab(true);
		});
	}, []);

	useEffect(() => {
		const timeoutId = setTimeout(updateIndicator, 0);

		window.addEventListener("resize", updateIndicator);
		const observer = new MutationObserver(updateIndicator);

		if (listRef.current) {
			observer.observe(listRef.current, {
				attributes: true,
				childList: true,
				subtree: true,
			});
		}

		return () => {
			clearTimeout(timeoutId);
			window.removeEventListener("resize", updateIndicator);
			observer.disconnect();
		};
	}, [updateIndicator]);

	return (
		<div ref={listRef} className="relative">
			<div
				role="tablist"
				className={cn("flex items-baseline gap-6", className)}
				{...props}
			/>
			<div
				className={cn(
					"absolute bottom-0 h-px bg-surface-invert-primary transition-all duration-200 ease-in-out",
					!hasActiveTab && "opacity-0",
				)}
				style={indicatorStyle}
			/>
		</div>
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
			data-active={isActive}
			{...linkProps}
			className={cn(
				"text-sm text-content-secondary no-underline font-medium py-3 px-1 hover:text-content-primary rounded-md",
				"focus-visible:ring-offset-1 focus-visible:ring-offset-surface-primary",
				"focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link focus-visible:rounded-sm",
				isActive ? "text-content-primary" : "",
				className,
			)}
		/>
	);
};

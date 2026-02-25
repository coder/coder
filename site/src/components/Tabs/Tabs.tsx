import {
	createContext,
	type FC,
	type HTMLAttributes,
	useCallback,
	useContext,
	useEffect,
	useLayoutEffect,
	useRef,
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
	const tabsContext = useContext(TabsContext);
	const listRef = useRef<HTMLDivElement>(null);
	const indicatorRef = useRef<HTMLDivElement>(null);
	const hasInitialized = useRef(false);

	const updateIndicator = useCallback((animate: boolean) => {
		const list = listRef.current;
		const indicator = indicatorRef.current;
		if (!list || !indicator) return;

		const activeTab = list.querySelector<HTMLElement>("[data-active='true']");
		if (!activeTab) {
			indicator.style.opacity = "0";
			return;
		}

		const listRect = list.getBoundingClientRect();
		const activeRect = activeTab.getBoundingClientRect();

		if (!animate) {
			indicator.style.transition = "none";
		}

		indicator.style.left = `${activeRect.left - listRect.left}px`;
		indicator.style.width = `${activeRect.width}px`;
		indicator.style.opacity = "1";

		if (!animate) {
			// Force a reflow so the position applies before
			// restoring the transition property.
			void indicator.offsetHeight;
			indicator.style.transition = "";
		}
	}, []);

	// Measure synchronously before paint so the indicator is
	// positioned correctly on the first frame. Animate only on
	// subsequent active-tab changes.
	const active = tabsContext?.active;
	useLayoutEffect(() => {
		// Re-run whenever the active tab changes.
		void active;
		updateIndicator(hasInitialized.current);
		hasInitialized.current = true;
	}, [active, updateIndicator]);

	// Reposition without animation on window resize.
	useEffect(() => {
		const handleResize = () => updateIndicator(false);
		window.addEventListener("resize", handleResize);
		return () => window.removeEventListener("resize", handleResize);
	}, [updateIndicator]);

	return (
		<div ref={listRef} className="relative">
			<div
				role="tablist"
				className={cn("flex items-baseline gap-6", className)}
				{...props}
			/>
			<div
				ref={indicatorRef}
				className="absolute bottom-0 h-px bg-surface-invert-primary opacity-0 transition-all duration-300 ease-in-out"
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

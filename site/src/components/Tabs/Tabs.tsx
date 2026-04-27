import { cva, type VariantProps } from "class-variance-authority";
import { Tabs as TabsPrimitive } from "radix-ui";
import {
	type ComponentProps,
	createContext,
	type HTMLAttributes,
	useCallback,
	useContext,
	useEffect,
	useLayoutEffect,
	useRef,
} from "react";
import { Link, type LinkProps } from "react-router";
import { cn } from "#/utils/cn";

// --- Radix tabs (stateful panels) ---

type TabsProps = ComponentProps<typeof TabsPrimitive.Root>;

export const Tabs = ({ ...props }: TabsProps) => {
	return <TabsPrimitive.Root data-slot="tabs" {...props} />;
};

const tabsListVariants = cva("flex flex-wrap items-center", {
	variants: {
		variant: {
			insideBox: cn(
				"border-solid border-x-0 border-y",
				"[&_[data-slot=tabs-trigger][data-state=active]]:bg-surface-secondary",
				"[&_[data-slot=tabs-trigger]]:border-x [&_[data-slot=tabs-trigger]]:border-y-0 [&_[data-slot=tabs-trigger]]:border-solid",
				"[&_[data-slot=tabs-trigger]]:border-x-transparent [&_[data-slot=tabs-trigger][data-state=active]]:border-x-border",
				"[&_[data-slot=tabs-trigger]]:px-4",
				"[&_[data-slot=tabs-trigger]]:text-content-secondary",
				"[&_[data-slot=tabs-trigger][data-state=active]]:text-content-primary",
			),
			outsideBox: cn(
				"border-solid border-0 border-b gap-6",
				"[&_[data-slot=tabs-trigger]]:text-content-secondary [&_[data-slot=tabs-trigger][data-state=active]]:text-content-primary",
				"[&_[data-slot=tabs-trigger]]:border-0 [&_[data-slot=tabs-trigger]]:border-y [&_[data-slot=tabs-trigger]]:border-solid",
				"[&_[data-slot=tabs-trigger]]:border-transparent [&_[data-slot=tabs-trigger][data-state=active]]:border-b-white",
				"[&_[data-slot=tabs-trigger]:hover]:text-content-primary",
				"[&_[data-slot=tabs-trigger]]:px-1",
			),
		},
	},
	defaultVariants: {
		variant: "outsideBox",
	},
});
type TabsListProps = ComponentProps<typeof TabsPrimitive.List> &
	VariantProps<typeof tabsListVariants> & {
		overflowKebabMenu?: boolean;
	};

export const TabsList = ({
	className,
	variant,
	overflowKebabMenu = false,
	ref,
	...props
}: TabsListProps) => {
	return (
		<TabsPrimitive.List
			ref={ref}
			data-slot="tabs-list"
			className={cn(
				tabsListVariants({ variant }),
				overflowKebabMenu && "min-w-0 w-full max-w-full flex-nowrap",
				className,
			)}
			{...props}
		/>
	);
};

type TabsTriggerProps = ComponentProps<typeof TabsPrimitive.Trigger>;

export const TabsTrigger = ({
	type: triggerType = "button",
	...props
}: TabsTriggerProps) => {
	const type = props.asChild ? undefined : triggerType;

	return (
		<TabsPrimitive.Trigger
			data-slot="tabs-trigger"
			type={type}
			className={cn(
				"border-none py-2.5 bg-transparent",
				"text-inherit font-normal text-sm",
				"inline-flex gap-2 items-center",
				"cursor-pointer",
				"transition-colors duration-150 ease-linear",
				"-mb-px",
			)}
			{...props}
		/>
	);
};

type TabsContentProps = ComponentProps<typeof TabsPrimitive.Content>;

export const TabsContent = ({ ...props }: TabsContentProps) => {
	return <TabsPrimitive.Content data-slot="tabs-content" {...props} />;
};

// --- Router link tabs (URL-driven navigation) ---

// Keeping this for now because of a workaround in WorkspaceBuildPageView.
export const TAB_PADDING_X = 16;

type LinkTabsContextValue = {
	active: string;
};

const LinkTabsContext = createContext<LinkTabsContextValue | undefined>(
	undefined,
);

type LinkTabsProps = HTMLAttributes<HTMLDivElement> & LinkTabsContextValue;

export const LinkTabs = ({
	className,
	active,
	...htmlProps
}: LinkTabsProps) => {
	return (
		<LinkTabsContext.Provider value={{ active }}>
			<div
				data-slot="link-tabs"
				// Because the Tailwind preflight is not used, its necessary to set border style to solid and
				// reset all border widths to 0 https://tailwindcss.com/docs/border-width#using-without-preflight
				className={cn(
					"border-0 border-b border-solid border-border",
					className,
				)}
				{...htmlProps}
			/>
		</LinkTabsContext.Provider>
	);
};

type LinkTabsListProps = HTMLAttributes<HTMLDivElement>;

export const LinkTabsList = ({ className, ...props }: LinkTabsListProps) => {
	const tabsContext = useContext(LinkTabsContext);
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
		<div ref={listRef} data-slot="link-tabs-list-root" className="relative">
			<div
				data-slot="link-tabs-list"
				className={cn("flex items-baseline gap-6", className)}
				{...props}
			/>
			<div
				ref={indicatorRef}
				data-slot="link-tabs-indicator"
				className="absolute bottom-0 h-px bg-surface-invert-primary opacity-0 transition-all duration-300 ease-in-out"
			/>
		</div>
	);
};

type TabLinkProps = LinkProps & {
	value: string;
};

export const TabLink = ({ value, className, ...linkProps }: TabLinkProps) => {
	const tabsContext = useContext(LinkTabsContext);
	if (!tabsContext) {
		throw new Error("TabLink must be used inside LinkTabs");
	}

	const isActive = tabsContext.active === value;

	return (
		<Link
			data-slot="tab-link"
			data-active={isActive}
			aria-current={isActive ? "page" : undefined}
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

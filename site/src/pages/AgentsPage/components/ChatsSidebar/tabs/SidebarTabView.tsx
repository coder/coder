import { cn } from "cn";
import {
	ArrowLeftIcon,
	ChevronLeftIcon,
	ChevronRightIcon,
	MaximizeIcon,
	MinimizeIcon,
	PanelLeftIcon,
	XIcon,
} from "lucide-react";
import {
	Children,
	createContext,
	isValidElement,
	useContext,
	useEffect,
	useEffectEvent,
	useId,
	useRef,
	useState,
} from "react";
import { useOutletContext } from "react-router";
import { Button } from "#/components/Button/Button";
import type { AgentsPageOutletContext } from "../../../AgentsPageLayout";
import { getEffectiveTabId } from "./getEffectiveTabId";

type SidebarTabProps = {
	id: string;
	/** Label shown in the tab button. */
	label: string;
	/** Optional icon shown before the label. */
	icon?: React.ReactNode;
	badge?: React.ReactNode;
	/**
	 * Called when the user closes the tab. `nextActiveTabId` is the
	 * neighbouring tab to select when the closed tab was the active one.
	 */
	onClose?: (nextActiveTabId: string | undefined) => void;
	/** The panel. It stays mounted while the tab is inactive. */
	children: React.ReactNode;
};

/** What a panel inside a `SidebarTab` knows about its tab. */
type TabPanelState = {
	isActive: boolean;
	/** Whether the right panel itself is open. */
	isPanelOpen: boolean;
	isExpanded: boolean;
};

/**
 * Outside a `SidebarTabView` a panel renders as the only, visible tab, which
 * keeps panels usable on their own in tests and stories.
 */
export const TabPanelStateContext = createContext<TabPanelState>({
	isActive: true,
	isPanelOpen: true,
	isExpanded: false,
});

/** Returns the state of the tab panel the calling component renders in. */
export const useTabPanelState = () => {
	return useContext(TabPanelStateContext);
};

type SidebarTabViewContextValue = {
	effectiveTabId: string | null;
	tabIdPrefix: string;
	isPanelOpen: boolean;
	isExpanded: boolean;
};

const SidebarTabViewContext = createContext<SidebarTabViewContextValue | null>(
	null,
);

/** Returns the `SidebarTabView` that a `SidebarTab` is a child of. */
const useParentTabView = () => {
	const view = useContext(SidebarTabViewContext);

	if (!view) {
		throw new Error("SidebarTab must be rendered inside a SidebarTabView.");
	}

	return view;
};

/**
 * One tab of a `SidebarTabView`. It must be a direct child of the view,
 * which reads its props to build the tab bar.
 */
export const SidebarTab: React.FC<SidebarTabProps> = ({ id, children }) => {
	const view = useParentTabView();

	const isActive = view.effectiveTabId === id;

	return (
		<div
			role="tabpanel"
			aria-labelledby={`${view.tabIdPrefix}-tab-${id}`}
			className={cn(
				"min-h-0 flex-1",
				// Keep inactive panels in the tree but invisible: a canvas xterm
				// preserves painted pixels while hidden, so switching back is instant.
				!isActive && "invisible absolute inset-0",
			)}
			inert={!isActive}
		>
			<TabPanelStateContext
				value={{
					isActive,
					isPanelOpen: view.isPanelOpen,
					isExpanded: view.isExpanded,
				}}
			>
				{children}
			</TabPanelStateContext>
		</div>
	);
};

type SidebarTabViewProps = {
	/** `SidebarTab` elements, in display order. */
	children: React.ReactNode;
	/**
	 * The stored tab selection. When that tab is not rendered, the first
	 * tab is shown instead.
	 */
	activeTabId: string | null;
	/** Called when the user switches tabs. */
	onActiveTabChange: (tabId: string) => void;
	/** Whether the right panel is open. Panels read it through `useTabPanelState`. */
	isPanelOpen: boolean;
	/** Whether the panel is in expanded/fullscreen mode. */
	isExpanded: boolean;
	/** Callback to toggle expanded state. */
	onToggleExpanded: () => void;
	/** Shown in center when expanded. */
	chatTitle?: string;
	/** Callback to close the panel (used on mobile). */
	onClose?: () => void;
	/**
	 * Renders the add-tab control. `closeTab` closes a tab the same way its
	 * close button does, so a menu toggle selects the same neighbour.
	 */
	addTabControl?: (closeTab: (tabId: string) => void) => React.ReactNode;
};

const TAB_SCROLL_AMOUNT = 120;

function useTabScroll() {
	const ref = useRef<HTMLDivElement>(null);
	const [canScrollLeft, setCanScrollLeft] = useState(false);
	const [canScrollRight, setCanScrollRight] = useState(false);
	const updateScrollState = useEffectEvent(() => {
		const element = ref.current;
		if (!element) {
			return;
		}

		setCanScrollLeft(element.scrollLeft > 0);
		setCanScrollRight(
			element.scrollLeft + element.clientWidth < element.scrollWidth - 1,
		);
	});

	useEffect(() => {
		const element = ref.current;
		if (!element) {
			return;
		}

		updateScrollState();
		element.addEventListener("scroll", updateScrollState, { passive: true });

		const resizeObserver = new ResizeObserver(updateScrollState);
		resizeObserver.observe(element);

		return () => {
			element.removeEventListener("scroll", updateScrollState);
			resizeObserver.disconnect();
		};
	}, []);

	useEffect(() => {
		updateScrollState();
	});

	const scrollLeft = () => {
		ref.current?.scrollBy({
			left: -TAB_SCROLL_AMOUNT,
			behavior: "smooth",
		});
	};

	const scrollRight = () => {
		ref.current?.scrollBy({
			left: TAB_SCROLL_AMOUNT,
			behavior: "smooth",
		});
	};

	return { ref, canScrollLeft, canScrollRight, scrollLeft, scrollRight };
}

type ScrollChevronButtonProps = {
	direction: "left" | "right";
	onClick: () => void;
	ariaLabel: string;
};

const ScrollChevronButton: React.FC<ScrollChevronButtonProps> = ({
	direction,
	onClick,
	ariaLabel,
}) => {
	const isLeft = direction === "left";
	const Icon = isLeft ? ChevronLeftIcon : ChevronRightIcon;

	return (
		<button
			type="button"
			onClick={onClick}
			aria-label={ariaLabel}
			className={cn(
				"absolute inset-y-0 z-10 flex w-8 cursor-pointer items-center border-none p-0 text-content-primary",
				isLeft
					? "left-0 justify-start pl-1 [background:linear-gradient(to_right,hsl(var(--surface-primary))_50%,transparent)]"
					: "right-0 justify-end pr-1 [background:linear-gradient(to_left,hsl(var(--surface-primary))_50%,transparent)]",
			)}
		>
			<Icon className="size-3.5" />
		</button>
	);
};

export const SidebarTabView: React.FC<SidebarTabViewProps> = ({
	children,
	activeTabId,
	onActiveTabChange,
	isPanelOpen,
	isExpanded,
	onToggleExpanded,
	chatTitle,
	onClose,
	addTabControl,
}) => {
	const { isSidebarCollapsed, onToggleSidebarCollapsed } =
		useOutletContext<AgentsPageOutletContext | undefined>() ?? {};

	const tabIdPrefix = useId();

	const {
		ref: tabScrollRef,
		canScrollLeft,
		canScrollRight,
		scrollLeft: scrollTabsLeft,
		scrollRight: scrollTabsRight,
	} = useTabScroll();

	const tabs = Children.toArray(children).map((child) => {
		if (!isValidElement<SidebarTabProps>(child) || child.type !== SidebarTab) {
			throw new Error("SidebarTabView children must be SidebarTab elements.");
		}

		return child.props;
	});

	const tabIds = tabs.map((tab) => tab.id);

	const effectiveTabId = getEffectiveTabId(tabIds, activeTabId);

	const closeTab = (tabId: string) => {
		const tab = tabs.find((candidate) => candidate.id === tabId);
		if (!tab?.onClose) {
			return;
		}

		const remainingTabIds = tabIds.filter((id) => id !== tabId);

		const closedTabIndex = tabIds.indexOf(tabId);
		const nextActiveTabId =
			effectiveTabId === tabId
				? remainingTabIds[Math.min(closedTabIndex, remainingTabIds.length - 1)]
				: undefined;

		tab.onClose(nextActiveTabId);
	};

	if (tabs.length === 0) {
		return (
			<div className="flex h-full min-w-0 flex-col overflow-hidden bg-surface-primary">
				<div
					role="tablist"
					className="flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border px-4 py-1.5 lg:px-3 lg:py-1"
				>
					{onClose && (
						<Button
							variant="subtle"
							size="icon"
							onClick={onClose}
							aria-label="Close panel"
							className="size-7 shrink-0 lg:hidden"
						>
							<ArrowLeftIcon />
						</Button>
					)}
					<div className="min-w-0 shrink-0 text-center">
						{isExpanded && chatTitle && (
							<span className="truncate text-sm text-content-primary">
								{chatTitle}
							</span>
						)}
					</div>
					{addTabControl?.(closeTab)}
					<Button
						variant="subtle"
						size="icon"
						onClick={onToggleExpanded}
						aria-label={isExpanded ? "Collapse panel" : "Expand panel"}
						className="hidden size-7 shrink-0 text-content-secondary hover:text-content-primary lg:inline-flex"
					>
						{isExpanded ? <MinimizeIcon /> : <MaximizeIcon />}
					</Button>
				</div>
				<div className="flex flex-1 items-center justify-center p-6 text-center text-xs text-content-secondary">
					No panels available.
				</div>
			</div>
		);
	}

	return (
		<div className="flex h-full min-w-0 flex-col overflow-hidden bg-surface-primary">
			<div
				role="tablist"
				className="relative flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border px-4 py-1.5 lg:px-3 lg:py-1"
			>
				{onClose && (
					<Button
						variant="subtle"
						size="icon"
						onClick={onClose}
						aria-label="Close panel"
						className="size-7 shrink-0 lg:hidden"
					>
						<ArrowLeftIcon />
					</Button>
				)}
				{isExpanded && isSidebarCollapsed && onToggleSidebarCollapsed && (
					<Button
						variant="subtle"
						size="icon"
						onClick={onToggleSidebarCollapsed}
						aria-label="Expand sidebar"
						className="mr-1 size-7 shrink-0"
					>
						<PanelLeftIcon />
					</Button>
				)}
				<div className="relative min-w-0 flex-1">
					{canScrollLeft && (
						<ScrollChevronButton
							ariaLabel="Scroll tabs left"
							direction="left"
							onClick={scrollTabsLeft}
						/>
					)}
					<div
						ref={tabScrollRef}
						className="flex w-full min-w-0 items-center gap-1 overflow-x-auto scrollbar-none [&::-webkit-scrollbar]:hidden"
					>
						{tabs.map((tab) => {
							const isActive = effectiveTabId === tab.id;
							const isCloseable = tab.onClose !== undefined;
							const tabButton = (
								<Button
									id={`${tabIdPrefix}-tab-${tab.id}`}
									role="tab"
									aria-selected={isActive}
									onClick={() => onActiveTabChange(tab.id)}
									variant="outline"
									size="lg"
									className={cn(
										"shrink-0 h-6 min-w-0 gap-1.5 px-2 py-0 bg-surface-primary",
										isActive &&
											"bg-surface-quaternary/25 text-content-primary hover:bg-surface-quaternary/50",
										tab.badge && "pr-0",
										isCloseable && "rounded-r-none border-r-0 pr-2.5",
									)}
								>
									{tab.icon}
									{tab.label}
									{tab.badge && (
										<span
											className={cn(
												"flex -my-px items-center self-stretch transition-opacity",
												!isActive && "opacity-50",
											)}
										>
											{tab.badge}
										</span>
									)}
								</Button>
							);

							if (!isCloseable) {
								return (
									<div key={tab.id} className="flex shrink-0 items-center">
										{tabButton}
									</div>
								);
							}

							return (
								<div key={tab.id} className="flex shrink-0 items-center">
									{tabButton}
									<Button
										variant="outline"
										size="icon"
										onClick={(event) => {
											event.stopPropagation();
											closeTab(tab.id);
										}}
										aria-label={`Close ${tab.label} tab`}
										className={cn(
											"size-6 rounded-l-none rounded-r-md bg-surface-primary p-0 text-content-secondary hover:text-content-primary [&>svg]:size-3",
											isActive &&
												"bg-surface-quaternary/25 text-content-primary hover:bg-surface-quaternary/50",
										)}
									>
										<XIcon />
									</Button>
								</div>
							);
						})}
						{addTabControl?.(closeTab)}
					</div>
					{canScrollRight && (
						<ScrollChevronButton
							ariaLabel="Scroll tabs right"
							direction="right"
							onClick={scrollTabsRight}
						/>
					)}
				</div>
				{isExpanded && chatTitle && (
					<div className="pointer-events-none absolute inset-0 flex items-center justify-center">
						<span className="truncate px-24 text-sm text-content-primary">
							{chatTitle}
						</span>
					</div>
				)}
				<Button
					variant="subtle"
					size="icon"
					onClick={onToggleExpanded}
					aria-label={isExpanded ? "Collapse panel" : "Expand panel"}
					className="hidden size-7 shrink-0 self-start text-content-secondary hover:text-content-primary lg:inline-flex"
				>
					{isExpanded ? <MinimizeIcon /> : <MaximizeIcon />}
				</Button>
			</div>
			<div className="relative flex min-h-0 flex-1 flex-col">
				<SidebarTabViewContext
					value={{
						effectiveTabId,
						tabIdPrefix,
						isPanelOpen,
						isExpanded,
					}}
				>
					{children}
				</SidebarTabViewContext>
			</div>
		</div>
	);
};

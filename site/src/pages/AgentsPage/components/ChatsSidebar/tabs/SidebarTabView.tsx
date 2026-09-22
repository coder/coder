import { cn } from "cn";
import {
	ChevronLeftIcon,
	ChevronRightIcon,
	MaximizeIcon,
	MinimizeIcon,
	PanelLeftIcon,
	PanelRightCloseIcon,
} from "lucide-react";
import {
	type FC,
	type ReactNode,
	useEffect,
	useEffectEvent,
	useId,
	useRef,
	useState,
} from "react";
import { useOutletContext } from "react-router";
import { Button } from "#/components/Button/Button";
import { Tabs, TabsList, TabsTrigger } from "#/components/Tabs/Tabs";
import type { AgentsPageOutletContext } from "../../../AgentsPageLayout";

/** A single tab definition for the sidebar panel. */
export type SidebarTab = {
	id: string;
	/** Label shown in the tab button. */
	label: string;
	/** Count shown after the label, for example open terminals. */
	badge?: number;
	/** The content to render when this tab is active. */
	content: ReactNode;
};

type SidebarTabViewProps = {
	/** The tabs to display. */
	tabs: SidebarTab[];
	/** Whether the panel is in expanded/fullscreen mode. */
	isExpanded: boolean;
	/** Callback to toggle expanded state. */
	onToggleExpanded: () => void;
	/** Shown in center when expanded. */
	chatTitle?: string;
	/** Callback to close the panel. */
	onClose?: () => void;
	/**
	 * The resolved tab ID to render as active (computed by the parent
	 * with `getEffectiveTabId`). Keeping a single source of truth in the
	 * parent prevents this component's highlight from drifting from
	 * parent-side gating like `TerminalPanel.isVisible` or
	 * `DebugPanel.isVisible`.
	 */
	effectiveTabId: string | null;
	/** Called when the user switches tabs. */
	onActiveTabChange: (tabId: string) => void;
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

const ScrollChevronButton: FC<ScrollChevronButtonProps> = ({
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

const HeaderActions: FC<{
	isExpanded: boolean;
	onToggleExpanded: () => void;
	onClose?: () => void;
}> = ({ isExpanded, onToggleExpanded, onClose }) => (
	<div className="flex shrink-0 items-center gap-0.5">
		<Button
			variant="subtle"
			size="icon"
			onClick={onToggleExpanded}
			aria-label={isExpanded ? "Collapse panel" : "Expand panel"}
			className="hidden size-8 text-content-secondary hover:text-content-primary lg:inline-flex"
		>
			{isExpanded ? <MinimizeIcon /> : <MaximizeIcon />}
		</Button>
		{onClose && (
			<Button
				variant="subtle"
				size="icon"
				onClick={onClose}
				aria-label="Close panel"
				className="size-8 text-content-secondary hover:text-content-primary"
			>
				<PanelRightCloseIcon />
			</Button>
		)}
	</div>
);

export const SidebarTabView: FC<SidebarTabViewProps> = ({
	tabs,
	isExpanded,
	onToggleExpanded,
	chatTitle,
	onClose,
	effectiveTabId,
	onActiveTabChange,
}) => {
	const { isSidebarCollapsed, onToggleSidebarCollapsed } =
		useOutletContext<AgentsPageOutletContext | undefined>() ?? {};
	const idPrefix = useId();
	const {
		ref: tabScrollRef,
		canScrollLeft,
		canScrollRight,
		scrollLeft: scrollTabsLeft,
		scrollRight: scrollTabsRight,
	} = useTabScroll();

	// Panels mount the first time their tab is selected and stay mounted so
	// iframes and terminals keep their state across switches. Restoring a
	// persisted tab counts as a selection, so this is derived during render.
	const [mountedTabIds, setMountedTabIds] = useState<readonly string[]>([]);
	if (effectiveTabId !== null && !mountedTabIds.includes(effectiveTabId)) {
		setMountedTabIds([...mountedTabIds, effectiveTabId]);
	}

	useEffect(() => {
		if (effectiveTabId === null) {
			return;
		}
		document
			.getElementById(`${idPrefix}-tab-${effectiveTabId}`)
			?.scrollIntoView({ block: "nearest", inline: "nearest" });
	}, [effectiveTabId, idPrefix]);

	const sidebarExpandButton = isExpanded &&
		isSidebarCollapsed &&
		onToggleSidebarCollapsed && (
			<Button
				variant="subtle"
				size="icon"
				onClick={onToggleSidebarCollapsed}
				aria-label="Expand sidebar"
				className="size-8 shrink-0"
			>
				<PanelLeftIcon />
			</Button>
		);

	if (tabs.length === 0) {
		return (
			<div className="flex h-full min-w-0 flex-col overflow-hidden bg-surface-primary">
				<div className="flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border-default px-3">
					{sidebarExpandButton}
					<div className="min-w-0 flex-1 truncate text-center text-sm text-content-primary">
						{isExpanded && chatTitle}
					</div>
					<HeaderActions
						isExpanded={isExpanded}
						onToggleExpanded={onToggleExpanded}
						onClose={onClose}
					/>
				</div>
				<div className="flex flex-1 items-center justify-center p-6 text-center text-xs text-content-secondary">
					No panels available.
				</div>
			</div>
		);
	}

	return (
		<Tabs
			value={effectiveTabId ?? undefined}
			onValueChange={onActiveTabChange}
			className="flex h-full min-w-0 flex-col overflow-hidden bg-surface-primary"
		>
			<div className="relative flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border-default px-3">
				{sidebarExpandButton}
				{/* Pulled down 1px so the active trigger's underline paints over the header border. */}
				<div className="relative -mb-px min-w-0 flex-1">
					{canScrollLeft && (
						<ScrollChevronButton
							ariaLabel="Scroll tabs left"
							direction="left"
							onClick={scrollTabsLeft}
						/>
					)}
					<TabsList
						ref={tabScrollRef}
						className="w-full flex-nowrap gap-3 overflow-x-auto border-b-0 scrollbar-none [&::-webkit-scrollbar]:hidden"
					>
						{tabs.map((tab) => (
							<TabsTrigger
								key={tab.id}
								id={`${idPrefix}-tab-${tab.id}`}
								value={tab.id}
								aria-controls={`${idPrefix}-panel-${tab.id}`}
								className="mb-0 shrink-0 whitespace-nowrap py-2"
							>
								{tab.label}
								{tab.badge !== undefined && (
									<>
										{" "}
										<span className="inline-flex h-5 min-w-5 items-center justify-center rounded bg-surface-quaternary/40 px-1 font-mono text-2xs text-content-secondary">
											{tab.badge}
										</span>
									</>
								)}
							</TabsTrigger>
						))}
					</TabsList>
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
				<HeaderActions
					isExpanded={isExpanded}
					onToggleExpanded={onToggleExpanded}
					onClose={onClose}
				/>
			</div>
			<div className="relative flex min-h-0 flex-1 flex-col">
				{tabs
					.filter((tab) => mountedTabIds.includes(tab.id))
					.map((tab) => {
						const isActive = effectiveTabId === tab.id;
						return (
							<div
								key={tab.id}
								id={`${idPrefix}-panel-${tab.id}`}
								role="tabpanel"
								aria-labelledby={`${idPrefix}-tab-${tab.id}`}
								className={cn(
									"flex min-h-0 flex-1 flex-col",
									// Keep inactive panels in the tree but invisible: a canvas xterm
									// preserves painted pixels while hidden, so switching back is instant.
									!isActive && "invisible absolute inset-0",
								)}
								inert={!isActive}
							>
								{tab.content}
							</div>
						);
					})}
			</div>
		</Tabs>
	);
};

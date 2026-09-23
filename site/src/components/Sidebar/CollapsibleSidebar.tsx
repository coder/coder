import { cn } from "cn";
import { type FC, type ReactNode, useMemo, useRef } from "react";
import { Drawer, DrawerContent, DrawerTitle } from "#/components/Drawer/Drawer";
import { SidebarContext } from "./SidebarContext";
import { SidebarResizeHandle } from "./SidebarResizeHandle";
import { COLLAPSED_WIDTH, useSidebarResize } from "./useSidebarResize";

/** Height of the sticky dashboard navbar the sidebar sits beneath. */
const NAVBAR_HEIGHT = 72;

interface CollapsibleSidebarProps {
	children: ReactNode;
	className?: string;
	/** Accessible name of the navigation, also titling the mobile drawer. */
	label: string;
	/** Key for the persisted collapsed preference, unique per sidebar. */
	storageKey: string;
	/**
	 * Content pinned above the scrolling nav list, inside the collapsing
	 * clipper. Receives the sidebar context, so it can render its own
	 * collapsed variant.
	 */
	header?: ReactNode;
	/**
	 * Space in px to leave free at the bottom of the viewport, for a
	 * bottom-pinned bar such as the deployment banner.
	 */
	bottomInset?: number;
}

/**
 * Sticky sidebar column beneath the dashboard navbar that collapses to a
 * 64px icon rail. The width is persisted per `storageKey`, narrow
 * viewports start collapsed, and below the md breakpoint the expanded
 * state is a modal drawer over the page. Children read `SidebarContext`
 * to render their collapsed variant.
 */
export const CollapsibleSidebar: FC<CollapsibleSidebarProps> = ({
	children,
	className,
	label,
	storageKey,
	header,
	bottomInset = 0,
}) => {
	const { width, collapsed, mobile, expand, collapse, toggle } =
		useSidebarResize(storageKey);
	const containerRef = useRef<HTMLDivElement>(null);

	// Below md the expanded sidebar is a drawer over the page while the
	// icon rail keeps its place in the layout underneath. The rail keeps
	// rendering the collapsed variant so its toggle can take focus back
	// when the drawer closes; while the drawer is open the children are
	// therefore mounted twice, once per variant.
	const drawerOpen = mobile && !collapsed;
	// The drawer has no Radix trigger, so remember which rail control
	// opened it and hand focus back there when it closes.
	const openerRef = useRef<HTMLElement | null>(null);
	const railContext = useMemo(() => {
		const rememberOpener = () => {
			const active = document.activeElement;
			openerRef.current = active instanceof HTMLElement ? active : null;
		};
		return {
			collapsed: mobile || collapsed,
			expand: () => {
				rememberOpener();
				expand();
			},
			toggle: () => {
				rememberOpener();
				toggle();
			},
		};
	}, [mobile, collapsed, expand, toggle]);
	const drawerContext = useMemo(
		() => ({ collapsed: false, expand, toggle }),
		[expand, toggle],
	);

	// The header stays put; only the nav list scrolls, within its own
	// scroll area so the page scrollbar never moves the sidebar.
	const renderPanel = (variant: "rail" | "drawer") => (
		<div
			className={cn(
				"flex h-full flex-col",
				variant === "drawer" ? "w-full" : "w-[240px]",
			)}
		>
			{/* Both the header and the list reserve a stable scrollbar gutter
			    so the toggle stays put when the list starts to scroll (only
			    matters with classic, non-overlay scrollbars). */}
			<div className="overflow-hidden [scrollbar-gutter:stable]">{header}</div>
			{header && <div className="h-px shrink-0 bg-border" />}
			<nav
				aria-label={label}
				className={cn(
					"flex-1 min-h-0 overflow-y-auto [scrollbar-gutter:stable]",
					"flex flex-col",
					"px-3 pt-2 pb-6",
					className,
				)}
			>
				{children}
			</nav>
		</div>
	);

	return (
		<>
			<SidebarContext.Provider value={railContext}>
				{/* Non-clipping wrapper for positioning. The resize handle
				    lives here so it isn't clipped by overflow-hidden. */}
				<div
					ref={containerRef}
					className="relative shrink-0 sticky z-30 transition-[width] duration-150 ease-in-out"
					style={{
						width: mobile ? COLLAPSED_WIDTH : width,
						top: NAVBAR_HEIGHT,
						height: `calc(100vh - ${NAVBAR_HEIGHT + bottomInset}px)`,
					}}
				>
					{/* Clipping container for the nav content. */}
					<div className="h-full overflow-hidden">{renderPanel("rail")}</div>
					{/* Handle sits outside the overflow-hidden div so its right
					    half isn't clipped. */}
					{!mobile && (
						<SidebarResizeHandle
							containerRef={containerRef}
							collapsed={collapsed}
							onCollapse={collapse}
							onExpand={expand}
						/>
					)}
				</div>
			</SidebarContext.Provider>
			{mobile && (
				<Drawer
					open={drawerOpen}
					onOpenChange={(open) => {
						if (!open) {
							collapse();
						}
					}}
					direction="top"
				>
					<DrawerContent
						className="bottom-0 max-h-none bg-surface-primary"
						style={{ top: NAVBAR_HEIGHT }}
						aria-describedby={undefined}
						// Following a link closes the drawer. Delegated here so views
						// need no wiring; keyboard activation of a link dispatches a
						// click as well.
						onClick={(event) => {
							if (
								event.target instanceof Element &&
								event.target.closest("a[href]")
							) {
								collapse();
							}
						}}
						onCloseAutoFocus={(event) => {
							const opener = openerRef.current;
							if (opener?.isConnected) {
								event.preventDefault();
								opener.focus();
							}
						}}
					>
						<DrawerTitle className="sr-only">{label}</DrawerTitle>
						<SidebarContext.Provider value={drawerContext}>
							{renderPanel("drawer")}
						</SidebarContext.Provider>
					</DrawerContent>
				</Drawer>
			)}
		</>
	);
};

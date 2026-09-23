import { cn } from "cn";
import { type FC, type ReactNode, useMemo, useRef } from "react";
import { Drawer, DrawerContent, DrawerTitle } from "#/components/Drawer/Drawer";
import { SidebarContext } from "./SidebarContext";
import { SidebarResizeHandle } from "./SidebarResizeHandle";
import { COLLAPSED_WIDTH, useSidebarResize } from "./useSidebarResize";

/** Height of the dashboard navbar. */
const NAVBAR_HEIGHT = 72;

type CollapsibleSidebarProps = {
	children: ReactNode;
	className?: string;
	/** Accessible name for the nav and the mobile drawer. */
	label: string;
	/** localStorage key for the collapsed state. */
	storageKey: string;
	/** Pinned above the scrolling nav list. */
	header?: ReactNode;
	/** Px reserved at the bottom of the viewport, e.g. for the deployment banner. */
	bottomInset?: number;
};

/** Sticky sidebar that collapses to a 64px icon rail, and opens as a drawer below md. */
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

	// Below md the rail stays collapsed and the drawer shows the expanded nav.
	const drawerOpen = mobile && !collapsed;
	// The rail control that opened the drawer, refocused when it closes.
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

	const renderPanel = (variant: "rail" | "drawer") => (
		<div
			className={cn(
				"flex h-full flex-col",
				variant === "drawer" ? "w-full" : "w-[240px]",
			)}
		>
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
				<div
					ref={containerRef}
					className="relative shrink-0 sticky z-30 transition-[width] duration-150 ease-in-out"
					style={{
						width: mobile ? COLLAPSED_WIDTH : width,
						top: NAVBAR_HEIGHT,
						height: `calc(100vh - ${NAVBAR_HEIGHT + bottomInset}px)`,
					}}
				>
					<div className="h-full overflow-hidden">{renderPanel("rail")}</div>
					{/* Outside the overflow-hidden panel so it isn't clipped. */}
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
						// Close the drawer when a link is followed.
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

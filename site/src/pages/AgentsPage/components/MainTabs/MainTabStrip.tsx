import { cva } from "class-variance-authority";
import { cn } from "cn";
import { ChevronDownIcon, XIcon } from "lucide-react";
import { useEffect, useEffectEvent, useRef, useState } from "react";
import { Button } from "#/components/Button/Button";
import {
	ContextMenu,
	ContextMenuContent,
	ContextMenuItem,
	ContextMenuSeparator,
	ContextMenuTrigger,
} from "#/components/ContextMenu/ContextMenu";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { useMediaQuery } from "#/hooks/useMediaQuery";
import { noHoverMediaQuery } from "#/utils/mobile";
import { getPathBasename } from "../../utils/path";
import { FileActionsMenuItems } from "../FileTab/FileActionsMenuItems";
import { getTabHints } from "./tabHints";

/** A tab's path, or null for the Chat tab. */
export type TabKey = string | null;

/** Element IDs of the file tab at `fileIndex` and its panel, or of the Chat tab's. */
export const getTabElementIds = (idPrefix: string, fileIndex?: number) => {
	const suffix = fileIndex ?? "chat";
	return {
		tabId: `${idPrefix}-tab-${suffix}`,
		panelId: `${idPrefix}-panel-${suffix}`,
	};
};

type MainTabStripProps = {
	idPrefix: string;
	paths: readonly string[];
	activePath: string | null;
	/** Changes on every activation; each change scrolls the active tab into view. */
	activation: number;
	/** Paths with unsaved edits. */
	dirtyPaths: ReadonlySet<string>;
	/** Receives the tab buttons, which keyboard navigation focuses. */
	tabButtonsRef: React.RefObject<Map<TabKey, HTMLButtonElement>>;
	onActivate: (key: TabKey) => void;
	onTabKeyDown: (event: React.KeyboardEvent, key: TabKey) => void;
	/** Closes `paths`, at the request of the tab at `source`. */
	onClose: (paths: readonly string[], source: string) => void;
};

const tabButtonVariants = cva(
	"h-6 min-w-0 shrink-0 gap-1.5 bg-surface-primary px-2 py-0",
	{
		variants: {
			active: {
				true: "bg-surface-quaternary/25 text-content-primary hover:bg-surface-quaternary/50",
				false: "",
			},
		},
	},
);

const trackElement =
	<Key, Tracked extends HTMLElement>(
		elements: React.RefObject<Map<Key, Tracked>>,
		key: Key,
	) =>
	(element: Tracked | null) => {
		if (!element) {
			return;
		}
		elements.current.set(key, element);
		return () => {
			elements.current.delete(key);
		};
	};

// Room kept between the scrolled-in tab and the strip edge, matching the
// strip's horizontal padding.
const scrollMargin = 16;

const scrollIntoStrip = (strip: HTMLElement, tab: HTMLElement) => {
	const stripRect = strip.getBoundingClientRect();
	const tabRect = tab.getBoundingClientRect();
	const overflowLeft = stripRect.left + scrollMargin - tabRect.left;
	const overflowRight = tabRect.right - (stripRect.right - scrollMargin);
	if (overflowLeft > 0) {
		strip.scrollLeft -= overflowLeft;
	} else if (overflowRight > 0) {
		strip.scrollLeft += overflowRight;
	}
};

// The value of the Chat tab in the open tabs menu. No file path is empty.
const chatMenuValue = "";

/**
 * The tab strip of the chat view: a pinned Chat tab followed by file tabs,
 * and a menu of all tabs while they overflow the strip.
 */
export const MainTabStrip: React.FC<MainTabStripProps> = ({
	idPrefix,
	paths,
	activePath,
	activation,
	dirtyPaths,
	tabButtonsRef,
	onActivate,
	onTabKeyDown,
	onClose,
}) => {
	const canHover = !useMediaQuery(noHoverMediaQuery);
	const stripRef = useRef<HTMLDivElement>(null);
	// File tabs scroll into view with their close buttons.
	const fileTabsRef = useRef(new Map<string, HTMLDivElement>());
	const [isOverflowing, setIsOverflowing] = useState(false);
	const hints = getTabHints(paths);
	const isChatActive = activePath === null;
	const chatIds = getTabElementIds(idPrefix);

	const updateOverflow = useEffectEvent(() => {
		const strip = stripRef.current;
		if (strip) {
			setIsOverflowing(strip.scrollWidth > strip.clientWidth);
		}
	});

	const revealActiveTab = useEffectEvent(() => {
		const strip = stripRef.current;
		const tab =
			activePath === null
				? tabButtonsRef.current.get(null)
				: fileTabsRef.current.get(activePath);
		if (strip && tab) {
			scrollIntoStrip(strip, tab);
		}
	});

	// Opening, closing, and renaming tabs change the content width without
	// resizing the strip, so every render rechecks it.
	useEffect(() => {
		updateOverflow();
	});

	useEffect(() => {
		const strip = stripRef.current;
		if (!strip) {
			return;
		}
		const resizeObserver = new ResizeObserver(() => {
			updateOverflow();
			revealActiveTab();
		});
		resizeObserver.observe(strip);
		// A mouse wheel scrolls vertically, which the strip cannot. React
		// registers wheel listeners as passive, so this one is native.
		const handleWheel = (event: WheelEvent) => {
			if (
				event.ctrlKey ||
				Math.abs(event.deltaX) >= Math.abs(event.deltaY) ||
				strip.scrollWidth <= strip.clientWidth
			) {
				return;
			}
			event.preventDefault();
			const lineHeight = 16;
			strip.scrollLeft +=
				event.deltaMode === WheelEvent.DOM_DELTA_LINE
					? event.deltaY * lineHeight
					: event.deltaY;
		};
		strip.addEventListener("wheel", handleWheel, { passive: false });
		return () => {
			resizeObserver.disconnect();
			strip.removeEventListener("wheel", handleWheel);
		};
	}, []);

	// Activation reruns this for the tab that is already active, which the
	// user may have scrolled out of view.
	useEffect(() => {
		revealActiveTab();
	}, [activePath, activation]);

	const getLabel = (path: string) => {
		const name = getPathBasename(path);
		const hint = hints.get(path);
		return { name, hint, accessibleName: hint ? `${name} ${hint}` : name };
	};

	return (
		<div className="relative z-10 flex shrink-0 items-center border-0 border-b border-solid border-border bg-surface-primary">
			<div
				ref={stripRef}
				role="tablist"
				aria-label="Chat and files"
				className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto px-4 py-1.5 scrollbar-none [&::-webkit-scrollbar]:hidden"
			>
				<Button
					ref={trackElement(tabButtonsRef, null)}
					id={chatIds.tabId}
					role="tab"
					aria-selected={isChatActive}
					aria-controls={chatIds.panelId}
					tabIndex={isChatActive ? 0 : -1}
					variant="outline"
					size="lg"
					className={tabButtonVariants({ active: isChatActive })}
					onClick={() => onActivate(null)}
					onKeyDown={(event) => onTabKeyDown(event, null)}
				>
					Chat
				</Button>
				{paths.map((path, index) => {
					const isActive = path === activePath;
					const isDirty = dirtyPaths.has(path);
					const { name, hint, accessibleName } = getLabel(path);
					const ids = getTabElementIds(idPrefix, index);
					return (
						<ContextMenu key={path}>
							<ContextMenuTrigger asChild>
								<div
									ref={trackElement(fileTabsRef, path)}
									className="group/tab flex shrink-0 items-center"
								>
									<Button
										ref={trackElement(tabButtonsRef, path)}
										id={ids.tabId}
										role="tab"
										aria-selected={isActive}
										aria-controls={ids.panelId}
										aria-label={
											isDirty ? `${accessibleName}, unsaved` : accessibleName
										}
										tabIndex={isActive ? 0 : -1}
										title={path}
										variant="outline"
										size="lg"
										className={cn(
											tabButtonVariants({ active: isActive }),
											"rounded-r-none border-r-0 pr-2.5",
										)}
										onClick={() => onActivate(path)}
										onKeyDown={(event) => onTabKeyDown(event, path)}
										onMouseDown={(event) => {
											// Middle-click would otherwise start autoscroll.
											if (event.button === 1) {
												event.preventDefault();
											}
										}}
										onAuxClick={(event) => {
											if (event.button === 1) {
												onClose([path], path);
											}
										}}
									>
										<span className="truncate">{name}</span>
										{hint && (
											<span className="truncate text-content-secondary">
												{hint}
											</span>
										)}
									</Button>
									<Button
										variant="outline"
										size="icon"
										// Delete on the tab closes it, so the button is not a Tab stop.
										tabIndex={-1}
										aria-label={`Close ${accessibleName}`}
										title={`Close ${path}`}
										className={cn(
											"relative size-6 rounded-l-none rounded-r-md bg-surface-primary p-0 text-content-secondary hover:text-content-primary [&>svg]:size-3",
											isActive &&
												"bg-surface-quaternary/25 text-content-primary hover:bg-surface-quaternary/50",
										)}
										onClick={() => onClose([path], path)}
									>
										{/* The unsaved dot takes the close button's slot so the tab
									    keeps its width; hovering the tab shows the X. Without
									    hover, the X always shows and the dot sits on its corner. */}
										{isDirty && (
											<span
												aria-hidden
												className={cn(
													"rounded-full bg-current",
													canHover
														? "size-2 group-hover/tab:hidden"
														: "absolute right-0.5 top-0.5 size-1.5",
												)}
											/>
										)}
										<XIcon
											className={cn(
												isDirty && canHover && "hidden group-hover/tab:block",
											)}
										/>
									</Button>
								</div>
							</ContextMenuTrigger>
							<ContextMenuContent>
								<ContextMenuItem onSelect={() => onClose([path], path)}>
									Close
								</ContextMenuItem>
								<ContextMenuItem
									disabled={paths.length === 1}
									onSelect={() =>
										onClose(
											paths.filter((other) => other !== path),
											path,
										)
									}
								>
									Close others
								</ContextMenuItem>
								<ContextMenuItem
									disabled={index === paths.length - 1}
									onSelect={() => onClose(paths.slice(index + 1), path)}
								>
									Close to the right
								</ContextMenuItem>
								<ContextMenuSeparator />
								<FileActionsMenuItems
									Item={ContextMenuItem}
									Separator={ContextMenuSeparator}
									path={path}
								/>
							</ContextMenuContent>
						</ContextMenu>
					);
				})}
			</div>
			{isOverflowing && (
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button
							variant="subtle"
							size="icon"
							aria-label="All tabs"
							title="All tabs"
							className="mr-2 size-6 shrink-0"
						>
							<ChevronDownIcon />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent
						align="end"
						collisionPadding={8}
						className="max-h-(--radix-dropdown-menu-content-available-height) max-w-96 overflow-y-auto"
					>
						<DropdownMenuRadioGroup
							value={activePath ?? chatMenuValue}
							onValueChange={(value) =>
								onActivate(value === chatMenuValue ? null : value)
							}
						>
							<DropdownMenuRadioItem value={chatMenuValue}>
								Chat
							</DropdownMenuRadioItem>
							{paths.map((path) => {
								const { name, hint, accessibleName } = getLabel(path);
								const isDirty = dirtyPaths.has(path);
								return (
									<DropdownMenuRadioItem
										key={path}
										value={path}
										title={path}
										aria-label={
											isDirty ? `${accessibleName}, unsaved` : accessibleName
										}
										className="gap-1.5"
									>
										<span className="truncate">{name}</span>
										{hint && (
											<span className="truncate text-content-secondary">
												{hint}
											</span>
										)}
										{isDirty && (
											<span
												aria-hidden
												className="size-2 shrink-0 rounded-full bg-current"
											/>
										)}
									</DropdownMenuRadioItem>
								);
							})}
						</DropdownMenuRadioGroup>
					</DropdownMenuContent>
				</DropdownMenu>
			)}
		</div>
	);
};

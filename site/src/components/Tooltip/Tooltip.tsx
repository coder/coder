/**
 * Copied from shadc/ui on 02/05/2025
 * @see {@link https://ui.shadcn.com/docs/components/tooltip}
 */

import { cn } from "cn";
import {
	Popover as PopoverPrimitive,
	Tooltip as TooltipPrimitive,
} from "radix-ui";
import {
	createContext,
	useContext,
	useEffect,
	useId,
	useRef,
	useState,
} from "react";

export const TooltipProvider = TooltipPrimitive.Provider;

/**
 * Shared open delay (ms) for tooltips. Used by the app-wide provider and by
 * self-contained tooltips such as InfoTooltip so hover timing stays consistent.
 */
export const TOOLTIP_DELAY_DURATION = 100;

/** Whether the closest `Tooltip` is interactive. */
const TooltipInteractiveContext = createContext(false);

type TooltipProps =
	| (React.ComponentProps<typeof TooltipPrimitive.Root> & {
			interactive?: false;
	  })
	| {
			/**
			 * Set when the content contains links, buttons, or other focusable
			 * elements. The ARIA tooltip pattern does not allow interactive
			 * content, so an interactive tooltip renders as a non-modal popover
			 * that still opens on hover and looks the same:
			 *
			 * - Opens on mouse hover and on keyboard focus, and stays open while
			 *   the pointer is over the content.
			 * - Click, Enter, or Space pins it open; touch devices open it with a
			 *   tap.
			 * - Tab moves from the trigger into the content and continues to the
			 *   element after the trigger. Escape closes it and returns focus to
			 *   the trigger.
			 * - While open, the content is the trigger's accessible description.
			 *
			 * Do not use it when the trigger performs its own action, such as a
			 * copy or refresh button, because clicking the trigger pins the
			 * tooltip instead.
			 */
			interactive: true;
			children: React.ReactNode;
			/** Delay in milliseconds before opening on hover. */
			delayDuration?: number;
	  };

export const Tooltip: React.FC<TooltipProps> = (props) => {
	if (props.interactive) {
		return (
			<TooltipInteractiveContext.Provider value={true}>
				<InteractiveTooltipRoot delayDuration={props.delayDuration}>
					{props.children}
				</InteractiveTooltipRoot>
			</TooltipInteractiveContext.Provider>
		);
	}
	return (
		<TooltipInteractiveContext.Provider value={false}>
			<TooltipPrimitive.Root {...props} />
		</TooltipInteractiveContext.Provider>
	);
};

type TooltipTriggerProps = React.ComponentProps<
	typeof TooltipPrimitive.Trigger
>;

export const TooltipTrigger: React.FC<TooltipTriggerProps> = (props) => {
	const interactive = useContext(TooltipInteractiveContext);
	return interactive ? (
		<InteractiveTooltipTrigger {...props} />
	) : (
		<TooltipPrimitive.Trigger {...props} />
	);
};

export const TooltipArrow = TooltipPrimitive.Arrow;

/** Surface styles shared by plain and interactive tooltips. */
const tooltipContentClassName = cn(
	"z-50 overflow-hidden rounded-md bg-surface-primary px-3 py-2 text-xs font-medium text-content-secondary",
	"border border-solid border-border animate-in fade-in-0 zoom-in-95",
	"origin-(--radix-popper-transform-origin)",
	"data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95",
	"data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2",
	"data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2",
);

type TooltipContentProps = React.ComponentProps<
	typeof TooltipPrimitive.Content
> & {
	disablePortal?: boolean;
};

export const TooltipContent: React.FC<TooltipContentProps> = ({
	className,
	sideOffset = 4,
	disablePortal,
	...props
}) => {
	const interactive = useContext(TooltipInteractiveContext);
	if (interactive) {
		return (
			<InteractiveTooltipContent
				className={className}
				sideOffset={sideOffset}
				disablePortal={disablePortal}
				{...props}
			/>
		);
	}

	const content = (
		<TooltipPrimitive.Content
			sideOffset={sideOffset}
			className={cn(tooltipContentClassName, className)}
			{...props}
		/>
	);

	return disablePortal ? (
		content
	) : (
		<TooltipPrimitive.Portal>{content}</TooltipPrimitive.Portal>
	);
};

export const TooltipTitle: React.FC<React.ComponentProps<"p">> = ({
	className,
	...props
}) => (
	<p
		className={cn("m-0 mb-1 font-semibold text-content-primary", className)}
		{...props}
	/>
);

export const TooltipMessage: React.FC<React.ComponentProps<"p">> = ({
	className,
	...props
}) => (
	<p
		className={cn("m-0 text-content-secondary [&_a]:mt-2", className)}
		{...props}
	/>
);

/**
 * Grace period before closing after the pointer leaves, so the pointer can
 * travel from the trigger into the content without the popover closing.
 */
const HOVER_CLOSE_DELAY = 150;

const TABBABLE_SELECTOR = [
	"a[href]",
	"button:not([disabled])",
	"input:not([disabled])",
	"select:not([disabled])",
	"textarea:not([disabled])",
	'[tabindex]:not([tabindex="-1"])',
].join(",");

const getTabbables = (root: ParentNode): HTMLElement[] =>
	Array.from(root.querySelectorAll<HTMLElement>(TABBABLE_SELECTOR)).filter(
		(el) =>
			!el.hasAttribute("data-radix-focus-guard") &&
			!el.closest("[inert],[hidden]") &&
			(el.checkVisibility?.() ?? true),
	);

type InteractiveTooltipContextValue = {
	open: boolean;
	triggerId: string;
	contentId: string;
	setTriggerNode: (node: HTMLButtonElement | null) => void;
	setContentNode: (node: HTMLDivElement | null) => void;
	setHovered: (hovered: boolean) => void;
	openFromFocus: () => void;
	toggleFromClick: (fromKeyboard: boolean) => void;
	close: () => void;
	/** Moves focus to the first focusable element in the content, if any. */
	focusContent: () => boolean;
	/** Moves focus to the element that follows the trigger, then closes. */
	focusAfterTrigger: () => void;
	focusTrigger: () => void;
};

const InteractiveTooltipContext =
	createContext<InteractiveTooltipContextValue | null>(null);

const useInteractiveTooltip = () => {
	const context = useContext(InteractiveTooltipContext);
	if (!context) {
		throw new Error(
			"Interactive tooltip parts must be used within <Tooltip interactive>",
		);
	}
	return context;
};

type InteractiveTooltipRootProps = {
	children: React.ReactNode;
	/** Delay in milliseconds before opening on hover. */
	delayDuration?: number;
};

/**
 * Implementation of `<Tooltip interactive>`: a non-modal popover that opens on
 * hover like a tooltip but can hold focusable content such as links.
 */
const InteractiveTooltipRoot: React.FC<InteractiveTooltipRootProps> = ({
	children,
	delayDuration = TOOLTIP_DELAY_DURATION,
}) => {
	const [hoverIntent, setHoverIntent] = useState(false);
	const [hovered, setHoveredState] = useState(false);
	const [focused, setFocused] = useState(false);
	const [pinned, setPinned] = useState(false);
	const [triggerNode, setTriggerNode] = useState<HTMLButtonElement | null>(
		null,
	);
	const [contentNode, setContentNode] = useState<HTMLDivElement | null>(null);
	const triggerId = useId();
	const contentId = useId();
	const open = hovered || focused || pinned;

	// Debounce hover changes: open after the delay, close after a grace period.
	useEffect(() => {
		if (hoverIntent === hovered) {
			return;
		}
		const delay = hoverIntent ? (open ? 0 : delayDuration) : HOVER_CLOSE_DELAY;
		const timer = window.setTimeout(() => setHoveredState(hoverIntent), delay);
		return () => window.clearTimeout(timer);
	}, [hoverIntent, hovered, open, delayDuration]);

	const close = () => {
		// Return focus to the trigger before the content unmounts, so it is not
		// lost to the document body. The focus handler's reopen is overridden by
		// the state updates below, which React batches after it.
		const activeElement = document.activeElement;
		if (activeElement && contentNode?.contains(activeElement)) {
			triggerNode?.focus();
		}
		setHoverIntent(false);
		setHoveredState(false);
		setFocused(false);
		setPinned(false);
	};

	const toggleFromClick = (fromKeyboard: boolean) => {
		// A mouse click on a hover-opened popover keeps it open. From the
		// keyboard, Enter and Space toggle it like a disclosure.
		if (pinned || (fromKeyboard && open)) {
			close();
			return;
		}
		setPinned(true);
	};

	const focusContent = () => {
		const first = contentNode ? getTabbables(contentNode)[0] : undefined;
		first?.focus();
		return Boolean(first);
	};

	const focusAfterTrigger = () => {
		if (triggerNode) {
			const tabbables = getTabbables(document).filter(
				(el) => !contentNode?.contains(el),
			);
			const next = tabbables[tabbables.indexOf(triggerNode) + 1];
			if (next) {
				next.focus();
			} else {
				triggerNode.blur();
			}
		}
		close();
	};

	return (
		<InteractiveTooltipContext.Provider
			value={{
				open,
				triggerId,
				contentId,
				setTriggerNode,
				setContentNode,
				setHovered: setHoverIntent,
				openFromFocus: () => setFocused(true),
				toggleFromClick,
				close,
				focusContent,
				focusAfterTrigger,
				focusTrigger: () => triggerNode?.focus(),
			}}
		>
			<PopoverPrimitive.Root
				open={open}
				onOpenChange={(next) => {
					if (!next) {
						close();
					}
				}}
			>
				{children}
			</PopoverPrimitive.Root>
		</InteractiveTooltipContext.Provider>
	);
};

type InteractiveTooltipTriggerProps = React.ComponentProps<
	typeof PopoverPrimitive.Trigger
>;

const InteractiveTooltipTrigger: React.FC<InteractiveTooltipTriggerProps> = ({
	onPointerEnter,
	onPointerLeave,
	onPointerDown,
	onFocus,
	onClick,
	onKeyDown,
	...props
}) => {
	const { setTriggerNode, ...context } = useInteractiveTooltip();
	const isPointerDownRef = useRef(false);

	return (
		<PopoverPrimitive.Trigger
			{...props}
			id={context.triggerId}
			ref={setTriggerNode}
			aria-controls={context.open ? context.contentId : undefined}
			aria-describedby={context.open ? context.contentId : undefined}
			onPointerEnter={(event) => {
				onPointerEnter?.(event);
				if (event.pointerType === "mouse") {
					context.setHovered(true);
				}
			}}
			onPointerLeave={(event) => {
				onPointerLeave?.(event);
				if (event.pointerType === "mouse") {
					context.setHovered(false);
				}
			}}
			onPointerDown={(event) => {
				onPointerDown?.(event);
				// Focus caused by a pointer press is handled by the click.
				isPointerDownRef.current = true;
				document.addEventListener(
					"pointerup",
					() => {
						isPointerDownRef.current = false;
					},
					{ once: true },
				);
			}}
			onFocus={(event) => {
				onFocus?.(event);
				if (!isPointerDownRef.current) {
					context.openFromFocus();
				}
			}}
			onClick={(event) => {
				onClick?.(event);
				// Keep the click from activating a surrounding clickable row, and
				// replace Radix's toggle with hover-aware pinning.
				event.stopPropagation();
				event.preventDefault();
				context.toggleFromClick(event.detail === 0);
			}}
			onKeyDown={(event) => {
				onKeyDown?.(event);
				if (event.key === "Enter" || event.key === " ") {
					event.stopPropagation();
				}
				if (
					event.key === "Tab" &&
					!event.shiftKey &&
					context.open &&
					context.focusContent()
				) {
					event.preventDefault();
				}
			}}
		/>
	);
};

type InteractiveTooltipContentProps = React.ComponentProps<
	typeof PopoverPrimitive.Content
> & {
	disablePortal?: boolean;
};

const InteractiveTooltipContent: React.FC<InteractiveTooltipContentProps> = ({
	className,
	sideOffset = 4,
	onPointerEnter,
	onPointerLeave,
	onClick,
	onKeyDown,
	disablePortal,
	...props
}) => {
	const { setContentNode, ...context } = useInteractiveTooltip();

	const content = (
		<PopoverPrimitive.Content
			aria-labelledby={props["aria-label"] ? undefined : context.triggerId}
			sideOffset={sideOffset}
			collisionPadding={8}
			{...props}
			id={context.contentId}
			ref={setContentNode}
			className={cn(tooltipContentClassName, className)}
			// Hover and focus open the popover without moving focus into it, and
			// `close` already restores focus when needed.
			onOpenAutoFocus={(event) => event.preventDefault()}
			onCloseAutoFocus={(event) => event.preventDefault()}
			onPointerEnter={(event) => {
				onPointerEnter?.(event);
				if (event.pointerType === "mouse") {
					context.setHovered(true);
				}
			}}
			onPointerLeave={(event) => {
				onPointerLeave?.(event);
				if (event.pointerType === "mouse") {
					context.setHovered(false);
				}
			}}
			onClick={(event) => {
				onClick?.(event);
				// React events bubble through portals, so keep interactions
				// inside the popover from activating a surrounding row.
				event.stopPropagation();
			}}
			onKeyDown={(event) => {
				onKeyDown?.(event);
				event.stopPropagation();
				if (event.key !== "Tab") {
					return;
				}
				const tabbables = getTabbables(event.currentTarget);
				const active = document.activeElement;
				if (!event.shiftKey && active === tabbables.at(-1)) {
					event.preventDefault();
					context.focusAfterTrigger();
				} else if (event.shiftKey && active === tabbables[0]) {
					event.preventDefault();
					context.focusTrigger();
				}
			}}
		/>
	);

	return disablePortal ? (
		content
	) : (
		<PopoverPrimitive.Portal>{content}</PopoverPrimitive.Portal>
	);
};

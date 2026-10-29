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

/** Grace period so the pointer can move from the trigger into the content. */
const HOVER_CLOSE_DELAY = 150;

const FOCUSABLE_ELEMENTS =
	'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/**
 * Returns an event handler that calls the consumer's handler from props, if
 * any, and then the tooltip's internal handler. Triggers are usually rendered
 * `asChild` around buttons or links that may have their own handlers, so the
 * tooltip must not replace them.
 */
const composeEventHandlers =
	<E,>(
		externalHandler: ((event: E) => void) | undefined,
		internalHandler: (event: E) => void,
	) =>
	(event: E) => {
		externalHandler?.(event);
		internalHandler(event);
	};

type InteractiveContextValue = {
	open: boolean;
	id: string;
	trigger: HTMLButtonElement | null;
	content: HTMLDivElement | null;
	setTrigger: (node: HTMLButtonElement | null) => void;
	setContent: (node: HTMLDivElement | null) => void;
	setIsPointerOver: (isPointerOver: boolean) => void;
	setFocused: (focused: boolean) => void;
	toggle: (fromKeyboard: boolean) => void;
	close: () => void;
};

/** Set by `<Tooltip interactive>`, null for plain tooltips. */
const InteractiveContext = createContext<InteractiveContextValue | null>(null);

type TooltipProps = React.ComponentProps<typeof TooltipPrimitive.Root> & {
	/**
	 * Use when the content has links or other focusable elements, which the
	 * ARIA tooltip pattern does not allow. It renders a non-modal popover that
	 * looks and opens like a tooltip, but Tab moves into the content, a click
	 * or Enter keeps it open, and Escape closes it. Only `delayDuration` is
	 * supported. Avoid setting `interactive` to true on triggers that are
	 * interactive in some other way (e.g., a link or button), because clicking
	 * the trigger pins the tooltip.
	 */
	interactive?: boolean;
};

export const Tooltip: React.FC<TooltipProps> = ({ interactive, ...props }) =>
	interactive ? (
		<InteractiveTooltip delayDuration={props.delayDuration}>
			{props.children}
		</InteractiveTooltip>
	) : (
		<InteractiveContext.Provider value={null}>
			<TooltipPrimitive.Root {...props} />
		</InteractiveContext.Provider>
	);

const InteractiveTooltip: React.FC<{
	children: React.ReactNode;
	delayDuration?: number;
}> = ({ children, delayDuration = TOOLTIP_DELAY_DURATION }) => {
	// Whether the mouse is over the trigger or the content right now.
	const [isPointerOver, setIsPointerOver] = useState(false);
	// Whether hover keeps the tooltip open. It follows `isPointerOver` after a
	// delay, so moving between the trigger and the content does not flicker.
	const [hovered, setHovered] = useState(false);
	const [focused, setFocused] = useState(false);
	const [pinned, setPinned] = useState(false);
	const [trigger, setTrigger] = useState<HTMLButtonElement | null>(null);
	const [content, setContent] = useState<HTMLDivElement | null>(null);
	const id = useId();
	const open = hovered || focused || pinned;

	// Sync `hovered` to `isPointerOver` after the open delay or the close grace
	// period. Popovers have no hover behavior, so this replaces the delays a
	// Radix tooltip would apply. If the pointer moves again first, the cleanup
	// cancels the pending update.
	useEffect(() => {
		if (isPointerOver === hovered) {
			return;
		}
		const openDelay = open ? 0 : delayDuration;
		const delay = isPointerOver ? openDelay : HOVER_CLOSE_DELAY;
		const timer = window.setTimeout(() => setHovered(isPointerOver), delay);
		return () => window.clearTimeout(timer);
	}, [isPointerOver, hovered, open, delayDuration]);

	const close = () => {
		// Restore focus before the content unmounts so it is not lost to the body.
		if (content?.contains(document.activeElement)) {
			trigger?.focus();
		}
		setIsPointerOver(false);
		setHovered(false);
		setFocused(false);
		setPinned(false);
	};

	const toggle = (fromKeyboard: boolean) => {
		// A click keeps a hover-opened tooltip open; Enter and Space toggle it.
		if (pinned || (fromKeyboard && open)) {
			close();
		} else {
			setPinned(true);
		}
	};

	return (
		<InteractiveContext.Provider
			value={{
				open,
				id,
				trigger,
				content,
				setTrigger,
				setContent,
				setIsPointerOver,
				setFocused,
				toggle,
				close,
			}}
		>
			<PopoverPrimitive.Root
				open={open}
				onOpenChange={(next) => next || close()}
			>
				{children}
			</PopoverPrimitive.Root>
		</InteractiveContext.Provider>
	);
};

type TooltipTriggerProps = React.ComponentProps<
	typeof TooltipPrimitive.Trigger
>;

export const TooltipTrigger: React.FC<TooltipTriggerProps> = (props) =>
	useContext(InteractiveContext) ? (
		<InteractiveTooltipTrigger {...props} />
	) : (
		<TooltipPrimitive.Trigger {...props} />
	);

const InteractiveTooltipTrigger: React.FC<TooltipTriggerProps> = ({
	className,
	onPointerEnter,
	onPointerLeave,
	onPointerDown,
	onFocus,
	onClick,
	onKeyDown,
	...props
}) => {
	const { setTrigger, ...context } = useInteractiveContext();
	const isPointerDownRef = useRef(false);

	return (
		<PopoverPrimitive.Trigger
			{...props}
			id={`${context.id}-trigger`}
			ref={setTrigger}
			aria-controls={context.open ? context.id : undefined}
			aria-describedby={context.open ? context.id : undefined}
			className={cn(
				"m-0 inline-flex items-center border-0 bg-transparent p-0 text-inherit rounded-sm",
				"focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
				className,
			)}
			onPointerEnter={composeEventHandlers(onPointerEnter, (event) => {
				if (event.pointerType === "mouse") context.setIsPointerOver(true);
			})}
			onPointerLeave={composeEventHandlers(onPointerLeave, (event) => {
				if (event.pointerType === "mouse") context.setIsPointerOver(false);
			})}
			onPointerDown={(event) => {
				onPointerDown?.(event);
				// Focus from a pointer press is handled by the click instead.
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
				if (!isPointerDownRef.current) context.setFocused(true);
			}}
			onClick={composeEventHandlers(onClick, (event) => {
				// Replace Radix's open toggle with our own, and keep the click from
				// reaching click handlers on ancestors.
				event.preventDefault();
				event.stopPropagation();
				// `detail` is the click count. Browsers set it to 0 when Enter or
				// Space activates the button.
				const isKeyboardClick = event.detail === 0;
				context.toggle(isKeyboardClick);
			})}
			onKeyDown={composeEventHandlers(onKeyDown, (event) => {
				if (event.key === "Enter" || event.key === " ") {
					event.stopPropagation();
				}
				// The content is portaled, so move focus into it explicitly.
				const first =
					context.open && event.key === "Tab" && !event.shiftKey
						? context.content?.querySelector<HTMLElement>(FOCUSABLE_ELEMENTS)
						: null;
				if (first) {
					event.preventDefault();
					first.focus();
				}
			})}
		/>
	);
};

/** Surface styles shared by plain and interactive tooltips. */
const tooltipContentClassName = cn(
	"z-50 overflow-hidden rounded-md bg-surface-primary px-3 py-2 text-xs font-medium text-content-secondary",
	"border border-solid border-border animate-in fade-in-0 duration-150",
	"data-[state=closed]:animate-out data-[state=closed]:fade-out-0",
);

export const TooltipArrow = TooltipPrimitive.Arrow;

/** The side of the trigger a tooltip opens on. */
export type TooltipSide = NonNullable<
	React.ComponentProps<typeof TooltipPrimitive.Content>["side"]
>;

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
	const interactive = useContext(InteractiveContext);
	if (interactive) {
		return (
			<InteractiveTooltipContent
				className={className}
				sideOffset={sideOffset}
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

const InteractiveTooltipContent: React.FC<
	React.ComponentProps<typeof PopoverPrimitive.Content>
> = ({
	className,
	onPointerEnter,
	onPointerLeave,
	onClick,
	onKeyDown,
	...props
}) => {
	const { setContent, ...context } = useInteractiveContext();

	return (
		<PopoverPrimitive.Portal>
			<PopoverPrimitive.Content
				aria-labelledby={`${context.id}-trigger`}
				collisionPadding={8}
				{...props}
				id={context.id}
				ref={setContent}
				className={cn(tooltipContentClassName, className)}
				// Opening never moves focus, and `close` restores it when needed.
				onOpenAutoFocus={(event) => event.preventDefault()}
				onCloseAutoFocus={(event) => event.preventDefault()}
				onPointerEnter={composeEventHandlers(onPointerEnter, (event) => {
					if (event.pointerType === "mouse") context.setIsPointerOver(true);
				})}
				onPointerLeave={composeEventHandlers(onPointerLeave, (event) => {
					if (event.pointerType === "mouse") context.setIsPointerOver(false);
				})}
				// React events bubble through portals; clicking the tooltip should
				// not click its ancestors.
				onClick={composeEventHandlers(onClick, (event) =>
					event.stopPropagation(),
				)}
				onKeyDown={composeEventHandlers(onKeyDown, (event) => {
					event.stopPropagation();
					if (event.key !== "Tab") {
						return;
					}
					const focusables =
						event.currentTarget.querySelectorAll(FOCUSABLE_ELEMENTS);
					const edge = focusables[event.shiftKey ? 0 : focusables.length - 1];
					if (document.activeElement === edge) {
						// Leave through the trigger: Shift+Tab stops there, and Tab
						// continues from the trigger's place in the page.
						context.trigger?.focus();
						if (event.shiftKey) event.preventDefault();
					}
				})}
			/>
		</PopoverPrimitive.Portal>
	);
};

const useInteractiveContext = () => {
	const context = useContext(InteractiveContext);
	if (!context) {
		throw new Error(
			"Interactive tooltip parts must be inside <Tooltip interactive>",
		);
	}
	return context;
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

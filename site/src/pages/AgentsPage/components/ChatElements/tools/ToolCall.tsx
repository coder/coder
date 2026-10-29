import { cn } from "cn";
import {
	ChevronDownIcon,
	LoaderIcon,
	ShieldIcon,
	TriangleAlertIcon,
} from "lucide-react";
import {
	createContext,
	useContext,
	useId,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useMessageScrollerUserLayoutIntent } from "#/vendor/message-scroller";
import { Shimmer } from "../Shimmer";
import { TranscriptRow } from "../TranscriptRow";
import { ToolIcon } from "./ToolIcon";
import { followLogEnd, type ToolStatus } from "./utils";

/**
 * Shared display states for tool call rows.
 *
 * `preview` is an initial or externally controlled display state for
 * renderers that want content visible without treating the row as fully
 * expanded. The built-in header toggle only switches between
 * `collapsed` and `expanded`, so toggle callbacks never emit `preview`.
 */
export type ToolCallView = "collapsed" | "preview" | "expanded";

type ToolCallAriaLabel = string | ((expanded: boolean) => string);

type ToolCallContextValue = {
	active: boolean;
	ariaLabel?: ToolCallAriaLabel;
	collapsible: boolean;
	errorMessage?: string;
	expanded: boolean;
	failed: boolean;
	onToggle: (event: React.MouseEvent<HTMLElement>) => void;
	status: ToolStatus;
	view: ToolCallView;
};

const ToolCallContext = createContext<ToolCallContextValue | null>(null);

const ToolPolicyContext = createContext<{ hookRewritten: boolean }>({
	hookRewritten: false,
});

/**
 * Rows inside render as one-line summaries: no toggle, chevron, content, or
 * header actions. For previews whose full rows render elsewhere.
 */
const ToolCallSummaryContext = createContext(false);

const SummaryProvider: React.FC<{ children: React.ReactNode }> = ({
	children,
}) => (
	<ToolCallSummaryContext.Provider value>
		{children}
	</ToolCallSummaryContext.Provider>
);

// Some renderer branches render no `ToolCall.Header`, so emitting the
// badge here rather than in the header keeps attribution on those cards.
const PolicyProvider: React.FC<{
	hookRewritten: boolean;
	children: React.ReactNode;
}> = ({ hookRewritten, children }) => {
	const badgeId = useId();
	return (
		<ToolPolicyContext.Provider value={{ hookRewritten }}>
			{hookRewritten ? (
				<div role="group" aria-labelledby={badgeId}>
					<span
						id={badgeId}
						className="mb-0.5 flex w-fit items-center gap-1 rounded border border-solid border-border px-1 text-[11px] leading-4 text-content-secondary"
					>
						<ShieldIcon aria-hidden className="size-3 shrink-0" />
						Modified by policy
					</span>
					{children}
				</div>
			) : (
				children
			)}
		</ToolPolicyContext.Provider>
	);
};

const useToolCallContext = () => {
	const context = useContext(ToolCallContext);
	if (!context) {
		throw new Error(
			"ToolCall components must be rendered inside ToolCall.Root",
		);
	}
	return context;
};

/**
 * Props for {@link ToolCall.Root}.
 *
 * The root can be controlled with `expanded`, or uncontrolled
 * with `defaultView` and `defaultExpanded`. When both uncontrolled props
 * are provided, `defaultView` wins because it can represent the more
 * specific `preview` state.
 *
 * `hasContent` controls whether the header behaves like a toggle. When
 * it is false, the header stays static and content is never shown.
 *
 * Standard `div` attributes are forwarded to the wrapper element so
 * callers can attach semantics such as live region roles.
 */
type ToolCallRootProps = Omit<React.ComponentProps<"div">, "children"> & {
	children: React.ReactNode;
	status: ToolStatus;
	isError?: boolean;
	errorMessage?: string;
	hasContent: boolean;
	defaultExpanded?: boolean;
	defaultView?: ToolCallView;
	expanded?: boolean;
	onExpandedChange?: (expanded: boolean) => void;
	ariaLabel?: ToolCallAriaLabel;
	/** At the transcript's end, expanding also resumes following new output. */
	revealEndOnExpand?: boolean;
};

/**
 * Provides shared state for tool-call rows and renders the wrapper div.
 *
 * The wrapper tracks the current display state, derives `expanded` from
 * it, and forwards wrapper attributes like `role` or `aria-live` to the
 * rendered `div`.
 */
const Root: React.FC<ToolCallRootProps> = ({
	children,
	status,
	isError = false,
	errorMessage,
	hasContent,
	defaultExpanded = false,
	defaultView,
	expanded: expandedProp,
	onExpandedChange,
	ariaLabel,
	revealEndOnExpand = false,
	className,
	...divProps
}) => {
	const [uncontrolledView, setUncontrolledView] = useState<ToolCallView>(
		defaultView ?? (defaultExpanded ? "expanded" : "collapsed"),
	);
	const controlledView =
		expandedProp === undefined
			? undefined
			: expandedProp
				? "expanded"
				: "collapsed";
	const view = controlledView ?? uncontrolledView;
	const summary = useContext(ToolCallSummaryContext);
	const expanded = view !== "collapsed";
	const collapsible = hasContent && !summary;
	const active = status === "running";
	const failed = status !== "running" && (isError || status === "error");
	const userLayoutIntent = useMessageScrollerUserLayoutIntent();
	const onToggle = (event: React.MouseEvent<HTMLElement>) => {
		userLayoutIntent(event.currentTarget, {
			revealEnd: revealEndOnExpand && !expanded,
		});
		const nextView: ToolCallView = expanded ? "collapsed" : "expanded";
		if (controlledView === undefined) {
			setUncontrolledView(nextView);
		}
		onExpandedChange?.(nextView !== "collapsed");
	};

	return (
		<ToolCallContext.Provider
			value={{
				active,
				ariaLabel,
				collapsible,
				errorMessage,
				expanded,
				failed,
				onToggle,
				status,
				view,
			}}
		>
			<div className={className} {...divProps}>
				{children}
			</div>
		</ToolCallContext.Provider>
	);
};
type ToolCallHeaderRowProps = {
	children: React.ReactNode;
	className?: string;
};

const HeaderRow: React.FC<ToolCallHeaderRowProps> = ({
	children,
	className,
}) => (
	<TranscriptRow className={cn("gap-2 text-content-secondary", className)}>
		{children}
	</TranscriptRow>
);

type ToolCallHeaderButtonProps = {
	children: React.ReactNode;
	className?: string;
	alwaysButton?: boolean;
};

const HeaderButton: React.FC<ToolCallHeaderButtonProps> = ({
	children,
	className,
	alwaysButton = false,
}) => {
	const { ariaLabel, collapsible, expanded, onToggle } = useToolCallContext();
	const { hookRewritten } = useContext(ToolPolicyContext);
	const summary = useContext(ToolCallSummaryContext);
	const resolvedAriaLabel =
		typeof ariaLabel === "function" ? ariaLabel(expanded) : ariaLabel;
	const buttonAriaLabel =
		resolvedAriaLabel && hookRewritten
			? `${resolvedAriaLabel}, modified by policy`
			: resolvedAriaLabel;
	if (summary || (!collapsible && !alwaysButton)) {
		return (
			<HeaderRow className={cn("min-w-0", className)}>{children}</HeaderRow>
		);
	}

	return (
		<TranscriptRow
			asChild
			className={cn(
				"m-0 min-w-0 max-w-full gap-2 border-0 bg-transparent p-0 text-left font-[inherit] text-inherit text-content-secondary transition-colors",
				collapsible && "cursor-pointer hover:text-content-primary",
				className,
			)}
		>
			<button
				type="button"
				aria-expanded={collapsible ? expanded : undefined}
				aria-label={buttonAriaLabel}
				onClick={collapsible ? onToggle : undefined}
			>
				{children}
			</button>
		</TranscriptRow>
	);
};

type ToolCallLeadingIconProps = {
	name?: string;
	children?: React.ReactNode;
	iconUrl?: string;
	serverName?: string;
};

const LeadingIcon: React.FC<ToolCallLeadingIconProps> = ({
	name,
	children,
	iconUrl,
	serverName,
}) => {
	const { active } = useToolCallContext();
	if (children) {
		return children;
	}
	if (!name) {
		return null;
	}

	return (
		<ToolIcon
			name={name}
			isRunning={active}
			iconUrl={iconUrl}
			serverName={serverName}
		/>
	);
};

type ToolCallLabelProps = {
	children: React.ReactNode;
	className?: string;
};

const Label: React.FC<ToolCallLabelProps> = ({ children, className }) => {
	const { active } = useToolCallContext();
	const labelClassName = cn(
		"min-w-0 truncate text-[13px] leading-6",
		className,
	);
	if (active && typeof children === "string") {
		return (
			<Shimmer as="span" className={labelClassName}>
				{children}
			</Shimmer>
		);
	}

	return <span className={labelClassName}>{children}</span>;
};

type ToolCallStatusProps = {
	className?: string;
};

const Status: React.FC<ToolCallStatusProps> = ({ className }) => {
	const { active, errorMessage, failed } = useToolCallContext();
	const message = errorMessage || "Tool call failed";
	return (
		<>
			{active && (
				<LoaderIcon
					aria-label="Tool call running"
					role="img"
					className={cn(
						"size-3.5 shrink-0 animate-spin motion-reduce:animate-none text-current",
						className,
					)}
				/>
			)}
			{failed && (
				<Tooltip>
					<TooltipTrigger asChild>
						<span
							aria-label={message}
							role="img"
							className={cn("flex shrink-0 text-current", className)}
						>
							<TriangleAlertIcon aria-hidden className="size-3.5 shrink-0" />
						</span>
					</TooltipTrigger>
					<TooltipContent>{message}</TooltipContent>
				</Tooltip>
			)}
		</>
	);
};

const Chevron: React.FC<{ className?: string }> = ({ className }) => {
	const { collapsible, expanded } = useToolCallContext();
	if (!collapsible) {
		return null;
	}
	return (
		<ChevronDownIcon
			className={cn(
				"order-last size-3 shrink-0 text-current transition-transform",
				expanded ? "rotate-0" : "-rotate-90",
				className,
			)}
		/>
	);
};

const Actions: React.FC<{ children: React.ReactNode; className?: string }> = ({
	children,
	className,
}) => (
	<div className={cn("flex shrink-0 items-center gap-1", className)}>
		{children}
	</div>
);

const HeaderActions: React.FC<{
	children: React.ReactNode;
	className?: string;
}> = ({ children, className }) => {
	if (useContext(ToolCallSummaryContext)) {
		return null;
	}
	return <Actions className={cn("ml-auto", className)}>{children}</Actions>;
};

const HeaderLayout: React.FC<{
	children: React.ReactNode;
	className?: string;
}> = ({ children, className }) => (
	<div className={cn("flex w-full items-center gap-2", className)}>
		{children}
	</div>
);

type ToolCallStateProps = {
	children: (state: ToolCallContextValue) => React.ReactNode;
};

/**
 * Render-prop access to the current tool-call state.
 *
 * This exposes the same derived state that the shared primitives use,
 * including the resolved view, whether the row is expanded, and whether
 * the row is considered active or failed.
 */
const State: React.FC<ToolCallStateProps> = ({ children }) =>
	children(useToolCallContext());

type ToolCallHeaderProps = {
	iconName?: string;
	label: React.ReactNode;
	iconUrl?: string;
	serverName?: string;
	secondaryLabel?: React.ReactNode;
	trailing?: React.ReactNode;
	showStatus?: boolean;
};

/**
 * Convenience header that renders the standard leading icon, label,
 * optional secondary label, status indicator, trailing content, and
 * chevron.
 *
 * Use this when a tool follows the default header layout. Callers that
 * need custom emphasis or status colors can compose the lower-level
 * primitives directly instead.
 */
const Header: React.FC<ToolCallHeaderProps> = ({
	iconName,
	label,
	iconUrl,
	serverName,
	secondaryLabel,
	trailing,
	showStatus = true,
}) => {
	return (
		<HeaderButton>
			<LeadingIcon name={iconName} iconUrl={iconUrl} serverName={serverName} />
			<Label>{label}</Label>
			{secondaryLabel}
			{showStatus && <Status />}
			{trailing}
			<Chevron />
		</HeaderButton>
	);
};
type ToolCallContentProps = {
	children: React.ReactNode;
};

const Content: React.FC<ToolCallContentProps> = ({ children }) => {
	const { collapsible, expanded } = useToolCallContext();
	if (!collapsible || !expanded) {
		return null;
	}
	return children;
};

type ToolCallScrollableProps = Omit<
	React.ComponentProps<typeof ScrollArea>,
	| "viewportAriaLabel"
	| "viewportRef"
	| "viewportTabIndex"
	| "scrollBarClassName"
	| "horizontalScrollBarClassName"
> & {
	/** Accessible name of the scrollable region. */
	ariaLabel: string;
	/**
	 * Keeps the newest content in view as the content grows, while the
	 * reader is at the end.
	 */
	followEnd?: boolean;
};

/**
 * Keyboard-focusable scroll region for tool-call output. Callers own the
 * box styling, including the height cap on the viewport.
 */
const Scrollable: React.FC<ToolCallScrollableProps> = ({
	ariaLabel,
	followEnd = false,
	children,
	...props
}) => {
	const viewportRef = useRef<HTMLDivElement>(null);
	const contentRef = useRef<HTMLDivElement>(null);

	// A ResizeObserver also sees growth from children that rerender on
	// their own, such as smoothly streamed text.
	useLayoutEffect(() => {
		const content = contentRef.current;
		if (!followEnd || !content) {
			return;
		}
		let scrollHeight = followLogEnd(viewportRef.current, 0);
		const observer = new ResizeObserver(() => {
			scrollHeight = followLogEnd(viewportRef.current, scrollHeight);
		});
		observer.observe(content);
		return () => observer.disconnect();
	}, [followEnd]);

	return (
		<ScrollArea
			viewportRef={viewportRef}
			viewportTabIndex={0}
			viewportAriaLabel={ariaLabel}
			scrollBarClassName="w-1.5"
			horizontalScrollBarClassName="h-1.5"
			{...props}
		>
			{followEnd ? <div ref={contentRef}>{children}</div> : children}
		</ScrollArea>
	);
};

export const ToolCall = {
	Root,
	PolicyProvider,
	SummaryProvider,
	HeaderRow,
	HeaderButton,
	LeadingIcon,
	Label,
	Status,
	Chevron,
	Actions,
	HeaderActions,
	HeaderLayout,
	State,
	Header,
	Content,
	Scrollable,
};

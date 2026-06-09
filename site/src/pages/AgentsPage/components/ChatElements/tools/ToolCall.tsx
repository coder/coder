import { ChevronDownIcon, LoaderIcon, TriangleAlertIcon } from "lucide-react";
import {
	type ComponentPropsWithoutRef,
	createContext,
	type FC,
	type ReactNode,
	useContext,
	useState,
} from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { Shimmer } from "../Shimmer";
import { TranscriptRow } from "../TranscriptRow";
import type { SubagentIconKind } from "./subagentDescriptor";
import { ToolIcon } from "./ToolIcon";
import type { ToolStatus } from "./utils";

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
	onToggle: () => void;
	status: ToolStatus;
	view: ToolCallView;
};

const ToolCallContext = createContext<ToolCallContextValue | null>(null);

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
 * The root can be controlled with `view` or `expanded`, or uncontrolled
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
type ToolCallRootProps = Omit<ComponentPropsWithoutRef<"div">, "children"> & {
	children: ReactNode;
	status: ToolStatus;
	isError?: boolean;
	errorMessage?: string;
	hasContent?: boolean;
	defaultExpanded?: boolean;
	defaultView?: ToolCallView;
	expanded?: boolean;
	onExpandedChange?: (expanded: boolean) => void;
	onViewChange?: (view: ToolCallView) => void;
	ariaLabel?: ToolCallAriaLabel;
	view?: ToolCallView;
};

/**
 * Provides shared state for tool-call rows and renders the wrapper div.
 *
 * The wrapper tracks the current display state, derives `expanded` from
 * it, and forwards wrapper attributes like `role` or `aria-live` to the
 * rendered `div`.
 */
const Root: FC<ToolCallRootProps> = ({
	children,
	status,
	isError = false,
	errorMessage,
	hasContent = true,
	defaultExpanded = false,
	defaultView,
	expanded: expandedProp,
	onExpandedChange,
	onViewChange,
	ariaLabel,
	className,
	view: viewProp,
	...divProps
}) => {
	const [uncontrolledView, setUncontrolledView] = useState<ToolCallView>(
		defaultView ?? (defaultExpanded ? "expanded" : "collapsed"),
	);
	const controlledView =
		viewProp ??
		(expandedProp === undefined
			? undefined
			: expandedProp
				? "expanded"
				: "collapsed");
	const view = controlledView ?? uncontrolledView;
	const expanded = view !== "collapsed";
	const collapsible = hasContent;
	const active = status === "running";
	const failed = status !== "running" && (isError || status === "error");
	const onToggle = () => {
		const nextView: ToolCallView = expanded ? "collapsed" : "expanded";
		if (controlledView === undefined) {
			setUncontrolledView(nextView);
		}
		onViewChange?.(nextView);
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
	children: ReactNode;
	className?: string;
};

const HeaderRow: FC<ToolCallHeaderRowProps> = ({ children, className }) => (
	<TranscriptRow className={cn("gap-2 text-content-secondary", className)}>
		{children}
	</TranscriptRow>
);

type ToolCallHeaderButtonProps = {
	children: ReactNode;
	className?: string;
	alwaysButton?: boolean;
};

const HeaderButton: FC<ToolCallHeaderButtonProps> = ({
	children,
	className,
	alwaysButton = false,
}) => {
	const { ariaLabel, collapsible, expanded, onToggle } = useToolCallContext();
	if (!collapsible && !alwaysButton) {
		return (
			<HeaderRow className={cn("min-w-0", className)}>{children}</HeaderRow>
		);
	}

	return (
		<TranscriptRow
			asChild
			className={cn(
				"m-0 min-w-0 max-w-full gap-2 border-0 bg-transparent p-0 text-left font-[inherit] text-[inherit] text-content-secondary transition-colors",
				collapsible && "cursor-pointer hover:text-content-primary",
				className,
			)}
		>
			<button
				type="button"
				aria-expanded={collapsible ? expanded : undefined}
				aria-label={
					typeof ariaLabel === "function" ? ariaLabel(expanded) : ariaLabel
				}
				onClick={collapsible ? onToggle : undefined}
			>
				{children}
			</button>
		</TranscriptRow>
	);
};

type ToolCallLeadingIconProps = {
	name?: string;
	children?: ReactNode;
	iconUrl?: string;
	serverName?: string;
	subagentIconKind?: SubagentIconKind;
};

const LeadingIcon: FC<ToolCallLeadingIconProps> = ({
	name,
	children,
	iconUrl,
	serverName,
	subagentIconKind,
}) => {
	const { active, failed } = useToolCallContext();
	if (children) {
		return <>{children}</>;
	}
	if (!name) {
		return null;
	}

	return (
		<ToolIcon
			name={name}
			isError={failed}
			isRunning={active}
			iconUrl={iconUrl}
			serverName={serverName}
			subagentIconKind={subagentIconKind}
		/>
	);
};

type ToolCallLabelProps = {
	children: ReactNode;
	className?: string;
	shimmerWhenActive?: boolean;
};

const Label: FC<ToolCallLabelProps> = ({
	children,
	className,
	shimmerWhenActive = true,
}) => {
	const { active } = useToolCallContext();
	const labelClassName = cn(
		"min-w-0 truncate text-[13px] leading-6",
		className,
	);
	if (active && shimmerWhenActive && typeof children === "string") {
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
	errorMessage?: string;
};

const Status: FC<ToolCallStatusProps> = ({ className, errorMessage }) => {
	const {
		active,
		errorMessage: contextErrorMessage,
		failed,
	} = useToolCallContext();
	const message = errorMessage || contextErrorMessage || "Tool call failed";
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

const Chevron: FC<{ className?: string }> = ({ className }) => {
	const { collapsible, expanded } = useToolCallContext();
	if (!collapsible) {
		return null;
	}
	return (
		<ChevronDownIcon
			className={cn(
				"size-3 shrink-0 text-current transition-transform",
				expanded ? "rotate-0" : "-rotate-90",
				className,
			)}
		/>
	);
};

const Actions: FC<{ children: ReactNode; className?: string }> = ({
	children,
	className,
}) => (
	<div className={cn("flex shrink-0 items-center gap-1", className)}>
		{children}
	</div>
);

const HeaderActions: FC<{ children: ReactNode; className?: string }> = ({
	children,
	className,
}) => {
	return <Actions className={cn("ml-auto", className)}>{children}</Actions>;
};

const HeaderLayout: FC<{ children: ReactNode; className?: string }> = ({
	children,
	className,
}) => (
	<div className={cn("flex w-full items-center gap-2", className)}>
		{children}
	</div>
);

type ToolCallStateProps = {
	children: (state: ToolCallContextValue) => ReactNode;
};

/**
 * Render-prop access to the current tool-call state.
 *
 * This exposes the same derived state that the shared primitives use,
 * including the resolved view, whether the row is expanded, and whether
 * the row is considered active or failed.
 */
const State: FC<ToolCallStateProps> = ({ children }) =>
	children(useToolCallContext());

type ToolCallHeaderProps = {
	iconName?: string;
	label: ReactNode;
	iconUrl?: string;
	serverName?: string;
	subagentIconKind?: SubagentIconKind;
	secondaryLabel?: ReactNode;
	trailing?: ReactNode;
	showStatus?: boolean;
	headerClassName?: string;
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
const Header: FC<ToolCallHeaderProps> = ({
	iconName,
	label,
	iconUrl,
	serverName,
	subagentIconKind,
	secondaryLabel,
	trailing,
	showStatus = true,
	headerClassName,
}) => {
	return (
		<HeaderButton className={headerClassName}>
			<LeadingIcon
				name={iconName}
				iconUrl={iconUrl}
				serverName={serverName}
				subagentIconKind={subagentIconKind}
			/>
			<Label>{label}</Label>
			{secondaryLabel}
			{showStatus && <Status />}
			{trailing}
			<Chevron />
		</HeaderButton>
	);
};
type ToolCallContentProps = {
	children: ReactNode;
};

const Content: FC<ToolCallContentProps> = ({ children }) => {
	const { collapsible, expanded } = useToolCallContext();
	if (!collapsible || !expanded) {
		return null;
	}
	return <>{children}</>;
};

export const ToolCall = {
	Root,
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
};

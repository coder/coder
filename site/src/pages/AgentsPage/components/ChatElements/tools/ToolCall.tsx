import { ChevronDownIcon, LoaderIcon, TriangleAlertIcon } from "lucide-react";
import {
	createContext,
	type FC,
	type ReactNode,
	useContext,
	useState,
} from "react";
import type { AgentDisplayMode } from "#/api/typesGenerated";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { Shimmer } from "../Shimmer";
import { TranscriptRow } from "../TranscriptRow";
import {
	type AgentDisplayState,
	isAgentDisplayOpen,
	resolveAgentDisplayState,
} from "./displayMode";
import type { SubagentIconKind } from "./subagentDescriptor";
import { ToolIcon } from "./ToolIcon";
import type { ToolStatus } from "./utils";

type ToolCallAriaLabel = string | ((expanded: boolean) => string);

type ToolCallContextValue = {
	active: boolean;
	ariaLabel?: ToolCallAriaLabel;
	className?: string;
	collapsible: boolean;
	errorMessage?: string;
	expanded: boolean;
	failed: boolean;
	onToggle: () => void;
	status: ToolStatus;
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

type ToolCallRootProps = {
	children: ReactNode;
	status: ToolStatus;
	isError?: boolean;
	errorMessage?: string;
	hasContent?: boolean;
	defaultExpanded?: boolean;
	expanded?: boolean;
	onExpandedChange?: (expanded: boolean) => void;
	ariaLabel?: ToolCallAriaLabel;
	className?: string;
};

type ToolCallAgentDisplayModeRootProps = Omit<
	ToolCallRootProps,
	"defaultExpanded"
> & {
	displayMode: AgentDisplayMode | undefined;
	autoDisplayState: AgentDisplayState;
};

const Root: FC<ToolCallRootProps> = ({
	children,
	status,
	isError = false,
	errorMessage,
	hasContent = true,
	defaultExpanded = false,
	expanded: expandedProp,
	onExpandedChange,
	ariaLabel,
	className,
}) => {
	const [uncontrolledExpanded, setUncontrolledExpanded] =
		useState(defaultExpanded);
	const expanded = expandedProp ?? uncontrolledExpanded;
	const collapsible = hasContent;
	const failed = isError || status === "error";
	const active = status === "running" && !failed;
	const onToggle = () => {
		const nextExpanded = !expanded;
		if (expandedProp === undefined) {
			setUncontrolledExpanded(nextExpanded);
		}
		onExpandedChange?.(nextExpanded);
	};

	return (
		<ToolCallContext.Provider
			value={{
				active,
				ariaLabel,
				className,
				collapsible,
				errorMessage,
				expanded,
				failed,
				onToggle,
				status,
			}}
		>
			<div className={className}>{children}</div>
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

const HeaderChevron: FC<{ className?: string }> = ({ className }) => (
	<Chevron className={className} />
);

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
	preserveDefaultStatusIndicator?: boolean;
	headerClassName?: string;
};

const Header: FC<ToolCallHeaderProps> = ({
	iconName,
	label,
	iconUrl,
	serverName,
	subagentIconKind,
	secondaryLabel,
	trailing,
	preserveDefaultStatusIndicator = true,
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
			{preserveDefaultStatusIndicator && <Status />}
			{trailing}
			<HeaderChevron />
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

const AgentDisplayModeRoot: FC<ToolCallAgentDisplayModeRootProps> = ({
	displayMode,
	autoDisplayState,
	...props
}) => {
	const displayState = resolveAgentDisplayState(displayMode, autoDisplayState);
	return (
		<Root
			key={`${displayMode ?? "auto"}:${autoDisplayState}`}
			{...props}
			defaultExpanded={isAgentDisplayOpen(displayState)}
		/>
	);
};

export const ToolCall = {
	Root,
	AgentDisplayModeRoot,
	HeaderRow,
	HeaderButton,
	LeadingIcon,
	Label,
	Status,
	Chevron,
	Actions,
	HeaderActions,
	HeaderChevron,
	HeaderLayout,
	State,
	Header,
	Content,
};

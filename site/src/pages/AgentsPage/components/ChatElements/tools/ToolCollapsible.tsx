import { ChevronDownIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { useState } from "react";
import type { AgentDisplayMode } from "#/api/typesGenerated";
import { cn } from "#/utils/cn";
import { TranscriptRow } from "../TranscriptRow";
import {
	type AgentDisplayState,
	isAgentDisplayOpen,
	resolveAgentDisplayState,
} from "./displayMode";

type ToolCollapsibleAriaLabel = string | ((expanded: boolean) => string);
type ToolCollapsibleHeader = ReactNode | ((expanded: boolean) => ReactNode);

interface ToolCollapsibleProps {
	children: ReactNode;
	header: ToolCollapsibleHeader;
	headerActions?: ReactNode;
	headerStatus?: ReactNode;
	hasContent?: boolean;
	defaultExpanded?: boolean;
	expanded?: boolean;
	onExpandedChange?: (expanded: boolean) => void;
	ariaLabel?: ToolCollapsibleAriaLabel;
	className?: string;
	headerClassName?: string;
}

interface AgentDisplayModeToolCollapsibleProps
	extends Omit<ToolCollapsibleProps, "defaultExpanded"> {
	displayMode: AgentDisplayMode | undefined;
	autoDisplayState: AgentDisplayState;
}

export const AgentDisplayModeToolCollapsible: FC<
	AgentDisplayModeToolCollapsibleProps
> = ({ displayMode, autoDisplayState, ...props }) => {
	const displayState = resolveAgentDisplayState(displayMode, autoDisplayState);

	return (
		<ToolCollapsible
			key={`${displayMode ?? "auto"}:${autoDisplayState}`}
			{...props}
			defaultExpanded={isAgentDisplayOpen(displayState)}
		/>
	);
};

export const ToolCollapsible: FC<ToolCollapsibleProps> = ({
	children,
	header,
	headerActions,
	headerStatus,
	hasContent = true,
	defaultExpanded = false,
	expanded: expandedProp,
	onExpandedChange,
	ariaLabel,
	className,
	headerClassName,
}) => {
	const [uncontrolledExpanded, setUncontrolledExpanded] =
		useState(defaultExpanded);
	const expanded = expandedProp ?? uncontrolledExpanded;
	const renderedHeader =
		typeof header === "function" ? header(expanded) : header;
	const toggleExpanded = () => {
		const nextExpanded = !expanded;
		if (expandedProp === undefined) {
			setUncontrolledExpanded(nextExpanded);
		}
		onExpandedChange?.(nextExpanded);
	};
	const headerButton = hasContent ? (
		<TranscriptRow
			asChild
			className={cn(
				"m-0 cursor-pointer gap-2 border-0 bg-transparent p-0 text-left font-[inherit] text-[inherit] text-content-secondary transition-colors hover:text-content-primary",
				headerActions ? "min-w-0 flex-1" : "w-full",
				headerClassName,
			)}
		>
			<button
				type="button"
				aria-expanded={expanded}
				aria-label={
					typeof ariaLabel === "function" ? ariaLabel(expanded) : ariaLabel
				}
				onClick={toggleExpanded}
			>
				{renderedHeader}
				{headerStatus}
				<ChevronDownIcon
					className={cn(
						"size-3 shrink-0 text-current transition-transform",
						expanded ? "rotate-0" : "-rotate-90",
					)}
				/>
			</button>
		</TranscriptRow>
	) : (
		<TranscriptRow
			className={cn(
				"gap-2 text-content-secondary",
				headerActions && "min-w-0 flex-1",
				headerClassName,
			)}
		>
			{renderedHeader}
			{headerStatus}
		</TranscriptRow>
	);

	return (
		<div className={className}>
			{headerActions ? (
				<div className="flex w-full items-center gap-2">
					{headerButton}
					<div className="flex shrink-0 items-center gap-1">
						{headerActions}
					</div>
				</div>
			) : (
				headerButton
			)}
			{expanded && hasContent && children}
		</div>
	);
};

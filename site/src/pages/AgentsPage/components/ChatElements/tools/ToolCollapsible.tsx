import { ChevronDownIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { useState } from "react";
import type { AgentDisplayMode } from "#/api/typesGenerated";
import { cn } from "#/utils/cn";
import {
	type AgentDisplayState,
	isAgentDisplayOpen,
	resolveAgentDisplayState,
} from "./displayMode";

type ToolCollapsibleHeader = ReactNode | ((expanded: boolean) => ReactNode);

interface ToolCollapsibleProps {
	children: ReactNode;
	header: ToolCollapsibleHeader;
	hasContent?: boolean;
	defaultExpanded?: boolean;
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
	hasContent = true,
	defaultExpanded = false,
	className,
	headerClassName,
}) => {
	const [expanded, setExpanded] = useState(defaultExpanded);
	const renderedHeader =
		typeof header === "function" ? header(expanded) : header;
	return (
		<div className={className}>
			{hasContent ? (
				<button
					type="button"
					aria-expanded={expanded}
					onClick={() => setExpanded(!expanded)}
					className={cn(
						"border-0 bg-transparent p-0 m-0 font-[inherit] text-[inherit] text-left",
						"flex w-full items-center gap-2 cursor-pointer",
						"text-content-secondary transition-colors hover:text-content-primary",
						headerClassName,
					)}
				>
					{renderedHeader}
					<ChevronDownIcon
						className={cn(
							"h-3 w-3 shrink-0 text-current transition-transform",
							expanded ? "rotate-0" : "-rotate-90",
						)}
					/>
				</button>
			) : (
				<div
					className={cn(
						"flex items-center gap-2 text-content-secondary",
						headerClassName,
					)}
				>
					{renderedHeader}
				</div>
			)}
			{expanded && hasContent && children}
		</div>
	);
};

import { ChevronDownIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { useState } from "react";
import type * as TypesGen from "#/api/typesGenerated";
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
	displayMode?: TypesGen.AgentDisplayMode;
	autoDisplayState?: AgentDisplayState;
	className?: string;
	headerClassName?: string;
}

interface ToolCollapsibleInnerProps extends ToolCollapsibleProps {
	defaultOpen: boolean;
}

const getDisplayModeResetKey = (
	displayMode: TypesGen.AgentDisplayMode | undefined,
	autoDisplayState: AgentDisplayState | undefined,
): string => {
	if (displayMode === undefined && autoDisplayState === undefined) {
		return "legacy";
	}
	return `${displayMode ?? "auto"}:${autoDisplayState ?? "collapsed"}`;
};

const getDefaultOpen = ({
	displayMode,
	autoDisplayState,
	defaultExpanded = false,
}: Pick<
	ToolCollapsibleProps,
	"displayMode" | "autoDisplayState" | "defaultExpanded"
>): boolean => {
	if (displayMode === undefined && autoDisplayState === undefined) {
		return defaultExpanded;
	}
	return isAgentDisplayOpen(
		resolveAgentDisplayState(displayMode, autoDisplayState ?? "collapsed"),
	);
};

export const ToolCollapsible: FC<ToolCollapsibleProps> = (props) => {
	const resetKey = getDisplayModeResetKey(
		props.displayMode,
		props.autoDisplayState,
	);
	return (
		<ToolCollapsibleInner
			key={resetKey}
			{...props}
			defaultOpen={getDefaultOpen(props)}
		/>
	);
};

const ToolCollapsibleInner: FC<ToolCollapsibleInnerProps> = ({
	children,
	header,
	hasContent = true,
	defaultOpen,
	className,
	headerClassName,
}) => {
	const [expanded, setExpanded] = useState(defaultOpen);
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

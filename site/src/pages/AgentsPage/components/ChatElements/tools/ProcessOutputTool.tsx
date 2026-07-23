import { ChevronDownIcon, OctagonXIcon } from "lucide-react";
import type React from "react";
import { useState } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import {
	type AgentDisplayState,
	isAgentDisplayFullyExpanded,
	resolveAgentDisplayState,
} from "./displayMode";
import { ToolCall } from "./ToolCall";
import { COLLAPSED_OUTPUT_HEIGHT, signalTooltipLabel } from "./utils";

type ProcessOutputToolProps = {
	output: string;
	isRunning: boolean;
	exitCode: number | null;
	isError: boolean;
	errorMessage?: string;
	killedBySignal?: "kill" | "terminate";
	shellToolDisplayMode?: TypesGen.AgentDisplayMode;
};

type ProcessOutputToolInnerProps = ProcessOutputToolProps & {
	defaultView: AgentDisplayState;
	outputInitiallyFullyExpanded: boolean;
};

export const ProcessOutputTool: React.FC<ProcessOutputToolProps> = (props) => {
	const autoDisplayState: AgentDisplayState =
		props.output.length > 0 ? "preview" : "collapsed";
	const resolvedDisplayState = resolveAgentDisplayState(
		props.shellToolDisplayMode,
		autoDisplayState,
	);
	return (
		<ProcessOutputToolInner
			key={`${props.shellToolDisplayMode ?? "auto"}:${autoDisplayState}`}
			{...props}
			defaultView={resolvedDisplayState}
			outputInitiallyFullyExpanded={isAgentDisplayFullyExpanded(
				resolvedDisplayState,
			)}
		/>
	);
};

const ProcessOutputToolInner: React.FC<ProcessOutputToolInnerProps> = ({
	output,
	isRunning,
	exitCode,
	isError,
	errorMessage,
	killedBySignal,
	defaultView,
	outputInitiallyFullyExpanded,
}) => {
	const [outputFullyExpanded, setOutputFullyExpanded] = useState(
		outputInitiallyFullyExpanded,
	);
	const hasOutput = output.length > 0;

	const [overflows, setOverflows] = useState(false);
	const measureRef = (node: HTMLPreElement | null) => {
		if (node) {
			setOverflows(node.scrollHeight > COLLAPSED_OUTPUT_HEIGHT);
		}
	};

	const showExitCode = exitCode !== null && exitCode !== 0;
	const toggleOutputExpansion = () => {
		setOutputFullyExpanded((expanded) => !expanded);
	};
	const hasHeaderActions = Boolean(killedBySignal) || showExitCode || hasOutput;

	return (
		<ToolCall.Root
			className="group/proc w-full"
			status={isRunning ? "running" : isError ? "error" : "completed"}
			isError={isError}
			errorMessage={errorMessage || "Failed to read process output"}
			hasContent={hasOutput}
			defaultView={defaultView}
			ariaLabel={(expanded) =>
				expanded ? "Collapse process output" : "Expand process output"
			}
		>
			<ToolCall.HeaderLayout>
				<ToolCall.HeaderButton>
					<ToolCall.LeadingIcon name="process_output" />
					<ToolCall.Label>Process output</ToolCall.Label>
					<ToolCall.Status />
					<ToolCall.Chevron />
				</ToolCall.HeaderButton>
				{hasHeaderActions && (
					<ToolCall.HeaderActions>
						{killedBySignal && !isRunning && (
							<Tooltip>
								<TooltipTrigger asChild>
									<OctagonXIcon className="size-3.5 shrink-0 text-content-secondary" />
								</TooltipTrigger>
								<TooltipContent>
									{signalTooltipLabel(killedBySignal)}
								</TooltipContent>
							</Tooltip>
						)}
						{showExitCode && (
							<span className="rounded px-1.5 py-0.5 font-mono text-2xs leading-none bg-surface-red text-content-destructive">
								exit {exitCode}
							</span>
						)}
						{hasOutput && (
							<CopyButton
								text={output}
								label="Copy output"
								className="-my-0.5 size-6 p-0 opacity-0 transition-opacity hover:bg-surface-tertiary group-hover/proc:opacity-100 focus-visible:opacity-100"
							/>
						)}
					</ToolCall.HeaderActions>
				)}
			</ToolCall.HeaderLayout>
			<ToolCall.Content>
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default text-2xs"
					viewportClassName={outputFullyExpanded ? "max-h-64" : ""}
					scrollBarClassName="w-1.5"
				>
					<pre
						ref={measureRef}
						style={
							outputFullyExpanded
								? undefined
								: { maxHeight: COLLAPSED_OUTPUT_HEIGHT, overflow: "hidden" }
						}
						className={cn(
							"m-0 border-0 whitespace-pre-wrap break-all bg-transparent px-3 py-2 font-mono text-xs",
							isError ? "text-content-destructive" : "text-content-secondary",
						)}
					>
						{output}
					</pre>
				</ScrollArea>
				{overflows && (
					<button
						type="button"
						aria-expanded={outputFullyExpanded}
						onClick={toggleOutputExpansion}
						className="border-0 bg-transparent m-0 mt-0.5 font-[inherit] text-[inherit] flex w-full cursor-pointer items-center justify-center rounded-md py-0.5 text-content-secondary transition-colors hover:bg-surface-secondary hover:text-content-primary"
						aria-label={
							outputFullyExpanded
								? "Collapse full process output"
								: "Expand full process output"
						}
					>
						<ChevronDownIcon
							className={cn(
								"size-3 transition-transform",
								outputFullyExpanded && "rotate-180",
							)}
						/>
					</button>
				)}
			</ToolCall.Content>
		</ToolCall.Root>
	);
};

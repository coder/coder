import { ChevronDownIcon, LoaderIcon, OctagonXIcon } from "lucide-react";
import type React from "react";
import { useRef, useState } from "react";
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
	resolveAgentDisplayState,
} from "./displayMode";
import { AgentDisplayModeToolCollapsible } from "./ToolCollapsible";
import { COLLAPSED_OUTPUT_HEIGHT, signalTooltipLabel } from "./utils";

type ProcessOutputToolProps = {
	output: string;
	isRunning: boolean;
	exitCode: number | null;
	isError: boolean;
	killedBySignal?: "kill" | "terminate";
	shellToolDisplayMode?: TypesGen.AgentDisplayMode;
};

type ProcessOutputToolInnerProps = ProcessOutputToolProps & {
	autoDisplayState: AgentDisplayState;
	outputInitiallyExpanded: boolean;
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
			autoDisplayState={autoDisplayState}
			outputInitiallyExpanded={resolvedDisplayState === "expanded"}
		/>
	);
};

const ProcessOutputToolInner: React.FC<ProcessOutputToolInnerProps> = ({
	output,
	isRunning,
	exitCode,
	isError,
	killedBySignal,
	shellToolDisplayMode,
	autoDisplayState,
	outputInitiallyExpanded,
}) => {
	const [outputExpanded, setOutputExpanded] = useState(outputInitiallyExpanded);
	const outputRef = useRef<HTMLPreElement | null>(null);
	const hasOutput = output.length > 0;

	const [overflows, setOverflows] = useState(false);
	const measureRef = (node: HTMLPreElement | null) => {
		outputRef.current = node;
		if (node) {
			setOverflows(node.scrollHeight > COLLAPSED_OUTPUT_HEIGHT);
		}
	};

	const showExitCode = exitCode !== null && exitCode !== 0;
	const toggleOutputExpansion = () => {
		setOutputExpanded((expanded) => !expanded);
	};
	const hasHeaderActions =
		isRunning || Boolean(killedBySignal) || showExitCode || hasOutput;

	return (
		<AgentDisplayModeToolCollapsible
			className="group/proc w-full"
			hasContent={hasOutput}
			displayMode={shellToolDisplayMode}
			autoDisplayState={autoDisplayState}
			ariaLabel={(expanded) =>
				expanded ? "Collapse process output" : "Expand process output"
			}
			header={<span className="text-[13px]">Process output</span>}
			headerActions={
				hasHeaderActions ? (
					<>
						{isRunning && (
							<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
						)}
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
						{hasOutput && <CopyButton text={output} label="Copy output" />}
					</>
				) : undefined
			}
		>
			<ScrollArea
				className="mt-1.5 rounded-md border border-solid border-border-default text-2xs"
				viewportClassName={outputExpanded ? "max-h-64" : ""}
				scrollBarClassName="w-1.5"
			>
				<pre
					ref={measureRef}
					style={
						outputExpanded
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
					aria-expanded={outputExpanded}
					onClick={toggleOutputExpansion}
					className="border-0 bg-transparent m-0 mt-0.5 font-[inherit] text-[inherit] flex w-full cursor-pointer items-center justify-center rounded-md py-0.5 text-content-secondary transition-colors hover:bg-surface-secondary hover:text-content-primary"
					aria-label={
						outputExpanded
							? "Collapse full process output"
							: "Expand full process output"
					}
				>
					<ChevronDownIcon
						className={cn(
							"h-3 w-3 transition-transform",
							outputExpanded && "rotate-180",
						)}
					/>
				</button>
			)}
		</AgentDisplayModeToolCollapsible>
	);
};

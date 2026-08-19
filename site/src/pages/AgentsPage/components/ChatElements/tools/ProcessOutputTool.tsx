import { OctagonXIcon } from "lucide-react";
import type React from "react";
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
import { ToolCall } from "./ToolCall";
import { sanitizeExecuteModelIntent, signalTooltipLabel } from "./utils";

type ProcessOutputToolProps = {
	output: string;
	command?: string;
	modelIntent?: string;
	isRunning: boolean;
	exitCode: number | null;
	isError: boolean;
	errorMessage?: string;
	killedBySignal?: "kill" | "terminate";
	shellToolDisplayMode?: TypesGen.AgentDisplayMode;
};

const getProcessOutputLabel = ({
	command,
	modelIntent,
	isRunning,
	isFailed,
}: {
	command: string | undefined;
	modelIntent: string | undefined;
	isRunning: boolean;
	isFailed: boolean;
}): string => {
	const trimmedCommand = command?.trim() ?? "";
	const intent = modelIntent
		? sanitizeExecuteModelIntent(modelIntent, trimmedCommand)
		: "";
	if (intent) {
		return intent;
	}
	if (!trimmedCommand) {
		return "Process output";
	}
	if (isRunning) {
		return `Checking ${trimmedCommand}`;
	}
	return `${isFailed ? "Failed" : "Checked"} ${trimmedCommand}`;
};

export const ProcessOutputTool: React.FC<ProcessOutputToolProps> = ({
	output,
	command,
	modelIntent,
	isRunning,
	exitCode,
	isError,
	errorMessage,
	killedBySignal,
	shellToolDisplayMode,
}) => {
	const autoDisplayState: AgentDisplayState =
		output.length > 0 ? "preview" : "collapsed";
	const defaultView = resolveAgentDisplayState(
		shellToolDisplayMode,
		autoDisplayState,
	);

	// A clean exit is the expected outcome of a check, so only
	// failures earn a badge. The label verb carries the rest.
	const isFailed = exitCode !== null && exitCode !== 0;
	const hasOutput = output.length > 0;
	const hasHeaderActions = Boolean(killedBySignal) || isFailed || hasOutput;

	return (
		<ToolCall.Root
			key={`${shellToolDisplayMode ?? "auto"}:${autoDisplayState}`}
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
					<ToolCall.Label>
						{getProcessOutputLabel({
							command,
							modelIntent,
							isRunning,
							isFailed,
						})}
					</ToolCall.Label>
					<ToolCall.Status />
					<ToolCall.Chevron />
				</ToolCall.HeaderButton>
				{hasHeaderActions && (
					<ToolCall.HeaderActions>
						{killedBySignal && !isRunning && (
							<Tooltip>
								<TooltipTrigger asChild>
									<span
										aria-label={signalTooltipLabel(killedBySignal)}
										role="img"
										className="flex shrink-0 items-center text-content-secondary"
									>
										<OctagonXIcon aria-hidden className="size-3.5 shrink-0" />
									</span>
								</TooltipTrigger>
								<TooltipContent>
									{signalTooltipLabel(killedBySignal)}
								</TooltipContent>
							</Tooltip>
						)}
						{isFailed && (
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
					className="mt-2 rounded-xl bg-surface-secondary/60 text-2xs"
					viewportClassName="max-h-64"
					scrollBarClassName="w-1.5"
				>
					<pre
						className={cn(
							"m-0 border-0 whitespace-pre-wrap break-all bg-transparent px-3 py-2.5 font-mono text-xs leading-5",
							isError ? "text-content-destructive" : "text-content-secondary",
						)}
					>
						{output}
					</pre>
				</ScrollArea>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};

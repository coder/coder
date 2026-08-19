import { CircleCheckIcon, OctagonXIcon } from "lucide-react";
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
}: {
	command: string | undefined;
	modelIntent: string | undefined;
	isRunning: boolean;
}): string => {
	const trimmedCommand = command?.trim() ?? "";
	const intent = modelIntent
		? sanitizeExecuteModelIntent(modelIntent, trimmedCommand)
		: "";
	if (intent) {
		return trimmedCommand ? `${intent} on ${trimmedCommand}` : intent;
	}
	if (!trimmedCommand) {
		return "Process output";
	}
	return `${isRunning ? "Checking" : "Checked"} ${trimmedCommand}`;
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

	const showExitCode = exitCode !== null;
	const hasOutput = output.length > 0;
	const hasHeaderActions = Boolean(killedBySignal) || showExitCode || hasOutput;

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
						{getProcessOutputLabel({ command, modelIntent, isRunning })}
					</ToolCall.Label>
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
							<span
								className={cn(
									"flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-2xs leading-none",
									exitCode === 0
										? "text-content-secondary"
										: "bg-surface-red text-content-destructive",
								)}
							>
								{exitCode === 0 && (
									<CircleCheckIcon aria-hidden className="size-3.5 shrink-0" />
								)}
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

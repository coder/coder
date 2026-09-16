import { cn } from "cn";
import { OctagonXIcon } from "lucide-react";
import type React from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import {
	type AgentDisplayState,
	resolveAgentDisplayState,
} from "./displayMode";
import { ProcessChip } from "./ProcessChip";
import { TerminalOutput } from "./TerminalOutput";
import { ToolCall } from "./ToolCall";
import type { ExecuteTranscriptBlock } from "./toolVisibility";
import {
	formatShellDurationMs,
	sanitizeExecuteModelIntent,
	signalTooltipLabel,
	summarizeParsedCommands,
	type ToolStatus,
} from "./utils";

type ExecuteToolProps = {
	command: string;
	transcriptBlocks: readonly ExecuteTranscriptBlock[];
	status: ToolStatus;
	isError: boolean;
	errorText?: string;
	durationMs?: number;
	isBackgrounded?: boolean;
	killedBySignal?: "kill" | "terminate";
	modelIntent?: string;
	parsedCommands?: readonly string[][];
	shellToolDisplayMode?: TypesGen.AgentDisplayMode;
	processId?: string;
	/** A foreground wait expired without the process finishing. */
	timedOut?: boolean;
	/** Whether the result confirmed the process was still alive. */
	processRunning?: boolean;
	/** The model's requested wait limit, e.g. "30s". */
	waitLimit?: string;
};

export const ExecuteTool: React.FC<ExecuteToolProps> = ({
	command,
	transcriptBlocks,
	status,
	isError,
	errorText,
	durationMs,
	isBackgrounded = false,
	killedBySignal,
	modelIntent,
	parsedCommands,
	shellToolDisplayMode,
	processId,
	timedOut = false,
	processRunning = false,
	waitLimit,
}) => {
	const hasTranscriptBlocks = transcriptBlocks.length > 0;
	const autoDisplayState: AgentDisplayState =
		hasTranscriptBlocks ||
		status === "running" ||
		isBackgrounded ||
		killedBySignal
			? "preview"
			: "collapsed";
	const isRunning = status === "running";
	// The wait-limit suffix is dropped for timeouts: it reads as a completed
	// run duration, and wall time includes snapshot recovery beyond the limit.
	const showDuration = !isBackgrounded && !(timedOut && !isRunning);
	const durationLabel = showDuration ? formatShellDurationMs(durationMs) : "";
	const { commandLabel, durationSuffix } = getShellCommandLine({
		command,
		modelIntent,
		parsedCommands,
		durationLabel,
		isRunning,
		isError,
		isBackgrounded,
		timedOut,
		processRunning,
	});
	const defaultView = resolveAgentDisplayState(
		shellToolDisplayMode,
		autoDisplayState,
	);
	const showStillRunningChip = timedOut && processRunning && !isRunning;
	const statusUnknown = timedOut && !processRunning;
	const chipLabel = showStillRunningChip
		? waitLimit
			? `Stopped waiting after ${waitLimit}. The process kept running in the workspace.`
			: "Stopped waiting. The process kept running in the workspace."
		: statusUnknown
			? waitLimit
				? `Stopped waiting after ${waitLimit} and could not read the process state.`
				: "Stopped waiting and could not read the process state."
			: "";

	return (
		<ToolCall.Root
			key={`${shellToolDisplayMode ?? "auto"}:${autoDisplayState}`}
			className="group/exec grid w-full grid-cols-[minmax(0,1fr)_auto] items-start rounded-md bg-surface-primary font-sans font-normal text-xs leading-5"
			status={status}
			isError={isError}
			errorMessage={errorText || "Command failed"}
			hasContent
			defaultView={defaultView}
			ariaLabel={(expanded) =>
				expanded ? "Collapse command" : "Expand command"
			}
		>
			<ToolCall.HeaderLayout>
				<ToolCall.HeaderButton className="col-start-1 row-start-1 min-w-0 font-normal">
					<ToolCall.LeadingIcon name="execute" />
					<span className="flex min-w-0 items-baseline">
						<ToolCall.Label>{commandLabel}</ToolCall.Label>
						{durationSuffix && (
							<span className="ml-1 shrink-0 text-content-secondary">
								{durationSuffix}
							</span>
						)}
					</span>
					<ToolCall.Status />
					<ToolCall.Chevron />
				</ToolCall.HeaderButton>
				<ToolCall.HeaderActions>
					{showStillRunningChip && (
						<Tooltip>
							<TooltipTrigger asChild>
								<span
									aria-label={chipLabel}
									role="img"
									className="shrink-0 rounded bg-surface-secondary px-1.5 py-0.5 font-mono text-2xs leading-none text-content-secondary"
								>
									Still running
								</span>
							</TooltipTrigger>
							<TooltipContent>{chipLabel}</TooltipContent>
						</Tooltip>
					)}
					{statusUnknown && (
						<Tooltip>
							<TooltipTrigger asChild>
								<span
									aria-label={chipLabel}
									role="img"
									className="shrink-0 rounded bg-surface-secondary px-1.5 py-0.5 font-mono text-2xs leading-none text-content-secondary"
								>
									Status unknown
								</span>
							</TooltipTrigger>
							<TooltipContent>{chipLabel}</TooltipContent>
						</Tooltip>
					)}
					{processId && <ProcessChip processId={processId} />}
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
					<CopyButton
						text={command}
						label="Copy command"
						className="-my-0.5 size-6 p-0 opacity-0 transition-opacity hover:bg-surface-tertiary group-hover/exec:opacity-100 focus-visible:opacity-100"
					/>
				</ToolCall.HeaderActions>
			</ToolCall.HeaderLayout>
			<ToolCall.Content>
				<ShellTranscriptBody
					command={command}
					transcriptBlocks={transcriptBlocks}
					isError={isError}
					isRunning={isRunning}
					timedOutStillRunning={showStillRunningChip}
					waitLimit={waitLimit}
				/>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};

type ShellCommandLineInput = {
	command: string;
	modelIntent?: string;
	parsedCommands?: readonly string[][];
	durationLabel: string;
	isRunning: boolean;
	isError: boolean;
	isBackgrounded: boolean;
	timedOut: boolean;
	processRunning: boolean;
};

const getShellCommandLine = ({
	command,
	modelIntent,
	parsedCommands,
	durationLabel,
	isRunning,
	isError,
	isBackgrounded,
	timedOut,
	processRunning,
}: ShellCommandLineInput): { commandLabel: string; durationSuffix: string } => {
	const summary =
		parsedCommands && parsedCommands.length > 0
			? summarizeParsedCommands(parsedCommands)
			: "";
	const commandDisplay = summary || command;
	const intentLabel = sanitizeExecuteModelIntent(modelIntent, command);
	let commandLabel = intentLabel
		? `${intentLabel} using ${commandDisplay}`
		: `Ran ${commandDisplay}`;
	if (intentLabel && isBackgrounded) {
		commandLabel = `${intentLabel} in the background using ${commandDisplay}`;
	} else if (isBackgrounded) {
		commandLabel = `Started ${commandDisplay} in the background`;
	}
	// The timeout branch precedes the failure branch so a success:false
	// timeout can never claim the command failed.
	if (timedOut) {
		commandLabel = intentLabel
			? `${intentLabel} using ${commandDisplay}`
			: `Started ${commandDisplay}`;
		if (!processRunning) {
			commandLabel = `Started ${commandDisplay}`;
		}
	}
	if (!isRunning && isError && !timedOut) {
		commandLabel = `Failed to run ${commandDisplay}`;
	}

	return {
		commandLabel,
		durationSuffix: durationLabel ? ` for ${durationLabel}` : "",
	};
};

const ShellTranscriptBody: React.FC<{
	command: string;
	transcriptBlocks: readonly ExecuteTranscriptBlock[];
	isError: boolean;
	isRunning: boolean;
	timedOutStillRunning: boolean;
	waitLimit?: string;
}> = ({
	command,
	transcriptBlocks,
	isError,
	isRunning,
	timedOutStillRunning,
	waitLimit,
}) => {
	return (
		<TerminalOutput
			ariaLabel="Command output"
			command={command}
			className="col-start-1 col-span-2 mt-2"
			streaming={isRunning}
		>
			{timedOutStillRunning && (
				<p className="m-0 border-0 bg-transparent p-0 font-sans text-2xs leading-5 text-content-secondary">
					{waitLimit
						? `Reached the ${waitLimit} wait limit; the process was not stopped.`
						: "Reached the wait limit; the process was not stopped."}
				</p>
			)}
			{transcriptBlocks.map((block) => (
				<pre
					key={block.kind}
					className={cn(
						"m-0 whitespace-pre-wrap break-all border-0 bg-transparent p-0 font-mono text-xs font-normal leading-5",
						block.kind === "error" || isError
							? "text-content-destructive"
							: "text-content-secondary",
					)}
				>
					{block.text}
				</pre>
			))}
		</TerminalOutput>
	);
};

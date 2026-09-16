import { cn } from "cn";
import { OctagonXIcon } from "lucide-react";
import { type FC, useEffect, useState } from "react";
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
import { TerminalOutput } from "./TerminalOutput";
import { ToolCall } from "./ToolCall";
import type { ExecuteTranscriptBlock } from "./toolVisibility";
import {
	formatElapsedMs,
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
	/** ISO timestamp the call was emitted. Falls back to mount time when absent. */
	startedAt?: string;
	shellToolDisplayMode?: TypesGen.AgentDisplayMode;
};

export const ExecuteTool: FC<ExecuteToolProps> = ({
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
	startedAt,
	shellToolDisplayMode,
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
	const durationLabel = isBackgrounded ? "" : formatShellDurationMs(durationMs);
	const { commandLabel, durationSuffix } = getShellCommandLine({
		command,
		modelIntent,
		parsedCommands,
		durationLabel,
		isRunning,
		isError,
		isBackgrounded,
	});
	const defaultView = resolveAgentDisplayState(
		shellToolDisplayMode,
		autoDisplayState,
	);

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
					showElapsed={isRunning && !isBackgrounded}
					startedAt={startedAt}
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
};

const getShellCommandLine = ({
	command,
	modelIntent,
	parsedCommands,
	durationLabel,
	isRunning,
	isError,
	isBackgrounded,
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
	if (!isRunning && isError) {
		commandLabel = `Failed to run ${commandDisplay}`;
	}

	return {
		commandLabel,
		durationSuffix: durationLabel ? ` for ${durationLabel}` : "",
	};
};

const ShellTranscriptBody: FC<{
	command: string;
	transcriptBlocks: readonly ExecuteTranscriptBlock[];
	isError: boolean;
	isRunning: boolean;
	showElapsed: boolean;
	startedAt?: string;
}> = ({
	command,
	transcriptBlocks,
	isError,
	isRunning,
	showElapsed,
	startedAt,
}) => {
	return (
		<TerminalOutput
			ariaLabel="Command output"
			command={command}
			className="col-start-1 col-span-2 mt-2"
			streaming={isRunning}
			headerTrailing={
				showElapsed ? <ElapsedTime startedAt={startedAt} /> : undefined
			}
		>
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

/**
 * Live elapsed-time readout for a running command, anchored to the
 * server-side created_at of the tool call so it survives reloads and
 * reconnects. Falls back to mount time when the timestamp is missing or
 * unparseable. Kept as a leaf holding the formatted label so only this
 * span re-renders, and only when the displayed second changes.
 */
const ElapsedTime: FC<{ startedAt?: string }> = ({ startedAt }) => {
	const [mountedAt] = useState(() => Date.now());
	const parsedStart = startedAt ? new Date(startedAt).getTime() : Number.NaN;
	const startMs = Number.isFinite(parsedStart) ? parsedStart : mountedAt;
	const [label, setLabel] = useState(() =>
		formatElapsedMs(Date.now() - startMs),
	);

	useEffect(() => {
		const update = () => setLabel(formatElapsedMs(Date.now() - startMs));
		update();
		const interval = setInterval(update, 250);
		return () => clearInterval(interval);
	}, [startMs]);

	return (
		<span
			title="Elapsed time"
			className="shrink-0 font-mono text-xs tabular-nums leading-5 text-content-secondary"
		>
			{label}
		</span>
	);
};

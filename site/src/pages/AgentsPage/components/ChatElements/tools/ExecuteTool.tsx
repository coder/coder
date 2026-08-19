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
import { BackgroundProcessChip } from "./BackgroundProcessChip";
import {
	type AgentDisplayState,
	resolveAgentDisplayState,
} from "./displayMode";
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
	backgroundProcess?: {
		state: "running" | "exited";
		exitCode?: number;
		startedAtMs?: number;
	};
	killedBySignal?: "kill" | "terminate";
	modelIntent?: string;
	parsedCommands?: readonly string[][];
	shellToolDisplayMode?: TypesGen.AgentDisplayMode;
};

export const ExecuteTool: React.FC<ExecuteToolProps> = ({
	command,
	transcriptBlocks,
	status,
	isError,
	errorText,
	durationMs,
	isBackgrounded = false,
	backgroundProcess,
	killedBySignal,
	modelIntent,
	parsedCommands,
	shellToolDisplayMode,
}) => {
	const hasTranscriptBlocks = transcriptBlocks.length > 0;
	const autoDisplayState: AgentDisplayState =
		hasTranscriptBlocks ||
		status === "running" ||
		isBackgrounded ||
		!!killedBySignal
			? "preview"
			: "collapsed";
	const isRunning = status === "running";
	// A backgrounded call's duration is the spawn time (often ~0ms),
	// not the process lifetime. The chip carries the live state
	// instead, so the suffix is suppressed to avoid lying.
	const durationLabel = isBackgrounded ? "" : formatShellDurationMs(durationMs);
	const { commandLabel, durationSuffix } = getShellCommandLine({
		command,
		modelIntent,
		parsedCommands,
		durationLabel,
		isRunning,
		isError,
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
					{isBackgrounded && !isRunning && (
						<BackgroundProcessChip
							state={backgroundProcess?.state ?? "running"}
							exitCode={backgroundProcess?.exitCode}
							killedBySignal={killedBySignal}
							startedAtMs={backgroundProcess?.startedAtMs}
						/>
					)}
					{killedBySignal && !isRunning && !isBackgrounded && (
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
};

const getShellCommandLine = ({
	command,
	modelIntent,
	parsedCommands,
	durationLabel,
	isRunning,
	isError,
}: ShellCommandLineInput): { commandLabel: string; durationSuffix: string } => {
	const intentLabel = sanitizeExecuteModelIntent(modelIntent, command);
	const summary =
		parsedCommands && parsedCommands.length > 0
			? summarizeParsedCommands(parsedCommands)
			: "";
	const commandDisplay = summary || command;
	let commandLabel = intentLabel
		? `${intentLabel} using ${commandDisplay}`
		: `Ran ${commandDisplay}`;
	if (!isRunning && isError) {
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
}> = ({ command, transcriptBlocks, isError }) => {
	return (
		<ScrollArea
			className="col-start-1 col-span-2 mt-2 rounded-xl bg-surface-secondary/60 text-2xs"
			viewportClassName="max-h-64"
			scrollBarClassName="w-1.5"
		>
			<div className="px-3 py-2.5">
				<pre className="m-0 whitespace-pre-wrap break-all border-0 bg-transparent p-0 font-mono text-xs font-semibold leading-5 text-content-primary">
					<span aria-hidden className="select-none">
						$
					</span>{" "}
					{command}
				</pre>
				{transcriptBlocks.map((block) => (
					<pre
						key={block.kind}
						className={cn(
							"m-0 mt-4 whitespace-pre-wrap break-all border-0 bg-transparent p-0 font-mono text-xs font-normal leading-5",
							block.kind === "error" || isError
								? "text-content-destructive"
								: "text-content-secondary",
						)}
					>
						{block.text}
					</pre>
				))}
			</div>
		</ScrollArea>
	);
};

import { cn } from "cn";
import { OctagonXIcon } from "lucide-react";
import { type FC, useLayoutEffect, useRef } from "react";
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
import { ProcessIdentity } from "./ProcessIdentity";
import { TerminalOutput } from "./TerminalOutput";
import { ToolCall } from "./ToolCall";
import {
	asNumber,
	asRecord,
	sanitizeExecuteModelIntent,
	signalTooltipLabel,
	type ToolStatus,
} from "./utils";

type ProcessOutputToolProps = {
	output: string;
	command?: string;
	modelIntent?: string;
	status: ToolStatus;
	/**
	 * Whether the result snapshot saw the process still alive. This only
	 * affects label tense and signal badges; the row must not animate for
	 * it, because the snapshot never updates once the poll completes.
	 */
	processRunning?: boolean;
	exitCode: number | null;
	isError: boolean;
	errorMessage?: string;
	killedBySignal?: "kill" | "terminate";
	shellToolDisplayMode?: TypesGen.AgentDisplayMode;
	processId?: string;
	/** Truncation metadata from the result payload, when the buffer was cut. */
	truncation?: unknown;
	/** Output byte-identical to the previous loaded snapshot of this process. */
	noNewOutput?: boolean;
};

const getProcessOutputLabel = ({
	command,
	modelIntent,
	isChecking,
	isFailed,
}: {
	command: string | undefined;
	modelIntent: string | undefined;
	isChecking: boolean;
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
		return isChecking ? "Checking on background process" : "Process check";
	}
	if (isChecking) {
		return `Checking on ${trimmedCommand}`;
	}
	if (isFailed) {
		return `Failed to check on ${trimmedCommand}`;
	}
	return `Checked on ${trimmedCommand}`;
};

const statusSuffix = ({
	sawProcessRunning,
	exitCode,
	killedBySignal,
	noNewOutput,
}: {
	sawProcessRunning: boolean;
	exitCode: number | null;
	killedBySignal?: "kill" | "terminate";
	noNewOutput: boolean;
}): string => {
	// SIGKILL overrides a stale running snapshot, so the suffix steps aside
	// for the kill badge.
	if (killedBySignal === "kill") {
		return "";
	}
	if (sawProcessRunning) {
		return noNewOutput ? "· no new output" : "· still running";
	}
	if (exitCode === 0) {
		return "· finished";
	}
	return "";
};

const formatBytes = (value: number): string => {
	if (value < 1024) {
		return `${value} B`;
	}
	if (value < 1024 * 1024) {
		return `${Math.round(value / 1024)} KB`;
	}
	return `${(value / (1024 * 1024)).toFixed(1)} MB`;
};

const getTruncationLabel = (truncation: unknown): string | undefined => {
	const rec = asRecord(truncation);
	if (!rec) {
		return undefined;
	}
	const original = asNumber(rec.original_bytes, { parseString: true });
	const retained = asNumber(rec.retained_bytes, { parseString: true });
	if (original === undefined || retained === undefined) {
		return undefined;
	}
	return `Output truncated; showing ${formatBytes(retained)} of ${formatBytes(original)}`;
};

export const ProcessOutputTool: FC<ProcessOutputToolProps> = ({
	output,
	command,
	modelIntent,
	status,
	processRunning = false,
	exitCode,
	isError,
	errorMessage,
	killedBySignal,
	shellToolDisplayMode,
	processId,
	truncation,
	noNewOutput = false,
}) => {
	// Completed polls of a live process collapse: their intermediate output
	// is low value, and re-rendering the cumulative buffer at full weight is
	// what reads as a second command execution. Resolution rows (exit,
	// failure) preview.
	const autoDisplayState: AgentDisplayState =
		output.length > 0 && !(processRunning && status !== "running")
			? "preview"
			: "collapsed";
	const defaultView = resolveAgentDisplayState(
		shellToolDisplayMode,
		autoDisplayState,
	);

	const isChecking = status === "running";
	const sawProcessRunning = processRunning && killedBySignal !== "kill";
	// A clean exit is the expected outcome of a check, so only
	// failures earn a badge. The label verb carries the rest.
	const isFailed = exitCode !== null && exitCode !== 0;
	const hasOutput = output.length > 0;
	const hasHeaderActions =
		Boolean(processId) || Boolean(killedBySignal) || isFailed || hasOutput;
	const suffix = statusSuffix({
		sawProcessRunning,
		exitCode,
		killedBySignal,
		noNewOutput,
	});

	return (
		<ToolCall.Root
			key={`${shellToolDisplayMode ?? "auto"}:${autoDisplayState}`}
			className="group/proc w-full"
			status={status}
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
							isChecking,
							isFailed: isError || isFailed,
						})}
					</ToolCall.Label>
					{suffix && !isError && (
						<span className="shrink-0 text-[13px] text-content-secondary">
							{suffix}
						</span>
					)}
					<ToolCall.Status />
					<ToolCall.Chevron />
				</ToolCall.HeaderButton>
				{hasHeaderActions && (
					<ToolCall.HeaderActions>
						{processId && <ProcessIdentity processId={processId} />}
						{killedBySignal && !isChecking && !sawProcessRunning && (
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
				<ProcessOutputBody
					output={output}
					isError={isError}
					sawProcessRunning={sawProcessRunning}
					truncation={truncation}
					noNewOutput={noNewOutput}
				/>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};

const ProcessOutputBody: FC<{
	output: string;
	isError: boolean;
	sawProcessRunning: boolean;
	truncation: unknown;
	noNewOutput: boolean;
}> = ({ output, isError, sawProcessRunning, truncation, noNewOutput }) => {
	const scrollRef = useRef<HTMLDivElement>(null);
	const truncationLabel = getTruncationLabel(truncation);
	// A live process's newest output is the news; expanded polls land at the
	// bottom of the cumulative buffer so the freshest lines are visible.
	useLayoutEffect(() => {
		if (sawProcessRunning && scrollRef.current) {
			scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
		}
	}, [sawProcessRunning, output.length]);

	return (
		<TerminalOutput ariaLabel="Process output" className="mt-2">
			<div ref={scrollRef} className="space-y-2">
				<p className="m-0 border-0 bg-transparent p-0 font-sans text-2xs leading-5 text-content-secondary">
					{sawProcessRunning ? "Full output so far" : "Full output"}
				</p>
				{noNewOutput && (
					<p className="m-0 border-0 bg-transparent p-0 font-sans text-2xs leading-5 text-content-secondary">
						No new output since the last check
					</p>
				)}
				{truncationLabel && (
					<p className="m-0 border-0 bg-transparent p-0 font-sans text-2xs leading-5 text-content-secondary">
						{truncationLabel}
					</p>
				)}
				<pre
					className={cn(
						"m-0 border-0 whitespace-pre-wrap break-all bg-transparent p-0 font-mono text-xs leading-5",
						isError ? "text-content-destructive" : "text-content-secondary",
					)}
				>
					{output}
				</pre>
			</div>
		</TerminalOutput>
	);
};

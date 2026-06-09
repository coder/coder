import {
	CheckIcon,
	CircleAlertIcon,
	ExternalLinkIcon,
	LayersIcon,
	LoaderIcon,
	OctagonXIcon,
} from "lucide-react";
import type React from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
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
	durationMs?: number;
	isBackgrounded?: boolean;
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
	durationMs,
	isBackgrounded = false,
	killedBySignal,
	modelIntent,
	parsedCommands,
	shellToolDisplayMode,
}) => {
	const hasCommand = command.trim().length > 0;
	const hasTranscriptBlocks = transcriptBlocks.length > 0;
	const autoDisplayState: AgentDisplayState =
		hasTranscriptBlocks ||
		status === "running" ||
		isBackgrounded ||
		!!killedBySignal
			? "preview"
			: "collapsed";
	const isRunning = status === "running";
	const durationLabel = formatShellDurationMs(durationMs);
	const { commandLabel, durationSuffix } = getShellCommandLine({
		command,
		modelIntent,
		parsedCommands,
		durationLabel,
	});
	const defaultView = resolveAgentDisplayState(
		shellToolDisplayMode,
		autoDisplayState,
	);

	if (!hasCommand) {
		return null;
	}

	return (
		<ToolCall.Root
			key={`${shellToolDisplayMode ?? "auto"}:${autoDisplayState}`}
			className="group/exec grid w-full grid-cols-[minmax(0,1fr)_auto] items-start gap-x-2 rounded-md bg-surface-primary font-sans font-normal text-xs leading-5"
			status={status}
			isError={isError}
			errorMessage="Command failed"
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
						<Tooltip>
							<TooltipTrigger asChild>
								<span
									aria-label="Running in background"
									role="img"
									className="flex shrink-0 text-content-secondary"
								>
									<LayersIcon aria-hidden className="size-3.5 shrink-0" />
								</span>
							</TooltipTrigger>
							<TooltipContent>Running in background</TooltipContent>
						</Tooltip>
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
};

const getShellCommandLine = ({
	command,
	modelIntent,
	parsedCommands,
	durationLabel,
}: ShellCommandLineInput): { commandLabel: string; durationSuffix: string } => {
	const intentLabel = sanitizeExecuteModelIntent(modelIntent, command);
	const summary =
		parsedCommands && parsedCommands.length > 0
			? summarizeParsedCommands(parsedCommands)
			: "";
	const commandDisplay = summary || command;
	const commandLabel = intentLabel
		? `${intentLabel} using ${commandDisplay}`
		: `Ran ${commandDisplay}`;

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

export const ExecuteAuthRequiredTool: React.FC<{
	command: string;
	output: string;
	authenticateURL: string;
	providerLabel: string;
}> = ({ command, output, authenticateURL, providerLabel }) => {
	const hasCommand = command.trim().length > 0;
	const hasOutput = output.trim().length > 0;

	return (
		<div className="w-full overflow-hidden rounded-md border border-solid border-border-default bg-surface-primary">
			<div className="flex flex-wrap items-center gap-2 px-3 py-2">
				<CircleAlertIcon className="size-4 shrink-0 text-content-warning" />
				<span className="text-[13px] text-content-primary">
					Authenticate with {providerLabel} to continue this command.
				</span>
			</div>
			<div className="flex flex-wrap items-center gap-2 px-3 pb-2">
				<Button
					variant="outline"
					size="sm"
					onClick={() =>
						window.open(authenticateURL, "_blank", "width=900,height=600")
					}
					className="inline-flex cursor-pointer items-center gap-1 text-xs"
				>
					<ExternalLinkIcon className="size-3.5 shrink-0" />
					Authenticate with {providerLabel}
				</Button>
				<a
					href={authenticateURL}
					target="_blank"
					rel="noreferrer"
					className="inline-flex items-center gap-1 text-xs text-content-link no-underline hover:underline"
				>
					<ExternalLinkIcon className="size-3.5 shrink-0" />
					Open authentication link
				</a>
			</div>
			{hasCommand && (
				<div className="px-3 pb-1">
					<code className="font-mono text-xs text-content-secondary">
						$ {command}
					</code>
				</div>
			)}
			{hasOutput && (
				<ScrollArea
					className="rounded-b-md border-t border-solid border-border-default text-2xs"
					viewportClassName="max-h-48"
					scrollBarClassName="w-1.5"
				>
					<pre className="m-0 whitespace-pre-wrap break-all border-0 bg-transparent px-3 py-2 font-mono text-xs text-content-secondary">
						{output}
					</pre>
				</ScrollArea>
			)}
		</div>
	);
};

export const WaitForExternalAuthTool: React.FC<{
	providerLabel: string;
	status: ToolStatus;
	authenticated: boolean;
	timedOut: boolean;
	isError: boolean;
	errorMessage?: string;
}> = ({
	providerLabel,
	status,
	authenticated,
	timedOut,
	isError,
	errorMessage,
}) => {
	const isRunning = status === "running";
	let label = `Waiting for ${providerLabel} authentication...`;
	let statusIcon: React.ReactNode = isRunning ? (
		<LoaderIcon
			aria-label="Authentication in progress"
			role="img"
			className="size-3.5 shrink-0 animate-spin text-content-link motion-reduce:animate-none"
		/>
	) : null;
	if (isError) {
		label =
			errorMessage ||
			`Failed while waiting for ${providerLabel} authentication`;
		statusIcon = (
			<OctagonXIcon
				aria-label="Authentication failed"
				role="img"
				className="size-3.5 shrink-0 text-content-destructive"
			/>
		);
	} else if (timedOut) {
		label = `Timed out waiting for ${providerLabel} authentication`;
		statusIcon = (
			<CircleAlertIcon
				aria-label="Authentication timed out"
				role="img"
				className="size-3.5 shrink-0 text-content-warning"
			/>
		);
	} else if (authenticated && !isRunning) {
		label = `Authenticated with ${providerLabel}`;
		statusIcon = (
			<CheckIcon
				aria-label="Authentication completed"
				role="img"
				className="size-3.5 shrink-0 text-content-success"
			/>
		);
	}

	return (
		<ToolCall.Root
			className="w-full overflow-hidden rounded-md border border-solid border-border-default bg-surface-primary px-3 py-2"
			status={status}
			isError={isError}
			errorMessage={
				errorMessage ||
				`Failed while waiting for ${providerLabel} authentication`
			}
			hasContent={false}
		>
			<ToolCall.HeaderLayout>
				<ToolCall.HeaderButton className="min-w-0 flex-1 font-normal text-content-secondary">
					<ToolCall.LeadingIcon>{statusIcon}</ToolCall.LeadingIcon>
					<ToolCall.Label className="text-content-primary">
						{label}
					</ToolCall.Label>
				</ToolCall.HeaderButton>
			</ToolCall.HeaderLayout>
		</ToolCall.Root>
	);
};

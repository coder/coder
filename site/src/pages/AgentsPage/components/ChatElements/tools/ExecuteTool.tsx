import {
	CheckIcon,
	ChevronDownIcon,
	CircleAlertIcon,
	ExternalLinkIcon,
	LayersIcon,
	LoaderIcon,
	OctagonXIcon,
	TriangleAlertIcon,
} from "lucide-react";
import type React from "react";
import { useState } from "react";
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
import { signalTooltipLabel, type ToolStatus } from "./utils";

type ExecuteToolProps = {
	command: string;
	output: string;
	status: ToolStatus;
	isError: boolean;
	exitCode?: number | null;
	durationMs?: number;
	isBackgrounded?: boolean;
	killedBySignal?: "kill" | "terminate";
	shellToolDisplayMode?: TypesGen.AgentDisplayMode;
};

type ExecuteToolInnerProps = ExecuteToolProps & {
	outputInitiallyExpanded: boolean;
};

export const ExecuteTool: React.FC<ExecuteToolProps> = (props) => {
	const autoDisplayState: AgentDisplayState =
		props.output.length > 0 ||
		props.status === "running" ||
		props.isBackgrounded ||
		!!props.killedBySignal
			? "preview"
			: "collapsed";
	const resolvedDisplayState = resolveAgentDisplayState(
		props.shellToolDisplayMode,
		autoDisplayState,
	);
	return (
		<ExecuteToolInner
			key={`${props.shellToolDisplayMode ?? "auto"}:${autoDisplayState}`}
			{...props}
			outputInitiallyExpanded={resolvedDisplayState !== "collapsed"}
		/>
	);
};

const ExecuteToolInner: React.FC<ExecuteToolInnerProps> = ({
	command,
	output,
	status,
	isError,
	durationMs,
	isBackgrounded = false,
	killedBySignal,
	outputInitiallyExpanded,
}) => {
	const hasOutput = output.length > 0;
	const isRunning = status === "running";
	const [outputExpanded, setOutputExpanded] = useState(outputInitiallyExpanded);
	const outputToggleLabel = outputExpanded
		? "Collapse command output"
		: "Expand command output";
	const durationLabel = formatShellDurationMs(durationMs);

	return (
		<div className="group/exec grid w-full grid-cols-[auto_minmax(0,1fr)_auto] items-start gap-x-2 rounded-md bg-surface-primary font-mono text-xs leading-5 sm:grid-cols-[auto_minmax(0,1fr)_auto_auto]">
			<span className="col-start-1 row-start-1 shrink-0 text-content-success">
				$
			</span>
			<div className="col-start-2 row-start-1 min-w-0">
				<Tooltip>
					<TooltipTrigger asChild>
						<span className="block truncate text-content-primary">
							{command}
						</span>
					</TooltipTrigger>
					<TooltipContent className="max-w-xl whitespace-pre-wrap break-all font-mono">
						{command}
					</TooltipContent>
				</Tooltip>
			</div>
			<div className="col-start-3 row-start-1 flex shrink-0 items-center gap-1">
				{isRunning && (
					<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
				)}
				{isBackgrounded && !isRunning && (
					<Tooltip>
						<TooltipTrigger asChild>
							<LayersIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
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
					className="-my-0.5 size-6 p-0 opacity-0 transition-opacity hover:bg-surface-tertiary group-hover/exec:opacity-100"
				/>
			</div>
			{hasOutput && (
				<ShellOutputToggle
					expanded={outputExpanded}
					label={outputToggleLabel}
					durationLabel={durationLabel}
					onToggle={() => setOutputExpanded((value) => !value)}
				/>
			)}
			{hasOutput && outputExpanded && (
				<ShellOutputBody output={output} isError={isError} />
			)}
		</div>
	);
};

const ShellOutputToggle: React.FC<{
	expanded: boolean;
	label: string;
	durationLabel: string;
	onToggle: () => void;
}> = ({ expanded, label, durationLabel, onToggle }) => {
	return (
		<button
			type="button"
			aria-expanded={expanded}
			aria-label={label}
			onClick={onToggle}
			className="col-start-2 col-span-2 row-start-2 mt-1 flex min-w-0 cursor-pointer items-center gap-2 border-0 bg-transparent p-0 font-mono text-xs leading-5 text-content-secondary hover:text-content-primary sm:col-start-4 sm:col-span-1 sm:row-start-1 sm:mt-0"
		>
			<span className="sm:hidden">output</span>
			{durationLabel && <span>{durationLabel}</span>}
			<ChevronDownIcon
				className={cn(
					"size-3.5 shrink-0 transition-transform",
					expanded ? "rotate-180" : "",
				)}
			/>
		</button>
	);
};

const ShellOutputBody: React.FC<{
	output: string;
	isError: boolean;
}> = ({ output, isError }) => {
	return (
		<ScrollArea
			className="col-start-1 col-span-3 mt-1 rounded-md border border-solid border-border-default/50 bg-surface-secondary/30 text-2xs sm:col-span-4"
			viewportClassName="max-h-64"
			scrollBarClassName="w-1.5"
		>
			<pre
				className={cn(
					"m-0 whitespace-pre-wrap break-all border-0 bg-transparent px-2 py-1.5 font-mono text-xs leading-5",
					isError ? "text-content-destructive" : "text-content-primary",
				)}
			>
				{output}
			</pre>
		</ScrollArea>
	);
};

const formatShellDurationMs = (durationMs: number | undefined): string => {
	if (durationMs === undefined || durationMs < 0) {
		return "";
	}
	if (durationMs < 1000) {
		return `${Math.round(durationMs)}ms`;
	}
	const seconds = durationMs / 1000;
	if (seconds < 60) {
		return `${Number(seconds.toFixed(1))}s`;
	}
	const minutes = seconds / 60;
	if (minutes < 60) {
		return `${Number(minutes.toFixed(1))}m`;
	}
	const hours = minutes / 60;
	return `${Number(hours.toFixed(1))}h`;
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
				<CircleAlertIcon className="h-4 w-4 shrink-0 text-content-warning" />
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
					<ExternalLinkIcon className="h-3.5 w-3.5 shrink-0" />
					Authenticate with {providerLabel}
				</Button>
				<a
					href={authenticateURL}
					target="_blank"
					rel="noreferrer"
					className="inline-flex items-center gap-1 text-xs text-content-link no-underline hover:underline"
				>
					<ExternalLinkIcon className="h-3.5 w-3.5 shrink-0" />
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
	let icon: React.ReactNode = (
		<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-link" />
	);
	if (isError) {
		label =
			errorMessage ||
			`Failed while waiting for ${providerLabel} authentication`;
		icon = (
			<TriangleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
		);
	} else if (timedOut) {
		label = `Timed out waiting for ${providerLabel} authentication`;
		icon = (
			<CircleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-warning" />
		);
	} else if (authenticated && !isRunning) {
		label = `Authenticated with ${providerLabel}`;
		icon = <CheckIcon className="h-3.5 w-3.5 shrink-0 text-content-success" />;
	}

	return (
		<div className="w-full overflow-hidden rounded-md border border-solid border-border-default bg-surface-primary px-3 py-2">
			<div className="flex items-center gap-2">
				{icon}
				<span className="text-[13px] text-content-primary">{label}</span>
			</div>
		</div>
	);
};

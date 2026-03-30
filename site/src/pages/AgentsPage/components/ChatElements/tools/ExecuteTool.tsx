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
import { useRef, useState } from "react";
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
	BORDER_BG_STYLE,
	COLLAPSED_OUTPUT_HEIGHT,
	signalTooltipLabel,
	type ToolStatus,
} from "./utils";

/**
 * Specialized rendering for `execute` tool calls. Shows the command
 * in a terminal-style block with a copy button. Output is shown in a
 * collapsed preview (~3 lines) with an expand chevron at the bottom.
 */
export const ExecuteTool: React.FC<{
	command: string;
	output: string;
	status: ToolStatus;
	isError: boolean;
	isBackgrounded?: boolean;
	killedBySignal?: "kill" | "terminate";
}> = ({ command, output, status, isBackgrounded = false, killedBySignal }) => {
	const [expanded, setExpanded] = useState(false);
	const outputRef = useRef<HTMLPreElement | null>(null);
	const hasOutput = output.length > 0;
	const isRunning = status === "running";

	// Track whether the command text is truncated so we can offer
	// a click-to-expand interaction. The ResizeObserver may clear
	// commandOverflows while the text is wrapped, but
	// canToggleCommand stays true via commandExpanded so the
	// collapse affordance remains visible.
	const [commandExpanded, setCommandExpanded] = useState(false);
	const [commandOverflows, setCommandOverflows] = useState(false);
	const canToggleCommand = commandOverflows || commandExpanded;
	const commandRef = (node: HTMLElement | null) => {
		if (!node) return;
		const measure = () => {
			setCommandOverflows(node.scrollWidth > node.clientWidth);
		};
		measure();
		const ro = new ResizeObserver(measure);
		ro.observe(node);
		return () => ro.disconnect();
	};

	// Check whether the output overflows the collapsed height so we
	// know if we need to show the expand toggle at all.
	const [overflows, setOverflows] = useState(false);
	const measureRef = (node: HTMLPreElement | null) => {
		outputRef.current = node;
		if (node) {
			setOverflows(node.scrollHeight > COLLAPSED_OUTPUT_HEIGHT);
		}
	};

	return (
		<div className="group/exec w-full overflow-hidden rounded-md border border-solid border-border-default bg-surface-primary">
			{/* Header: $ command + copy button */}
			<div className="flex w-full items-start justify-between gap-2 px-3 py-2">
				{/* biome-ignore lint/a11y/useKeyWithClickEvents: Click toggles for mouse users; keyboard users use the chevron button. */}
				<div
					className={cn(
						"flex min-w-0 flex-1 items-start gap-2",
						canToggleCommand && "cursor-pointer",
					)}
					onClick={
						canToggleCommand ? () => setCommandExpanded((v) => !v) : undefined
					}
				>
					<span className="shrink-0 font-mono text-xs leading-5 text-content-secondary">
						$
					</span>
					<code
						ref={commandRef}
						className={cn(
							"min-w-0 flex-1 font-mono text-xs leading-5 text-content-primary",
							commandExpanded ? "whitespace-pre-wrap break-all" : "truncate",
						)}
					>
						{command}
					</code>
				</div>
				<div className="flex shrink-0 items-center gap-1">
					{canToggleCommand && (
						<button
							type="button"
							onClick={() => setCommandExpanded((v) => !v)}
							className={cn(
								"border-0 bg-transparent p-0 m-0 cursor-pointer flex items-center text-content-secondary hover:text-content-primary transition-colors transition-opacity focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link",
								commandExpanded
									? "opacity-100"
									: "opacity-0 group-hover/exec:opacity-100",
							)}
							aria-expanded={commandExpanded}
							aria-label={
								commandExpanded ? "Collapse command" : "Expand command"
							}
						>
							<ChevronDownIcon
								className={cn(
									"h-3.5 w-3.5 transition-transform",
									commandExpanded && "rotate-180",
								)}
							/>
						</button>
					)}
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
			</div>
			{/* Output preview / expanded */}
			{hasOutput && (
				<>
					<div className="h-px" style={BORDER_BG_STYLE} />
					<ScrollArea
						className="text-2xs"
						viewportClassName={expanded ? "max-h-96" : ""}
						scrollBarClassName="w-1.5"
					>
						<pre
							ref={measureRef}
							style={
								expanded
									? undefined
									: { maxHeight: COLLAPSED_OUTPUT_HEIGHT, overflow: "hidden" }
							}
							className={cn(
								"m-0 border-0 whitespace-pre-wrap break-all bg-transparent px-3 py-2 font-mono text-xs",
								"text-content-secondary",
							)}
						>
							{output}
						</pre>
					</ScrollArea>

					{/* Expand / collapse toggle at the bottom */}
					{overflows && (
						<button
							type="button"
							aria-expanded={expanded}
							onClick={() => setExpanded((v) => !v)}
							className="border-0 bg-transparent m-0 font-[inherit] text-[inherit] flex w-full cursor-pointer items-center justify-center py-0.5 text-content-secondary transition-colors hover:bg-surface-secondary hover:text-content-primary"
							aria-label={expanded ? "Collapse output" : "Expand output"}
						>
							<ChevronDownIcon
								className={cn(
									"h-3 w-3 transition-transform",
									expanded && "rotate-180",
								)}
							/>
						</button>
					)}
				</>
			)}
		</div>
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
				<CircleAlertIcon className="h-4 w-4 shrink-0 text-content-warning" />
				<span className="text-sm text-content-primary">
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
				<span className="text-sm text-content-primary">{label}</span>
			</div>
		</div>
	);
};

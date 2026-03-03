import { Button } from "components/Button/Button";
import { CopyButton } from "components/CopyButton/CopyButton";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import {
	CheckIcon,
	ChevronDownIcon,
	CircleAlertIcon,
	ExternalLinkIcon,
	LoaderIcon,
} from "lucide-react";
import type React from "react";
import { useRef, useState } from "react";
import { cn } from "utils/cn";
import {
	BORDER_BG_STYLE,
	COLLAPSED_OUTPUT_HEIGHT,
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
}> = ({ command, output, status, isError }) => {
	const [expanded, setExpanded] = useState(false);
	const outputRef = useRef<HTMLPreElement | null>(null);
	const hasOutput = output.length > 0;
	const isRunning = status === "running";

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
			<div className="flex w-full items-center justify-between gap-2 px-2.5 py-0.5">
				<div className="flex min-w-0 flex-1 items-center gap-2">
					<span className="shrink-0 font-mono text-xs text-content-secondary">
						$
					</span>
					<code className="min-w-0 flex-1 truncate font-mono text-xs text-content-primary">
						{command}
					</code>
				</div>
				<div className="flex shrink-0 items-center gap-1">
					{isRunning && (
						<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
					)}
					<span className="opacity-0 transition-opacity group-hover/exec:opacity-100">
						<CopyButton text={command} label="Copy command" />
					</span>
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
								"m-0 border-0 whitespace-pre-wrap break-all bg-transparent px-2.5 py-2 font-mono text-xs",
								isError ? "text-content-destructive" : "text-content-secondary",
							)}
						>
							{output}
						</pre>
					</ScrollArea>

					{/* Expand / collapse toggle at the bottom */}
					{overflows && (
						<div
							role="button"
							tabIndex={0}
							aria-expanded={expanded}
							onClick={() => setExpanded((v) => !v)}
							onKeyDown={(e) => {
								if (e.key === "Enter" || e.key === " ") {
									setExpanded((v) => !v);
								}
							}}
							className="flex w-full cursor-pointer items-center justify-center py-0.5 text-content-secondary transition-colors hover:bg-surface-secondary hover:text-content-primary"
							aria-label={expanded ? "Collapse output" : "Expand output"}
						>
							<ChevronDownIcon
								className={cn(
									"h-3 w-3 transition-transform",
									expanded && "rotate-180",
								)}
							/>
						</div>
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
			<CircleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-destructive" />
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

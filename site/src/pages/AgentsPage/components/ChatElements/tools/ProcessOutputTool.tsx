import { ChevronDownIcon, LoaderIcon } from "lucide-react";
import type React from "react";
import { useRef, useState } from "react";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { cn } from "#/utils/cn";
import { COLLAPSED_OUTPUT_HEIGHT } from "./utils";

/**
 * Specialized rendering for `process_output` tool calls. Shows
 * process output directly in a terminal-style block with a
 * collapsible preview and an expand chevron at the bottom.
 */
export const ProcessOutputTool: React.FC<{
	output: string;
	isRunning: boolean;
	exitCode: number | null;
	isError: boolean;
}> = ({ output, isRunning, exitCode, isError }) => {
	const [expanded, setExpanded] = useState(false);
	const outputRef = useRef<HTMLPreElement | null>(null);
	const hasOutput = output.length > 0;

	const [overflows, setOverflows] = useState(false);
	const measureRef = (node: HTMLPreElement | null) => {
		outputRef.current = node;
		if (node) {
			setOverflows(node.scrollHeight > COLLAPSED_OUTPUT_HEIGHT);
		}
	};

	const showExitCode = exitCode !== null && exitCode !== 0;

	return (
		<div className="group/proc w-full overflow-hidden rounded-md border border-solid border-border-default bg-surface-primary">
			{hasOutput ? (
				<>
					<div className="relative">
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
									isError
										? "text-content-destructive"
										: "text-content-secondary",
								)}
							>
								{output}
							</pre>
						</ScrollArea>
						<div className="absolute right-1 top-0.5 flex items-center gap-1 opacity-0 transition-opacity group-hover/proc:opacity-100">
							{isRunning && (
								<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
							)}
							{showExitCode && (
								<span className="rounded px-1.5 py-0.5 font-mono text-2xs leading-none bg-surface-red text-content-destructive">
									exit {exitCode}
								</span>
							)}
							<CopyButton text={output} label="Copy output" />
						</div>
					</div>

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
			) : (
				<div className="flex items-center gap-1 px-3 py-1.5">
					{isRunning && (
						<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
					)}
					{showExitCode && (
						<span className="rounded px-1.5 py-0.5 font-mono text-2xs leading-none bg-surface-red text-content-destructive">
							exit {exitCode}
						</span>
					)}
				</div>
			)}
		</div>
	);
};

import { cn } from "cn";
import type React from "react";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";

type TerminalOutputProps = {
	/** Accessible name for the scrollable output region. */
	ariaLabel: string;
	/**
	 * Shell command the output belongs to. Pinned to the top of the pane so
	 * it stays visible while the output scrolls.
	 */
	command?: string;
	/** Outer placement classes, e.g. "col-start-1 col-span-2 mt-2". */
	className?: string;
	/** Renders the streaming cursor; pass the tool call status, not a process liveness snapshot. */
	streaming?: boolean;
	children: React.ReactNode;
};

/**
 * Shared terminal-style output pane for execute and process output tool
 * cards. Keeps the shared ScrollArea contract: a max-h-64 viewport cap,
 * a focusable region with an accessible name, and the narrow scrollbar.
 */
export const TerminalOutput: React.FC<TerminalOutputProps> = ({
	ariaLabel,
	command,
	className,
	streaming = false,
	children,
}) => (
	<ScrollArea
		className={cn(
			"rounded-md border border-solid border-border bg-surface-secondary/60 text-2xs",
			className,
		)}
		viewportClassName="max-h-64"
		viewportTabIndex={0}
		viewportAriaLabel={ariaLabel}
		scrollBarClassName="w-1.5"
	>
		{command?.trim() && (
			<div className="sticky top-0 z-10 flex items-start gap-1.5 border-0 border-b border-solid border-border bg-surface-secondary px-3 py-1.5">
				<span
					aria-hidden
					className="select-none shrink-0 font-mono text-xs font-normal leading-5 text-content-secondary"
				>
					$
				</span>
				<pre className="m-0 min-w-0 whitespace-pre-wrap break-all border-0 bg-transparent p-0 font-mono text-xs font-semibold leading-5 text-content-primary">
					{command}
				</pre>
			</div>
		)}
		<div className="space-y-2 px-3 py-2.5 font-mono text-xs leading-5">
			{children}
			{streaming && <TerminalCursor />}
		</div>
	</ScrollArea>
);

/** Streaming block cursor shown while a command is still producing output. */
export const TerminalCursor: React.FC = () => (
	<span
		aria-hidden
		data-testid="terminal-streaming-cursor"
		className="ml-0.5 inline-block h-3.5 w-1.5 translate-y-0.5 animate-pulse bg-content-secondary align-middle motion-reduce:animate-none"
	/>
);

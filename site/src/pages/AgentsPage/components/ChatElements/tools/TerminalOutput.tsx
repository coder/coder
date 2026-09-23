import { cn } from "cn";
import type React from "react";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";

type TerminalOutputProps = {
	ariaLabel: string;
	command?: string;
	className?: string;
	streaming?: boolean;
	/** Rendered at the trailing edge of the command header row. */
	headerTrailing?: React.ReactNode;
	children: React.ReactNode;
};

export const TerminalOutput: React.FC<TerminalOutputProps> = ({
	ariaLabel,
	command,
	className,
	streaming = false,
	headerTrailing,
	children,
}) => (
	<div
		className={cn(
			"overflow-hidden rounded-md border border-solid border-border bg-surface-primary",
			className,
		)}
	>
		{command?.trim() && (
			<div className="flex items-start gap-1.5 border-0 border-b border-solid border-border bg-surface-secondary px-3 py-1.5">
				<span
					aria-hidden
					className="select-none shrink-0 font-mono text-xs leading-5 text-content-secondary"
				>
					$
				</span>
				<pre className="m-0 min-w-0 flex-1 whitespace-pre-wrap break-all border-0 bg-transparent p-0 font-mono text-xs font-semibold leading-5 text-content-primary">
					{command}
				</pre>
				{headerTrailing}
			</div>
		)}
		<ScrollArea
			viewportClassName="max-h-64"
			viewportTabIndex={0}
			viewportAriaLabel={ariaLabel}
			scrollBarClassName="w-1.5"
		>
			<div className="space-y-2 px-3 py-2.5 font-mono text-xs leading-5">
				{children}
				{streaming && (
					<span
						aria-hidden
						className="ml-0.5 inline-block h-3.5 w-1.5 translate-y-0.5 animate-pulse bg-content-secondary align-middle motion-reduce:animate-none"
					/>
				)}
			</div>
		</ScrollArea>
	</div>
);

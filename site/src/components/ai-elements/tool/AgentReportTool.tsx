import {
	CheckIcon,
	ChevronDownIcon,
	CircleAlertIcon,
	LoaderIcon,
} from "lucide-react";
import type React from "react";
import { useRef, useState } from "react";
import { cn } from "utils/cn";
import { Response } from "../response";
import {
	BORDER_BG_STYLE,
	COLLAPSED_REPORT_HEIGHT,
	type ToolStatus,
} from "./utils";

/**
 * Specialized rendering for `subagent_report` tool calls. Shows the
 * report as collapsible markdown with a short preview that users
 * can expand.
 */
export const AgentReportTool: React.FC<{
	title: string;
	report: string;
	toolStatus: ToolStatus;
	isError: boolean;
}> = ({ title, report, toolStatus, isError }) => {
	const [expanded, setExpanded] = useState(false);
	const contentRef = useRef<HTMLDivElement>(null);
	const [overflows, setOverflows] = useState(false);
	const measureRef = (node: HTMLDivElement | null) => {
		(contentRef as React.MutableRefObject<HTMLDivElement | null>).current =
			node;
		if (node) {
			setOverflows(node.scrollHeight > COLLAPSED_REPORT_HEIGHT);
		}
	};
	const isRunning = toolStatus === "running";

	return (
		<div className="w-full overflow-hidden rounded-lg border border-solid border-border-default bg-surface-primary">
			<div
				role="button"
				tabIndex={0}
				onClick={() => overflows && setExpanded((v) => !v)}
				onKeyDown={(e) => {
					if ((e.key === "Enter" || e.key === " ") && overflows) {
						setExpanded((v) => !v);
					}
				}}
				className={cn(
					"flex items-center gap-2 px-3 py-2",
					overflows && "cursor-pointer",
				)}
			>
				{isRunning ? (
					<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin text-content-link" />
				) : isError ? (
					<CircleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-destructive" />
				) : (
					<CheckIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
				)}
				<span
					className={cn(
						"min-w-0 flex-1 truncate text-sm",
						isError ? "text-content-destructive" : "text-content-secondary",
					)}
				>
					{title}
				</span>
				{overflows && (
					<ChevronDownIcon
						className={cn(
							"h-3 w-3 shrink-0 text-content-secondary transition-transform",
							expanded ? "rotate-0" : "-rotate-90",
						)}
					/>
				)}
			</div>
			{report.trim() && (
				<>
					<div className="h-px" style={BORDER_BG_STYLE} />
					<div className="px-3 py-2">
						<div
							ref={measureRef}
							style={
								expanded
									? undefined
									: {
											maxHeight: COLLAPSED_REPORT_HEIGHT,
											overflow: "hidden",
										}
							}
						>
							<Response>{report}</Response>
						</div>
					</div>
				</>
			)}
		</div>
	);
};

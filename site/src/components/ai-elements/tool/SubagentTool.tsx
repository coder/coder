import {
	BotIcon,
	ChevronDownIcon,
	CircleXIcon,
	ClockIcon,
	ExternalLinkIcon,
	LoaderIcon,
} from "lucide-react";
import type React from "react";
import { useState } from "react";
import { Link } from "react-router";
import { cn } from "utils/cn";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Response } from "../response";
import {
	isSubagentSuccessStatus,
	shortDurationMs,
	type ToolStatus,
} from "./utils";

const SUBAGENT_VERBS: Record<
	string,
	{ completed: string; running: string; error: string; timeout: string }
> = {
	spawn_agent: {
		completed: "Spawned ",
		running: "Spawning ",
		error: "Failed to spawn ",
		timeout: "Timed out spawning ",
	},
	wait_agent: {
		completed: "Waited for ",
		running: "Waiting for ",
		error: "Failed waiting for ",
		timeout: "Timed out waiting for ",
	},
	message_agent: {
		completed: "Messaged ",
		running: "Messaging ",
		error: "Failed to message ",
		timeout: "Timed out messaging ",
	},
	close_agent: {
		completed: "Terminated ",
		running: "Terminating ",
		error: "Failed to terminate ",
		timeout: "Timed out terminating ",
	},
};

/**
 * Resolves a sub-agent status string and tool-level status into a
 * display icon. The sub-agent status in the tool result is a
 * snapshot from when the tool returned and may be stale (e.g. a
 * background sub-agent records "pending" forever). The icon is
 * therefore driven primarily by the tool-call status itself.
 */
const SubagentStatusIcon: React.FC<{
	subagentStatus: string;
	toolStatus: ToolStatus;
	isError: boolean;
	isTimeout: boolean;
}> = ({ subagentStatus, toolStatus, isError, isTimeout }) => {
	const subagentCompleted = isSubagentSuccessStatus(subagentStatus);
	if (isTimeout && !subagentCompleted) {
		return <ClockIcon className="h-4 w-4 shrink-0 text-content-secondary" />;
	}
	if ((isError && !subagentCompleted) || toolStatus === "error") {
		return <CircleXIcon className="h-4 w-4 shrink-0 text-content-secondary" />;
	}
	if (toolStatus === "running") {
		return (
			<LoaderIcon className="h-4 w-4 shrink-0 animate-spin motion-reduce:animate-none text-content-link" />
		);
	}
	return <BotIcon className="h-4 w-4 shrink-0 text-content-secondary" />;
};

/**
 * Specialized rendering for delegated sub-agent tool calls.
 * Shows a clickable header row with the sub-agent title, status
 * icon, and a chevron to expand the prompt / report below. A
 * "View Agent" link navigates to the sub-agent chat.
 */
export const SubagentTool: React.FC<{
	toolName: string;
	title: string;
	chatId: string;
	subagentStatus: string;
	prompt?: string;
	message?: string;
	durationMs?: number;
	report?: string;
	toolStatus: ToolStatus;
	isError: boolean;
	isTimeout?: boolean;
}> = ({
	toolName,
	title,
	chatId,
	subagentStatus,
	prompt,
	message,
	durationMs,
	report,
	toolStatus,
	isError,
	isTimeout = false,
}) => {
	const [expanded, setExpanded] = useState(false);
	const hasPrompt = Boolean(prompt?.trim());
	const hasMessage = Boolean(message?.trim());
	const hasReport = Boolean(report?.trim());
	const hasExpandableContent = hasPrompt || hasMessage || hasReport;
	const durationLabel = shortDurationMs(durationMs);

	return (
		<div className="w-full">
			<button
				type="button"
				aria-expanded={expanded}
				onClick={() => hasExpandableContent && setExpanded((v) => !v)}
				className={cn(
					"border-0 bg-transparent p-0 m-0 font-[inherit] text-[inherit] text-left",
					"flex w-full items-center gap-2",
					hasExpandableContent && "cursor-pointer",
				)}
			>
				<SubagentStatusIcon
					subagentStatus={subagentStatus}
					toolStatus={toolStatus}
					isError={isError}
					isTimeout={isTimeout}
				/>
				<span className="min-w-0 flex-1 truncate text-sm text-content-secondary">
					{SUBAGENT_VERBS[toolName]?.[
						isTimeout
							? "timeout"
							: toolStatus === "completed"
								? "completed"
								: toolStatus === "error"
									? "error"
									: "running"
					] ?? ""}
					<span className="text-content-secondary opacity-60">{title}</span>
					{chatId && (
						<Link
							to={`/agents/${chatId}`}
							onClick={(e) => e.stopPropagation()}
							className="ml-1 inline-flex align-middle text-content-secondary opacity-50 transition-opacity hover:opacity-100"
							aria-label="View agent"
						>
							<ExternalLinkIcon className="h-3 w-3" />
						</Link>
					)}
				</span>
				{durationLabel && (
					<span className="shrink-0 text-xs text-content-secondary">
						Worked for {durationLabel}
					</span>
				)}
				{hasExpandableContent && (
					<ChevronDownIcon
						className={cn(
							"h-3 w-3 shrink-0 text-content-secondary transition-transform",
							expanded ? "rotate-0" : "-rotate-90",
						)}
					/>
				)}
			</button>

			{expanded && hasPrompt && (
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default"
					viewportClassName="max-h-64"
					scrollBarClassName="w-1.5"
				>
					<div className="px-3 py-2">
						<Response>{prompt ?? ""}</Response>
					</div>
				</ScrollArea>
			)}

			{expanded && hasMessage && (
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default"
					viewportClassName="max-h-64"
					scrollBarClassName="w-1.5"
				>
					<div className="px-3 py-2">
						<Response>{message ?? ""}</Response>
					</div>
				</ScrollArea>
			)}

			{expanded && hasReport && (
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default"
					viewportClassName="max-h-64"
					scrollBarClassName="w-1.5"
				>
					<div className="px-3 py-2">
						<Response>{report ?? ""}</Response>
					</div>
				</ScrollArea>
			)}
		</div>
	);
};

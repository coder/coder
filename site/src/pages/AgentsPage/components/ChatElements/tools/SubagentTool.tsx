import {
	BotIcon,
	ChevronDownIcon,
	CircleXIcon,
	ClockIcon,
	ExternalLinkIcon,
	LoaderIcon,
	MonitorIcon,
} from "lucide-react";
import type React from "react";
import { useState } from "react";
import { Link } from "react-router";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { cn } from "#/utils/cn";
import { Response } from "../Response";
import { Shimmer } from "../Shimmer";
import { useDesktopPanel } from "./DesktopPanelContext";
import { InlineDesktopPreview } from "./InlineDesktopPreview";
import { RecordingPreview } from "./RecordingPreview";
import type { SubagentAction, SubagentDescriptor } from "./subagentDescriptor";
import {
	isSubagentSuccessStatus,
	shortDurationMs,
	type ToolStatus,
} from "./utils";

const SUBAGENT_VERBS: Record<
	SubagentAction,
	{ completed: string; running: string; error: string; timeout: string }
> = {
	spawn: {
		completed: "Spawned ",
		running: "Spawning ",
		error: "Failed to spawn ",
		timeout: "Timed out spawning ",
	},
	wait: {
		completed: "Waited for ",
		running: "Waiting for ",
		error: "Failed waiting for ",
		timeout: "Timed out waiting for ",
	},
	message: {
		completed: "Messaged ",
		running: "Messaging ",
		error: "Failed to message ",
		timeout: "Timed out messaging ",
	},
	close: {
		completed: "Terminated ",
		running: "Terminating ",
		error: "Failed to terminate ",
		timeout: "Timed out terminating ",
	},
};

/**
 * Returns the label JSX for a sub-agent tool row. Extracted to keep
 * the rendering logic for the three label variants readable.
 */
function getSubagentLabel(
	showDesktopPreview: boolean | undefined,
	toolStatus: ToolStatus,
	descriptor: SubagentDescriptor,
	title: string,
	isTimeout: boolean,
): React.ReactNode {
	if (showDesktopPreview && toolStatus === "running") {
		return (
			<Shimmer as="span" className="text-[13px]">
				Using the computer...
			</Shimmer>
		);
	}
	if (
		descriptor.variant === "computer_use" &&
		descriptor.action === "wait" &&
		toolStatus === "completed"
	) {
		return (
			<>
				Used the computer <span className="opacity-60">{title}</span>
			</>
		);
	}
	const phase = isTimeout
		? "timeout"
		: toolStatus === "completed"
			? "completed"
			: toolStatus === "error"
				? "error"
				: "running";
	return (
		<>
			{SUBAGENT_VERBS[descriptor.action][phase]}
			<span className="opacity-60">{title}</span>
		</>
	);
}

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
	iconKind?: SubagentDescriptor["iconKind"];
	showDesktopPreview?: boolean;
}> = ({
	subagentStatus,
	toolStatus,
	isError,
	isTimeout,
	iconKind = "bot",
	showDesktopPreview = false,
}) => {
	const subagentCompleted = isSubagentSuccessStatus(subagentStatus);
	const DefaultIcon = iconKind === "monitor" ? MonitorIcon : BotIcon;
	if (isTimeout && !subagentCompleted) {
		return <ClockIcon className="h-4 w-4 shrink-0 text-current" />;
	}
	if ((isError && !subagentCompleted) || toolStatus === "error") {
		return <CircleXIcon className="h-4 w-4 shrink-0 text-current" />;
	}
	if (toolStatus === "running") {
		if (showDesktopPreview) {
			return <MonitorIcon className="h-4 w-4 shrink-0 text-current" />;
		}
		return (
			<LoaderIcon className="h-4 w-4 shrink-0 animate-spin motion-reduce:animate-none text-content-link" />
		);
	}
	return <DefaultIcon className="h-4 w-4 shrink-0 text-current" />;
};

/**
 * Specialized rendering for delegated sub-agent tool calls.
 * Shows a clickable header row with the sub-agent title, status
 * icon, and a chevron to expand the prompt / report below. A
 * "View Agent" link navigates to the sub-agent chat.
 */
export const SubagentTool: React.FC<{
	descriptor: SubagentDescriptor;
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
	/** Show an inline VNC desktop preview (for computer-use subagents). */
	showDesktopPreview?: boolean;
	/** File ID for a completed recording (shown after tool completes). */
	recordingFileId?: string;
	/** File ID for the JPEG thumbnail of a completed recording. */
	thumbnailFileId?: string;
}> = ({
	descriptor,
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
	showDesktopPreview,
	recordingFileId,
	thumbnailFileId,
}) => {
	const [expanded, setExpanded] = useState(false);
	const { desktopChatId, onOpenDesktop } = useDesktopPanel();
	const hasPrompt = Boolean(prompt?.trim());
	const hasMessage = Boolean(message?.trim());
	const hasReport = Boolean(report?.trim());
	const hasExpandableContent = hasPrompt || hasMessage || hasReport;
	const durationLabel = shortDurationMs(durationMs);

	return (
		<div className="w-full">
			<button
				type="button"
				aria-expanded={hasExpandableContent ? expanded : undefined}
				onClick={() => hasExpandableContent && setExpanded((v) => !v)}
				className={cn(
					"border-0 bg-transparent p-0 m-0 font-[inherit] text-[inherit] text-left",
					"flex w-full items-center gap-2",
					"text-content-secondary transition-colors",
					hasExpandableContent && "cursor-pointer hover:text-content-primary",
				)}
			>
				<SubagentStatusIcon
					subagentStatus={subagentStatus}
					toolStatus={toolStatus}
					isError={isError}
					isTimeout={isTimeout}
					iconKind={descriptor.iconKind}
					showDesktopPreview={showDesktopPreview}
				/>{" "}
				<span className="min-w-0 truncate text-[13px]">
					{getSubagentLabel(
						showDesktopPreview,
						toolStatus,
						descriptor,
						title,
						isTimeout,
					)}
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
				{hasExpandableContent && (
					<ChevronDownIcon
						className={cn(
							"h-3 w-3 shrink-0 text-current transition-transform",
							expanded ? "rotate-0" : "-rotate-90",
						)}
					/>
				)}
				{durationLabel && (
					<span className="ml-auto shrink-0 text-xs">
						{`Worked for ${durationLabel}`}
					</span>
				)}
			</button>

			{showDesktopPreview && desktopChatId && toolStatus !== "completed" && (
				<div className="mt-1.5 w-fit overflow-hidden rounded-lg border border-solid border-border-default">
					<InlineDesktopPreview
						chatId={desktopChatId}
						onClick={onOpenDesktop}
					/>
				</div>
			)}

			{recordingFileId && toolStatus === "completed" && (
				<div className="mt-1.5 w-fit">
					<RecordingPreview
						recordingFileId={recordingFileId}
						thumbnailFileId={thumbnailFileId}
					/>
				</div>
			)}
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

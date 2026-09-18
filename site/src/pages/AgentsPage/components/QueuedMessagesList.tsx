import { cn } from "cn";
import {
	ArrowUpIcon,
	CornerDownLeftIcon,
	InfoIcon,
	PaperclipIcon,
	Trash2Icon,
} from "lucide-react";
import { Fragment, useEffect, useState } from "react";
import type {
	ChatQueuedMessage,
	ChatSkillInvocationPart,
	ChatTextPart,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { SkillChip } from "./ChatMessageInput/SkillChip";

type QueuedMessagesListProps = {
	messages: readonly ChatQueuedMessage[];
	onDelete: (id: number) => Promise<void> | void;
	onPromote: (id: number) => Promise<void> | void;
	className?: string;
};

type QueuedMessagePreviewPart = ChatTextPart | ChatSkillInvocationPart;

type QueuedMessageInfo = {
	previewParts: QueuedMessagePreviewPart[];
	attachmentCount: number;
	hookNotices: string[];
};

export const getQueuedMessageInfo = (
	message: ChatQueuedMessage,
): QueuedMessageInfo => {
	let attachmentCount = 0;
	const previewParts: QueuedMessagePreviewPart[] = [];
	const hookNotices: string[] = [];
	for (const part of message.content) {
		if (part.type === "file" || part.type === "workspace-file-reference") {
			attachmentCount++;
		} else if (
			part.type === "text" &&
			// A blank line after a chip still ends the preview's first line.
			(part.text?.trim() ||
				(previewParts.length > 0 && part.text?.includes("\n")))
		) {
			previewParts.push({ type: "text", text: part.text });
		} else if (part.type === "skill-invocation") {
			previewParts.push(part);
		} else if (part.type === "hook-notice" && part.text?.trim()) {
			hookNotices.push(part.text);
		}
	}

	return { previewParts, attachmentCount, hookNotices };
};

/** Cuts preview parts at the first line break, trimming text only after the cut. */
export const getFirstLinePreview = (
	parts: readonly QueuedMessagePreviewPart[],
): { parts: QueuedMessagePreviewPart[]; isTruncated: boolean } => {
	const firstLine: QueuedMessagePreviewPart[] = [];
	for (const [index, part] of parts.entries()) {
		if (part.type !== "text") {
			firstLine.push(part);
			continue;
		}
		const text = firstLine.length === 0 ? part.text.trimStart() : part.text;
		const lineBreakIndex = text.indexOf("\n");
		const line = lineBreakIndex === -1 ? text : text.slice(0, lineBreakIndex);
		if (line.trim()) {
			firstLine.push({ ...part, text: line.trim() });
		}
		if (lineBreakIndex !== -1) {
			const isTruncated =
				Boolean(text.slice(lineBreakIndex).trim()) || index < parts.length - 1;
			return { parts: firstLine, isTruncated };
		}
	}
	return { parts: firstLine, isTruncated: false };
};

const QueuedMessagePreview: React.FC<{
	parts: readonly QueuedMessagePreviewPart[];
}> = ({ parts }) => {
	if (parts.length === 0) {
		return "[Queued message]";
	}
	const firstLine = getFirstLinePreview(parts);
	return (
		<>
			{firstLine.parts.map((part, index) => (
				<Fragment key={index}>
					{index > 0 && " "}
					{part.type === "skill-invocation" ? (
						<SkillChip source={part.skill_source} name={part.skill_name} />
					) : (
						part.text
					)}
				</Fragment>
			))}
			{firstLine.isTruncated && "…"}
		</>
	);
};

type QueuedMessageAction = "delete" | "promote";

type QueuedMessageActionButtonProps = {
	label: string;
	// Defaults to label.
	tooltip?: string;
	icon: React.ReactNode;
	busy: boolean;
	disabled: boolean;
	destructive?: boolean;
	onClick: () => void;
};

const QueuedMessageActionButton: React.FC<QueuedMessageActionButtonProps> = ({
	label,
	tooltip,
	icon,
	busy,
	disabled,
	destructive = false,
	onClick,
}) => (
	<Tooltip>
		{/* A disabled button receives no pointer events, so the span hosts the tooltip. */}
		<TooltipTrigger asChild>
			<span className="inline-flex">
				<Button
					variant="subtle"
					size="icon"
					aria-label={label}
					disabled={disabled}
					onClick={onClick}
					className={cn(
						"size-6 rounded text-content-secondary hover:bg-surface-tertiary",
						destructive
							? "hover:text-content-destructive"
							: "hover:text-content-primary",
					)}
				>
					<Spinner className="h-3.5 w-3.5" loading={busy}>
						{icon}
					</Spinner>
				</Button>
			</span>
		</TooltipTrigger>
		<TooltipContent side="top">{tooltip ?? label}</TooltipContent>
	</Tooltip>
);

export const QueuedMessagesList: React.FC<QueuedMessagesListProps> = ({
	messages,
	onDelete,
	onPromote,
	className,
}) => {
	const items = messages.map((message) => ({
		id: message.id,
		...getQueuedMessageInfo(message),
	}));

	const [hoveredID, setHoveredID] = useState<number | null>(null);
	// Tracks which item has an async action in flight and what kind.
	const [pendingAction, setPendingAction] = useState<{
		id: number;
		action: QueuedMessageAction;
	} | null>(null);
	const [optimisticallyHiddenIDs, setOptimisticallyHiddenIDs] = useState<
		ReadonlySet<number>
	>(new Set());

	const hideItemOptimistically = (id: number) => {
		setOptimisticallyHiddenIDs((current) => {
			if (current.has(id)) {
				return current;
			}
			const next = new Set(current);
			next.add(id);
			return next;
		});
	};

	const restoreHiddenItem = (id: number) => {
		setOptimisticallyHiddenIDs((current) => {
			if (!current.has(id)) {
				return current;
			}
			const next = new Set(current);
			next.delete(id);
			return next;
		});
	};

	useEffect(() => {
		const liveIDs = new Set(messages.map((message) => message.id));
		setOptimisticallyHiddenIDs((current) => {
			if (current.size === 0) {
				return current;
			}
			let didChange = false;
			const next = new Set<number>();
			for (const id of current) {
				if (liveIDs.has(id)) {
					next.add(id);
					continue;
				}
				didChange = true;
			}
			return didChange ? next : current;
		});
	}, [messages]);

	// Both actions remove the row, so it is hidden optimistically.
	const runAction = async (
		id: number,
		action: QueuedMessageAction,
		run: (id: number) => Promise<void> | void,
	) => {
		setPendingAction({ id, action });
		hideItemOptimistically(id);
		try {
			await run(id);
		} catch {
			restoreHiddenItem(id);
		}
		setPendingAction((current) => (current?.id === id ? null : current));
	};

	const visibleItems = items.filter(
		(item) => !optimisticallyHiddenIDs.has(item.id),
	);

	if (visibleItems.length === 0) {
		return null;
	}

	const isBusy = pendingAction !== null;

	return (
		<div
			className={cn(
				"flex w-full flex-col max-h-[40svh] overflow-y-auto scrollbar-gutter-stable scrollbar-thin",
				className,
			)}
		>
			{visibleItems.map((item, index) => {
				const isFirst = index === 0;
				const isRowPending =
					pendingAction !== null && pendingAction.id === item.id;
				const isHovered = hoveredID === item.id;
				const showActions = isHovered || (isFirst && hoveredID === null);
				return (
					<div
						key={item.id}
						className="my-1 opacity-40 transition-opacity hover:opacity-80"
						onMouseEnter={() => setHoveredID(item.id)}
						onMouseLeave={() =>
							setHoveredID((current) => (current === item.id ? null : current))
						}
					>
						<div className="flex items-center gap-2 rounded-lg border border-solid border-border bg-surface-secondary px-3 py-2 font-sans text-sm leading-relaxed text-content-primary shadow-xs">
							<span className="min-w-0 flex-1 truncate">
								<QueuedMessagePreview parts={item.previewParts} />
							</span>
							{item.attachmentCount > 0 && (
								<span
									role="img"
									aria-label={`${item.attachmentCount} attachment${item.attachmentCount !== 1 ? "s" : ""}`}
									className="flex shrink-0 items-center gap-1 text-xs text-content-secondary"
								>
									<PaperclipIcon className="size-3" aria-hidden="true" />
									<span aria-hidden="true">{item.attachmentCount}</span>
								</span>
							)}
							{item.hookNotices.length > 0 && (
								<Tooltip>
									<TooltipTrigger asChild>
										<button
											type="button"
											aria-label={`Lifecycle hook notice: ${item.hookNotices.join(" ")}`}
											className="flex shrink-0 cursor-default items-center border-none bg-transparent p-0 text-highlight-sky"
										>
											<InfoIcon className="size-3" aria-hidden="true" />
										</button>
									</TooltipTrigger>
									<TooltipContent side="top">
										{item.hookNotices.join(" ")}
									</TooltipContent>
								</Tooltip>
							)}
							{isFirst && (
								<span
									className={cn(
										"hidden shrink-0 items-center gap-1 text-xs text-content-secondary transition-opacity sm:flex",
										showActions ? "opacity-100" : "opacity-0",
									)}
								>
									<CornerDownLeftIcon className="size-3" aria-hidden="true" />
									<span className="sr-only">Enter</span>
									to send
								</span>
							)}
							<div
								className={cn(
									"flex shrink-0 items-center gap-0.5 transition-opacity",
									showActions ? "opacity-100" : "opacity-0",
								)}
							>
								<QueuedMessageActionButton
									label="Send now"
									icon={<ArrowUpIcon className="size-3.5" />}
									busy={isRowPending && pendingAction.action === "promote"}
									disabled={isBusy}
									onClick={() => void runAction(item.id, "promote", onPromote)}
								/>
								<QueuedMessageActionButton
									label="Remove from queue"
									tooltip="Remove"
									icon={<Trash2Icon className="size-3.5" />}
									busy={isRowPending && pendingAction.action === "delete"}
									disabled={isBusy}
									destructive
									onClick={() => void runAction(item.id, "delete", onDelete)}
								/>
							</div>
						</div>
					</div>
				);
			})}
		</div>
	);
};

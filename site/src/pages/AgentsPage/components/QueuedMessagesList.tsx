import { cn } from "cn";
import {
	ArrowUpIcon,
	CornerDownLeftIcon,
	InfoIcon,
	PaperclipIcon,
	Trash2Icon,
} from "lucide-react";
import { Fragment, useEffect, useId, useState } from "react";
import type {
	ChatBusyBehavior,
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

type QueuedMessageItem = QueuedMessageInfo & {
	id: number;
	busyBehavior: ChatBusyBehavior;
};

type QueuedMessageAction = "delete" | "promote";

const lanes: readonly { busyBehavior: ChatBusyBehavior; label: string }[] = [
	{ busyBehavior: "steer", label: "Next step" },
	{ busyBehavior: "queue", label: "After turn" },
];

export const QueuedMessagesList: React.FC<QueuedMessagesListProps> = ({
	messages,
	onDelete,
	onPromote,
	className,
}) => {
	const items: QueuedMessageItem[] = messages.map((message) => ({
		...getQueuedMessageInfo(message),
		id: message.id,
		busyBehavior: message.busy_behavior,
	}));

	const [hoveredID, setHoveredID] = useState<number | null>(null);
	// Tracks which item has an async action in flight and what kind.
	const [busyItem, setBusyItem] = useState<{
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

	const handleDelete = async (id: number) => {
		setBusyItem({ id, action: "delete" });
		hideItemOptimistically(id);
		try {
			await onDelete(id);
			setBusyItem((current) => (current?.id === id ? null : current));
		} catch {
			restoreHiddenItem(id);
			setBusyItem((current) => (current?.id === id ? null : current));
		}
	};

	const handlePromote = async (id: number) => {
		setBusyItem({ id, action: "promote" });
		try {
			await onPromote(id);
			setBusyItem((current) => (current?.id === id ? null : current));
		} catch {
			setBusyItem((current) => (current?.id === id ? null : current));
		}
	};

	const visibleItems = items.filter(
		(item) => !optimisticallyHiddenIDs.has(item.id),
	);

	if (visibleItems.length === 0) {
		return null;
	}

	const firstItemID = visibleItems[0].id;
	const isBusy = busyItem !== null;

	return (
		<div
			className={cn(
				"flex w-full flex-col max-h-[40svh] overflow-y-auto scrollbar-gutter-stable scrollbar-thin",
				className,
			)}
		>
			{lanes.map(({ busyBehavior, label }) => {
				const laneItems = visibleItems.filter(
					(item) => item.busyBehavior === busyBehavior,
				);
				if (laneItems.length === 0) {
					return null;
				}
				return (
					<QueuedMessageGroup key={busyBehavior} label={label}>
						{laneItems.map((item) => {
							const isFirst = item.id === firstItemID;
							return (
								<QueuedMessageRow
									key={item.id}
									item={item}
									isFirst={isFirst}
									showActions={
										hoveredID === item.id || (isFirst && hoveredID === null)
									}
									busyAction={
										busyItem?.id === item.id ? busyItem.action : undefined
									}
									disabled={isBusy}
									onMouseEnter={() => setHoveredID(item.id)}
									onMouseLeave={() =>
										setHoveredID((current) =>
											current === item.id ? null : current,
										)
									}
									onPromote={() => void handlePromote(item.id)}
									onDelete={() => void handleDelete(item.id)}
								/>
							);
						})}
					</QueuedMessageGroup>
				);
			})}
		</div>
	);
};

type QueuedMessageGroupProps = {
	label: string;
	children: React.ReactNode;
};

const QueuedMessageGroup: React.FC<QueuedMessageGroupProps> = ({
	label,
	children,
}) => {
	const labelID = useId();
	return (
		<div role="group" aria-labelledby={labelID} className="mt-2 first:mt-0">
			<p
				id={labelID}
				className="m-0 text-xs font-medium text-content-secondary"
			>
				{label}
			</p>
			{children}
		</div>
	);
};

type QueuedMessageRowProps = {
	item: QueuedMessageItem;
	isFirst: boolean;
	showActions: boolean;
	busyAction: QueuedMessageAction | undefined;
	disabled: boolean;
	onMouseEnter: () => void;
	onMouseLeave: () => void;
	onPromote: () => void;
	onDelete: () => void;
};

const QueuedMessageRow: React.FC<QueuedMessageRowProps> = ({
	item,
	isFirst,
	showActions,
	busyAction,
	disabled,
	onMouseEnter,
	onMouseLeave,
	onPromote,
	onDelete,
}) => (
	<div
		className="my-1 opacity-40 transition-opacity focus-within:opacity-80 hover:opacity-80"
		onMouseEnter={onMouseEnter}
		onMouseLeave={onMouseLeave}
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
					"flex shrink-0 items-center gap-0.5 transition-opacity focus-within:opacity-100",
					showActions ? "opacity-100" : "opacity-0",
				)}
			>
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							variant="subtle"
							size="icon"
							aria-label="Send now"
							disabled={disabled}
							onClick={onPromote}
							className="size-6 rounded text-content-secondary hover:bg-surface-tertiary hover:text-content-primary"
						>
							{busyAction === "promote" ? (
								<Spinner className="size-3.5" loading />
							) : (
								<ArrowUpIcon className="size-3.5" />
							)}
						</Button>
					</TooltipTrigger>
					<TooltipContent side="top">Send now</TooltipContent>
				</Tooltip>
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							variant="subtle"
							size="icon"
							aria-label="Remove from queue"
							disabled={disabled}
							onClick={onDelete}
							className="size-6 rounded text-content-secondary hover:bg-surface-tertiary hover:text-content-destructive"
						>
							{busyAction === "delete" ? (
								<Spinner className="size-3.5" loading />
							) : (
								<Trash2Icon className="size-3.5" />
							)}
						</Button>
					</TooltipTrigger>
					<TooltipContent side="top">Remove</TooltipContent>
				</Tooltip>
			</div>
		</div>
	</div>
);

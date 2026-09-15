import { cn } from "cn";
import {
	ArrowUpIcon,
	CornerDownLeftIcon,
	ImageIcon,
	InfoIcon,
	PencilIcon,
	Trash2Icon,
	XIcon,
} from "lucide-react";
import { type FC, useEffect, useState } from "react";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

interface QueuedMessagesListProps {
	messages: readonly ChatQueuedMessage[];
	onDelete: (id: number) => Promise<void> | void;
	onPromote: (id: number) => Promise<void> | void;
	onEdit?: (id: number) => Promise<void> | void;
	onEndEdit?: (id: number) => Promise<void> | void;
	// While paused the server refuses edits on rows other than the head,
	// and ending the head's edit sends it.
	chatPaused?: boolean;
	className?: string;
}

interface QueuedMessageInfo {
	displayText: string;
	attachmentCount: number;
	hookNotices: string[];
}

export const getQueuedMessageInfo = (
	message: ChatQueuedMessage,
): QueuedMessageInfo => {
	let attachmentCount = 0;
	const textParts: string[] = [];
	const hookNotices: string[] = [];
	for (const part of message.content) {
		if (part.type === "file") {
			attachmentCount++;
		} else if (part.type === "text" && part.text?.trim()) {
			textParts.push(part.text);
		} else if (part.type === "hook-notice" && part.text?.trim()) {
			hookNotices.push(part.text);
		}
	}
	const rawText = textParts.join(" ").trim();

	return {
		displayText: rawText || "[Queued message]",
		attachmentCount,
		hookNotices,
	};
};

type QueuedMessageAction = "delete" | "promote" | "edit" | "end_edit";

export const QueuedMessagesList: FC<QueuedMessagesListProps> = ({
	messages,
	onDelete,
	onPromote,
	onEdit,
	onEndEdit,
	chatPaused = false,
	className,
}) => {
	const editingIndex = messages.findIndex((message) => message.editing_since);
	const items = messages.map((message, index) => {
		const { displayText, attachmentCount, hookNotices } =
			getQueuedMessageInfo(message);
		const isUnderEdit = Boolean(message.editing_since);
		const isWaitingBehindEdit = editingIndex !== -1 && index > editingIndex;
		return {
			id: message.id,
			displayText,
			attachmentCount,
			hookNotices,
			isUnderEdit,
			isWaitingBehindEdit,
			badge: isUnderEdit
				? {
						label: "Editing",
						tooltip:
							"Not sent while being edited. Messages behind it wait too.",
					}
				: isWaitingBehindEdit
					? {
							label: "Waiting",
							tooltip: "Waits behind a message that is being edited.",
						}
					: undefined,
		};
	});

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

	// Delete and promote remove the row, so they hide it optimistically.
	const runAction = async (
		id: number,
		action: QueuedMessageAction,
		run: (id: number) => Promise<void> | void,
	) => {
		const hidesRow = action === "delete" || action === "promote";
		setBusyItem({ id, action });
		if (hidesRow) {
			hideItemOptimistically(id);
		}
		try {
			await run(id);
		} catch {
			if (hidesRow) {
				restoreHiddenItem(id);
			}
		}
		setBusyItem((current) => (current?.id === id ? null : current));
	};

	const visibleItems = items.filter(
		(item) => !optimisticallyHiddenIDs.has(item.id),
	);

	if (visibleItems.length === 0) {
		return null;
	}

	const isBusy = busyItem !== null;

	return (
		<div
			className={cn(
				"flex w-full flex-col max-h-[40svh] overflow-y-auto scrollbar-gutter-stable scrollbar-thin [scrollbar-color:hsl(var(--surface-quaternary))_transparent]",
				className,
			)}
		>
			{visibleItems.map((item, index) => {
				const isFirst = index === 0;
				const isItemBusy = busyItem !== null && busyItem.id === item.id;
				const isHovered = hoveredID === item.id;
				const showActions = isHovered || (isFirst && hoveredID === null);
				const renderActionIcon = (
					action: QueuedMessageAction,
					icon: React.ReactNode,
				) =>
					isItemBusy && busyItem.action === action ? (
						<Spinner className="h-3.5 w-3.5" loading />
					) : (
						icon
					);

				return (
					<div
						key={item.id}
						className={cn(
							"my-1 transition-opacity",
							item.isWaitingBehindEdit
								? "opacity-25 hover:opacity-60"
								: "opacity-40 hover:opacity-80",
						)}
						onMouseEnter={() => setHoveredID(item.id)}
						onMouseLeave={() =>
							setHoveredID((current) => (current === item.id ? null : current))
						}
					>
						<div className="flex items-center gap-2 rounded-lg border border-solid border-border-default bg-surface-secondary px-3 py-2 font-sans text-sm leading-relaxed text-content-primary shadow-xs">
							<span className="min-w-0 flex-1 truncate">
								{item.displayText.split("\n")[0]}
								{item.displayText.includes("\n") ? "…" : ""}
							</span>
							{item.badge && (
								<Tooltip>
									<TooltipTrigger asChild>
										<Badge
											asChild
											variant={item.isUnderEdit ? "warning" : "default"}
											size="xs"
											className="shrink-0 cursor-default"
										>
											<button type="button">{item.badge.label}</button>
										</Badge>
									</TooltipTrigger>
									<TooltipContent side="top">
										{item.badge.tooltip}
									</TooltipContent>
								</Tooltip>
							)}
							{item.attachmentCount > 0 && (
								<span
									role="img"
									aria-label={`${item.attachmentCount} image attachment${item.attachmentCount !== 1 ? "s" : ""}`}
									className="flex shrink-0 items-center gap-1 text-xs text-content-secondary"
								>
									<ImageIcon className="size-3" aria-hidden="true" />
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
							{isFirst && !item.isUnderEdit && (
								<span
									className={cn(
										"flex shrink-0 items-center gap-1 text-xs text-content-secondary transition-opacity",
										showActions ? "opacity-100" : "opacity-0",
									)}
								>
									<CornerDownLeftIcon className="size-3" />
									to send
								</span>
							)}
							<div
								className={cn(
									"flex shrink-0 items-center gap-0.5 transition-opacity",
									showActions ? "opacity-100" : "opacity-0",
								)}
							>
								{item.isUnderEdit && onEndEdit && (
									<Tooltip>
										<TooltipTrigger asChild>
											<Button
												variant="subtle"
												size="icon"
												aria-label="Cancel edit"
												disabled={isBusy}
												onClick={() =>
													void runAction(item.id, "end_edit", onEndEdit)
												}
												className="size-6 rounded text-content-secondary hover:bg-surface-tertiary hover:text-content-primary"
											>
												{renderActionIcon(
													"end_edit",
													<XIcon className="size-3.5" />,
												)}
											</Button>
										</TooltipTrigger>
										<TooltipContent side="top">
											{chatPaused
												? "Cancel edit and send this message as it is"
												: "Cancel edit"}
										</TooltipContent>
									</Tooltip>
								)}
								{onEdit && (!chatPaused || item.isUnderEdit) && (
									<Tooltip>
										<TooltipTrigger asChild>
											<Button
												variant="subtle"
												size="icon"
												aria-label="Edit"
												disabled={isBusy}
												onClick={() => void runAction(item.id, "edit", onEdit)}
												className="size-6 rounded text-content-secondary hover:bg-surface-tertiary hover:text-content-primary"
											>
												{renderActionIcon(
													"edit",
													<PencilIcon className="size-3.5" />,
												)}
											</Button>
										</TooltipTrigger>
										<TooltipContent side="top">Edit</TooltipContent>
									</Tooltip>
								)}
								<Tooltip>
									<TooltipTrigger asChild>
										<Button
											variant="subtle"
											size="icon"
											aria-label="Send now"
											disabled={isBusy}
											onClick={() =>
												void runAction(item.id, "promote", onPromote)
											}
											className="size-6 rounded text-content-secondary hover:bg-surface-tertiary hover:text-content-primary"
										>
											{renderActionIcon(
												"promote",
												<ArrowUpIcon className="size-3.5" />,
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
											disabled={isBusy}
											onClick={() =>
												void runAction(item.id, "delete", onDelete)
											}
											className="size-6 rounded text-content-secondary hover:bg-surface-tertiary hover:text-content-destructive"
										>
											{renderActionIcon(
												"delete",
												<Trash2Icon className="size-3.5" />,
											)}
										</Button>
									</TooltipTrigger>
									<TooltipContent side="top">Remove</TooltipContent>
								</Tooltip>
							</div>
						</div>
					</div>
				);
			})}
		</div>
	);
};

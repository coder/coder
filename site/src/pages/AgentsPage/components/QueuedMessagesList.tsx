import {
	ArrowUpIcon,
	CornerDownLeftIcon,
	ImageIcon,
	PencilIcon,
	Trash2Icon,
} from "lucide-react";
import { type FC, useEffect, useState } from "react";
import type { ChatMessagePart, ChatQueuedMessage } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";

interface QueuedMessagesListProps {
	messages: readonly ChatQueuedMessage[];
	onDelete: (id: number) => Promise<void> | void;
	onPromote: (id: number) => Promise<void> | void;
	onEdit?: (
		id: number,
		text: string,
		fileBlocks: readonly ChatMessagePart[],
	) => void;
	editingMessageID?: number | null;
	className?: string;
}

interface QueuedMessageInfo {
	displayText: string;
	rawText: string;
	attachmentCount: number;
	fileBlocks: readonly ChatMessagePart[];
}

export const getQueuedMessageInfo = (
	message: ChatQueuedMessage,
): QueuedMessageInfo => {
	const { content } = message;
	const fileBlocks = content.filter((p) => p.type === "file");
	const rawText = content
		.filter((p) => p.type === "text")
		.map((p) => p.text)
		.filter((t): t is string => Boolean(t?.trim()))
		.join(" ")
		.trim();

	if (rawText) {
		return {
			displayText: rawText,
			rawText,
			attachmentCount: fileBlocks.length,
			fileBlocks,
		};
	}
	return {
		displayText: "[Queued message]",
		rawText: "",
		attachmentCount: fileBlocks.length,
		fileBlocks,
	};
};

export const QueuedMessagesList: FC<QueuedMessagesListProps> = ({
	messages,
	onDelete,
	onPromote,
	onEdit,
	editingMessageID = null,
	className,
}) => {
	const items = messages.map((message) => {
		const { displayText, rawText, attachmentCount, fileBlocks } =
			getQueuedMessageInfo(message);
		return {
			id: message.id,
			displayText,
			rawText,
			attachmentCount,
			fileBlocks,
		};
	});

	const [hoveredID, setHoveredID] = useState<number | null>(null);
	// Tracks which item has an async action in flight and what kind.
	const [busyItem, setBusyItem] = useState<{
		id: number;
		action: "delete" | "promote";
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
		hideItemOptimistically(id);
		try {
			await onPromote(id);
			setBusyItem((current) => (current?.id === id ? null : current));
		} catch {
			restoreHiddenItem(id);
			setBusyItem((current) => (current?.id === id ? null : current));
		}
	};

	const visibleItems = items.filter(
		(item) => !optimisticallyHiddenIDs.has(item.id),
	);

	if (visibleItems.length === 0) {
		return null;
	}

	const isBusy = busyItem !== null;

	return (
		<div className={cn("flex w-full flex-col", className)}>
			{visibleItems.map((item, index) => {
				const isEditing = item.id === editingMessageID;
				const isFirst = index === 0;
				const isItemBusy = busyItem !== null && busyItem.id === item.id;
				const isHovered = hoveredID === item.id;
				// Show actions when: first and nothing else hovered,
				// or this item is hovered, or being edited.
				const showActions =
					isEditing || isHovered || (isFirst && hoveredID === null);

				return (
					<div
						key={item.id}
						className={cn(
							"my-1 opacity-40 hover:opacity-80 transition-opacity",
							isEditing && "rounded-lg opacity-100 ring-2 ring-content-link/40",
						)}
						onMouseEnter={() => setHoveredID(item.id)}
						onMouseLeave={() =>
							setHoveredID((current) => (current === item.id ? null : current))
						}
					>
						<div className="flex items-center gap-2 rounded-lg border border-solid border-border-default bg-surface-secondary px-3 py-2 font-sans text-sm leading-relaxed text-content-primary shadow-sm">
							<span className="min-w-0 flex-1 truncate">
								{item.displayText.split("\n")[0]}
								{item.displayText.includes("\n") ? "…" : ""}
							</span>
							{item.attachmentCount > 0 && (
								<span
									role="img"
									aria-label={`${item.attachmentCount} image attachment${item.attachmentCount !== 1 ? "s" : ""}`}
									className="flex shrink-0 items-center gap-1 text-xs text-content-secondary"
								>
									<ImageIcon className="h-3 w-3" aria-hidden="true" />
									<span aria-hidden="true">{item.attachmentCount}</span>
								</span>
							)}
							{isFirst && (
								<span
									className={cn(
										"flex shrink-0 items-center gap-1 text-xs text-content-secondary transition-opacity",
										showActions ? "opacity-100" : "opacity-0",
									)}
								>
									<CornerDownLeftIcon className="h-3 w-3" />
									to send
								</span>
							)}
							<div
								className={cn(
									"flex shrink-0 items-center gap-0.5 transition-opacity",
									showActions ? "opacity-100" : "opacity-0",
								)}
							>
								{onEdit && (
									<Tooltip>
										<TooltipTrigger asChild>
											<Button
												variant="subtle"
												size="icon"
												aria-label="Edit"
												disabled={isBusy}
												onClick={() =>
													onEdit(item.id, item.rawText, item.fileBlocks)
												}
												className="size-6 rounded text-content-secondary hover:bg-surface-tertiary hover:text-content-primary"
											>
												<PencilIcon className="h-3.5 w-3.5" />
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
											onClick={() => void handlePromote(item.id)}
											className="size-6 rounded text-content-secondary hover:bg-surface-tertiary hover:text-content-primary"
										>
											{isItemBusy && busyItem.action === "promote" ? (
												<Spinner className="h-3.5 w-3.5" loading />
											) : (
												<ArrowUpIcon className="h-3.5 w-3.5" />
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
											onClick={() => void handleDelete(item.id)}
											className="size-6 rounded text-content-secondary hover:bg-surface-tertiary hover:text-content-destructive"
										>
											{isItemBusy && busyItem.action === "delete" ? (
												<Spinner className="h-3.5 w-3.5" loading />
											) : (
												<Trash2Icon className="h-3.5 w-3.5" />
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

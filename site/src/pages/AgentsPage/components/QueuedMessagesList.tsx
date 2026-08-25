import {
	ArrowUpIcon,
	CornerDownLeftIcon,
	ImageIcon,
	InfoIcon,
	PencilIcon,
	Trash2Icon,
} from "lucide-react";
import { type FC, useEffect, useState } from "react";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
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
	// Removes the message from the queue and returns its text to the composer.
	onEdit?: (id: number) => Promise<void> | void;
	editDisabledReason?: string;
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

export const QueuedMessagesList: FC<QueuedMessagesListProps> = ({
	messages,
	onDelete,
	onPromote,
	onEdit,
	editDisabledReason,
	className,
}) => {
	const items = messages.map((message) => {
		const { displayText, attachmentCount, hookNotices } =
			getQueuedMessageInfo(message);
		return { id: message.id, displayText, attachmentCount, hookNotices };
	});

	const [hoveredID, setHoveredID] = useState<number | null>(null);
	const [expandedIDs, setExpandedIDs] = useState<ReadonlySet<number>>(
		new Set(),
	);

	const toggleExpanded = (id: number) => {
		setExpandedIDs((current) => {
			const next = new Set(current);
			if (!next.delete(id)) {
				next.add(id);
			}
			return next;
		});
	};
	// Tracks which item has an async action in flight and what kind.
	const [busyItem, setBusyItem] = useState<{
		id: number;
		action: "delete" | "promote" | "edit";
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

	const handleEdit = async (id: number) => {
		if (!onEdit) {
			return;
		}
		setBusyItem({ id, action: "edit" });
		hideItemOptimistically(id);
		try {
			await onEdit(id);
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
		<div
			className={cn(
				"flex w-full flex-col max-h-[40svh] overflow-y-auto [scrollbar-gutter:stable] [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]",
				className,
			)}
		>
			{visibleItems.map((item, index) => {
				const isFirst = index === 0;
				const isItemBusy = busyItem !== null && busyItem.id === item.id;
				const isHovered = hoveredID === item.id;
				const showActions = isHovered || (isFirst && hoveredID === null);
				const isExpanded = expandedIDs.has(item.id);
				const hasMoreLines = item.displayText.includes("\n");
				// Composer drafts cannot hold files uploaded in an earlier session.
				const itemEditDisabledReason =
					item.attachmentCount > 0
						? "Queued messages with attachments cannot be edited."
						: editDisabledReason;

				return (
					<div
						key={item.id}
						className="my-1 opacity-40 transition-opacity hover:opacity-80"
						onMouseEnter={() => setHoveredID(item.id)}
						onMouseLeave={() =>
							setHoveredID((current) => (current === item.id ? null : current))
						}
					>
						<div className="flex items-center gap-2 rounded-lg border border-solid border-border-default bg-surface-secondary px-3 py-2 font-sans text-sm leading-relaxed text-content-primary shadow-sm">
							<button
								type="button"
								aria-expanded={isExpanded}
								onClick={() => toggleExpanded(item.id)}
								className={cn(
									"min-w-0 flex-1 cursor-pointer border-none bg-transparent p-0 text-left font-sans text-sm leading-relaxed text-content-primary",
									isExpanded ? "whitespace-pre-wrap break-words" : "truncate",
								)}
							>
								{isExpanded ? (
									item.displayText
								) : (
									<>
										{item.displayText.split("\n")[0]}
										{hasMoreLines ? "…" : ""}
									</>
								)}
							</button>
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
							{isFirst && (
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
												<ArrowUpIcon className="size-3.5" />
											)}
										</Button>
									</TooltipTrigger>
									<TooltipContent side="top">Send now</TooltipContent>
								</Tooltip>
								{onEdit && (
									<Tooltip>
										<TooltipTrigger asChild>
											<Button
												variant="subtle"
												size="icon"
												aria-label="Edit queued message"
												disabled={
													isBusy || itemEditDisabledReason !== undefined
												}
												onClick={() => void handleEdit(item.id)}
												className="size-6 rounded text-content-secondary hover:bg-surface-tertiary hover:text-content-primary"
											>
												{isItemBusy && busyItem.action === "edit" ? (
													<Spinner className="h-3.5 w-3.5" loading />
												) : (
													<PencilIcon className="size-3.5" />
												)}
											</Button>
										</TooltipTrigger>
										<TooltipContent side="top">
											{itemEditDisabledReason ?? "Edit in composer"}
										</TooltipContent>
									</Tooltip>
								)}
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
			})}
		</div>
	);
};

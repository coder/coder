import type { ChatQueuedMessage } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import {
	ArrowUpIcon,
	CornerDownLeftIcon,
	Loader2Icon,
	PencilIcon,
	Trash2Icon,
} from "lucide-react";
import { type FC, useCallback, useEffect, useMemo, useState } from "react";
import { cn } from "utils/cn";

interface QueuedMessagesListProps {
	messages: readonly ChatQueuedMessage[];
	onDelete: (id: number) => Promise<void> | void;
	onPromote: (id: number) => Promise<void> | void;
	onEdit?: (id: number, text: string) => void;
	editingMessageID?: number | null;
	className?: string;
}

const asRecord = (value: unknown): Record<string, unknown> | undefined => {
	if (!value || typeof value !== "object" || Array.isArray(value)) {
		return undefined;
	}
	return value as Record<string, unknown>;
};

const extractBlockText = (value: unknown): string | undefined => {
	if (typeof value === "string") {
		return value;
	}
	const record = asRecord(value);
	if (!record) {
		return undefined;
	}
	if (typeof record.text === "string") {
		return record.text;
	}
	const data = asRecord(record.data);
	if (data && typeof data.text === "string") {
		return data.text;
	}
	return undefined;
};

const extractQueuedContentText = (value: unknown): string => {
	if (typeof value === "string") {
		const trimmed = value.trim();
		if (trimmed === "") {
			return "";
		}
		if (trimmed.startsWith("[") || trimmed.startsWith("{")) {
			try {
				return extractQueuedContentText(JSON.parse(trimmed));
			} catch {
				return value;
			}
		}
		return value;
	}

	if (Array.isArray(value)) {
		const texts = value
			.map(extractBlockText)
			.filter((text): text is string => Boolean(text?.trim()));
		if (texts.length > 0) {
			return texts.join(" ");
		}
		try {
			return JSON.stringify(value);
		} catch {
			return "";
		}
	}

	const record = asRecord(value);
	if (record) {
		const text = extractBlockText(record);
		if (text?.trim()) {
			return text;
		}
		if ("content" in record) {
			const nested = extractQueuedContentText(record.content);
			if (nested.trim()) {
				return nested;
			}
		}
		try {
			return JSON.stringify(record);
		} catch {
			return "";
		}
	}

	return "";
};

const getQueuedMessageText = (message: ChatQueuedMessage): string => {
	const text = extractQueuedContentText(message.content).trim();
	return text || "Queued message";
};

export const QueuedMessagesList: FC<QueuedMessagesListProps> = ({
	messages,
	onDelete,
	onPromote,
	onEdit,
	editingMessageID = null,
	className,
}) => {
	const items = useMemo(
		() =>
			messages.map((message) => ({
				id: message.id,
				text: getQueuedMessageText(message),
			})),
		[messages],
	);

	const [hoveredID, setHoveredID] = useState<number | null>(null);
	// Tracks which item has an async action in flight and what kind.
	const [busyItem, setBusyItem] = useState<{
		id: number;
		action: "delete" | "promote";
	} | null>(null);
	const [optimisticallyHiddenIDs, setOptimisticallyHiddenIDs] = useState<
		ReadonlySet<number>
	>(new Set());

	const hideItemOptimistically = useCallback((id: number) => {
		setOptimisticallyHiddenIDs((current) => {
			if (current.has(id)) {
				return current;
			}
			const next = new Set(current);
			next.add(id);
			return next;
		});
	}, []);

	const restoreHiddenItem = useCallback((id: number) => {
		setOptimisticallyHiddenIDs((current) => {
			if (!current.has(id)) {
				return current;
			}
			const next = new Set(current);
			next.delete(id);
			return next;
		});
	}, []);

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

	const handleDelete = useCallback(
		async (id: number) => {
			setBusyItem({ id, action: "delete" });
			hideItemOptimistically(id);
			try {
				await onDelete(id);
			} catch {
				restoreHiddenItem(id);
			} finally {
				setBusyItem((current) => (current?.id === id ? null : current));
			}
		},
		[hideItemOptimistically, onDelete, restoreHiddenItem],
	);

	const handlePromote = useCallback(
		async (id: number) => {
			setBusyItem({ id, action: "promote" });
			hideItemOptimistically(id);
			try {
				await onPromote(id);
			} catch {
				restoreHiddenItem(id);
			} finally {
				setBusyItem((current) => (current?.id === id ? null : current));
			}
		},
		[hideItemOptimistically, onPromote, restoreHiddenItem],
	);

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
								{item.text.split("\n")[0]}
								{item.text.includes("\n") ? "…" : ""}
							</span>
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
												onClick={() => onEdit(item.id, item.text)}
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
												<Loader2Icon className="h-3.5 w-3.5 animate-spin" />
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
												<Loader2Icon className="h-3.5 w-3.5 animate-spin" />
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

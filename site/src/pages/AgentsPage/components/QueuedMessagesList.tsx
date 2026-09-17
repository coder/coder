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
import { type FC, type ReactNode, useEffect, useState } from "react";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import type { QueuedEditOverride } from "./ChatConversation/types";

interface QueuedMessagesListProps {
	messages: readonly ChatQueuedMessage[];
	onDelete: (id: number) => Promise<void> | void;
	onPromote: (id: number) => Promise<void> | void;
	onEdit?: (id: number) => Promise<void> | void;
	onEndEdit?: (id: number) => Promise<void> | void;
	// While paused the server refuses edits on rows other than the head,
	// and ending the head's edit sends it.
	chatPaused?: boolean;
	queuedEditOverride?: QueuedEditOverride;
	// Enter in an empty composer sends the head. False while the composer
	// edits a message, when Enter saves instead.
	enterSendsHead?: boolean;
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

interface QueuedMessageActionButtonProps {
	label: string;
	// Defaults to label.
	tooltip?: string;
	icon: ReactNode;
	busy: boolean;
	disabled: boolean;
	// Disables the button and replaces the tooltip with the reason.
	disabledReason?: string;
	destructive?: boolean;
	onClick: () => void;
}

const QueuedMessageActionButton: FC<QueuedMessageActionButtonProps> = ({
	label,
	tooltip,
	icon,
	busy,
	disabled,
	disabledReason,
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
					disabled={disabled || disabledReason !== undefined}
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
		<TooltipContent side="top">
			{disabledReason ?? tooltip ?? label}
		</TooltipContent>
	</Tooltip>
);

export const QueuedMessagesList: FC<QueuedMessagesListProps> = ({
	messages,
	onDelete,
	onPromote,
	onEdit,
	onEndEdit,
	chatPaused = false,
	queuedEditOverride,
	enterSendsHead = true,
	className,
}) => {
	const isMessageUnderEdit = (message: ChatQueuedMessage) => {
		// Only one row per chat is under edit, so a local begin also clears
		// every other row's marker.
		if (queuedEditOverride?.editing) {
			return message.id === queuedEditOverride.id;
		}
		if (queuedEditOverride?.id === message.id) {
			return false;
		}
		return Boolean(message.editing_since);
	};
	const editingIndex = messages.findIndex(isMessageUnderEdit);
	const items = messages.map((message, index) => {
		const { displayText, attachmentCount, hookNotices } =
			getQueuedMessageInfo(message);
		const isUnderEdit = isMessageUnderEdit(message);
		const isWaitingBehindEdit = editingIndex !== -1 && index > editingIndex;
		let badge: { label: string; tooltip: string } | undefined;
		if (isUnderEdit) {
			badge = {
				label: "Editing",
				tooltip: "Not sent until you finish editing.",
			};
		} else if (isWaitingBehindEdit) {
			badge = {
				label: "Waiting",
				tooltip: "Waits for the edit above to finish.",
			};
		}
		return {
			id: message.id,
			displayText,
			attachmentCount,
			hookNotices,
			isUnderEdit,
			isWaitingBehindEdit,
			badge,
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
							{isFirst && !item.isUnderEdit && enterSendsHead && (
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
									<QueuedMessageActionButton
										label="Cancel edit"
										tooltip={
											chatPaused
												? "Cancel edit and send unchanged"
												: "Cancel edit"
										}
										icon={<XIcon className="size-3.5" />}
										busy={isItemBusy && busyItem.action === "end_edit"}
										disabled={isBusy}
										onClick={() =>
											void runAction(item.id, "end_edit", onEndEdit)
										}
									/>
								)}
								{onEdit && (
									<QueuedMessageActionButton
										label="Edit"
										icon={<PencilIcon className="size-3.5" />}
										busy={isItemBusy && busyItem.action === "edit"}
										disabled={isBusy}
										disabledReason={
											chatPaused && !item.isUnderEdit
												? "Finish the current edit first."
												: undefined
										}
										onClick={() => void runAction(item.id, "edit", onEdit)}
									/>
								)}
								<QueuedMessageActionButton
									label="Send now"
									icon={<ArrowUpIcon className="size-3.5" />}
									busy={isItemBusy && busyItem.action === "promote"}
									disabled={isBusy}
									onClick={() => void runAction(item.id, "promote", onPromote)}
								/>
								<QueuedMessageActionButton
									label="Remove from queue"
									tooltip="Remove"
									icon={<Trash2Icon className="size-3.5" />}
									busy={isItemBusy && busyItem.action === "delete"}
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

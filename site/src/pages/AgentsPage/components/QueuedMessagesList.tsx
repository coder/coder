import { cn } from "cn";
import {
	ArrowUpIcon,
	CornerDownLeftIcon,
	ImageIcon,
	InfoIcon,
	Trash2Icon,
} from "lucide-react";
import { type FC, type ReactNode, useEffect, useState } from "react";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type QueuedMessagesListProps = {
	messages: readonly ChatQueuedMessage[];
	onDelete: (id: number) => Promise<void> | void;
	onPromote: (id: number) => Promise<void> | void;
	className?: string;
};

type QueuedMessageInfo = {
	displayText: string;
	attachmentCount: number;
	hookNotices: string[];
};

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

type QueuedMessageAction = "delete" | "promote";

type QueuedMessageActionButtonProps = {
	label: string;
	// Defaults to label.
	tooltip?: string;
	icon: ReactNode;
	busy: boolean;
	disabled: boolean;
	destructive?: boolean;
	onClick: () => void;
};

const QueuedMessageActionButton: FC<QueuedMessageActionButtonProps> = ({
	label,
	tooltip,
	icon,
	busy,
	disabled,
	destructive = false,
	onClick,
}) => (
	<Tooltip>
		<TooltipTrigger asChild>
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
		</TooltipTrigger>
		<TooltipContent side="top">{tooltip ?? label}</TooltipContent>
	</Tooltip>
);

export const QueuedMessagesList: FC<QueuedMessagesListProps> = ({
	messages,
	onDelete,
	onPromote,
	className,
}) => {
	const items = messages.map((message) => {
		const { displayText, attachmentCount, hookNotices } =
			getQueuedMessageInfo(message);
		return { id: message.id, displayText, attachmentCount, hookNotices };
	});

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
				"flex w-full flex-col max-h-[40svh] overflow-y-auto scrollbar-gutter-stable scrollbar-thin [scrollbar-color:hsl(var(--surface-quaternary))_transparent]",
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
						<div className="flex items-center gap-2 rounded-lg border border-solid border-border-default bg-surface-secondary px-3 py-2 font-sans text-sm leading-relaxed text-content-primary shadow-xs">
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

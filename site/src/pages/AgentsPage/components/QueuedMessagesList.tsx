import { cn } from "cn";
import {
	ArrowUpIcon,
	CornerDownLeftIcon,
	InfoIcon,
	PaperclipIcon,
	Trash2Icon,
} from "lucide-react";
import { useState } from "react";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { isPendingRowID } from "./ChatConversation/chatStore";

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
		if (part.type === "file" || part.type === "workspace-file-reference") {
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

export const QueuedMessagesList: React.FC<QueuedMessagesListProps> = ({
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
	// The action in flight for each row. Only that row's actions wait for it.
	const [busyItems, setBusyItems] = useState<
		ReadonlyMap<number, "delete" | "promote">
	>(new Map());
	const setBusy = (id: number, action: "delete" | "promote" | null) => {
		setBusyItems((current) => {
			const next = new Map(current);
			if (action) {
				next.set(id, action);
			} else {
				next.delete(id);
			}
			return next;
		});
	};
	const handleDelete = async (id: number) => {
		setBusy(id, "delete");
		try {
			await onDelete(id);
			setBusy(id, null);
		} catch {
			setBusy(id, null);
		}
	};

	const handlePromote = async (id: number) => {
		setBusy(id, "promote");
		try {
			await onPromote(id);
			setBusy(id, null);
		} catch {
			setBusy(id, null);
		}
	};

	if (items.length === 0) {
		return null;
	}

	return (
		<div
			className={cn(
				"flex w-full flex-col max-h-[40svh] overflow-y-auto scrollbar-gutter-stable scrollbar-thin",
				className,
			)}
		>
			{items.map((item, index) => {
				const isFirst = index === 0;
				const busyAction = busyItems.get(item.id);
				const isPending = isPendingRowID(item.id);
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
								{item.displayText.split("\n")[0]}
								{item.displayText.includes("\n") ? "…" : ""}
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
								<Tooltip>
									<TooltipTrigger asChild>
										<Button
											variant="subtle"
											size="icon"
											aria-label="Send now"
											disabled={busyAction !== undefined || isPending}
											onClick={() => void handlePromote(item.id)}
											className="size-6 rounded text-content-secondary hover:bg-surface-tertiary hover:text-content-primary"
										>
											{busyAction === "promote" ? (
												<Spinner className="h-3.5 w-3.5" loading />
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
											disabled={busyAction !== undefined || isPending}
											onClick={() => void handleDelete(item.id)}
											className="size-6 rounded text-content-secondary hover:bg-surface-tertiary hover:text-content-destructive"
										>
											{busyAction === "delete" ? (
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

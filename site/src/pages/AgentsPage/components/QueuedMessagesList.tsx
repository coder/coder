import { cn } from "cn";
import {
	ArrowUpIcon,
	CornerDownLeftIcon,
	InfoIcon,
	PaperclipIcon,
	PencilIcon,
	Trash2Icon,
	XIcon,
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
	onDelete?: (id: number) => Promise<void> | void;
	onPromote?: (id: number) => Promise<void> | void;
	onEdit?: (id: number) => void;
	onEndEdit?: (id: number) => Promise<void> | void;
	isChatPaused?: boolean;
	queuedMessageUnderEditID: number | null;
	// The queued row this composer edits, or null. It gets no Edit action; a
	// row under edit elsewhere keeps Edit so this client can take it over.
	composerQueuedMessageID: number | null;
	showEnterToSendHint?: boolean;
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

type QueuedMessageAction = "delete" | "promote" | "end_edit";

type QueuedMessageBadge = { label: string; tooltip: string };

type QueuedMessageItem = QueuedMessageInfo & {
	id: number;
	busyBehavior: ChatBusyBehavior;
	isUnderEdit: boolean;
	isWaitingForEdit: boolean;
	badge: QueuedMessageBadge | undefined;
};

const lanes: readonly { busyBehavior: ChatBusyBehavior; label: string }[] = [
	{ busyBehavior: "steer", label: "Next step" },
	{ busyBehavior: "queue", label: "After turn" },
];

type QueuedMessageActionButtonProps = {
	label: string;
	// Defaults to label.
	tooltip?: string;
	icon: React.ReactNode;
	busy: boolean;
	disabled: boolean;
	// Makes the button unavailable and replaces the tooltip with the reason.
	disabledReason?: string;
	destructive?: boolean;
	onClick: () => void;
};

const QueuedMessageActionButton: React.FC<QueuedMessageActionButtonProps> = ({
	label,
	tooltip,
	icon,
	busy,
	disabled,
	disabledReason,
	destructive = false,
	onClick,
}) => {
	const unavailable = disabledReason !== undefined;
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					aria-label={label}
					disabled={disabled}
					// aria-disabled leaves the button focusable and hoverable, so the
					// tooltip can show the reason.
					aria-disabled={unavailable}
					onClick={unavailable ? undefined : onClick}
					className={cn(
						"size-6 rounded text-content-secondary hover:bg-surface-tertiary",
						destructive
							? "hover:text-content-destructive"
							: "hover:text-content-primary",
						unavailable &&
							"cursor-not-allowed text-content-disabled hover:bg-transparent hover:text-content-disabled",
					)}
				>
					<Spinner className="h-3.5 w-3.5" loading={busy}>
						{icon}
					</Spinner>
				</Button>
			</TooltipTrigger>
			<TooltipContent side="top">
				{disabledReason ?? tooltip ?? label}
			</TooltipContent>
		</Tooltip>
	);
};

/**
 * Whether a row offers Send now, which moves a queue row into the steer lane.
 * A steer row needs no action: the next model call, or the next message in a
 * paused or errored chat, delivers the steer lane.
 */
export const canSendQueuedMessageNow = (
	busyBehavior: ChatBusyBehavior,
): boolean => busyBehavior !== "steer";

export const QueuedMessagesList: React.FC<QueuedMessagesListProps> = ({
	messages,
	onDelete,
	onPromote,
	onEdit,
	onEndEdit,
	isChatPaused = false,
	queuedMessageUnderEditID,
	composerQueuedMessageID,
	showEnterToSendHint,
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
	const handleDelete = async (id: number) => {
		setBusyItem({ id, action: "delete" });
		try {
			await run(id);
		} catch {
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
		setPendingAction((current) => (current?.id === id ? null : current));
	};

	if (items.length === 0) {
		return null;
	}

	const firstItemID = visibleItems[0].id;
	const isBusy = pendingAction !== null;

	return (
		<div
			className={cn(
				"flex w-full flex-col max-h-[40svh] overflow-y-auto scrollbar-gutter-stable scrollbar-thin",
				className,
			)}
		>
			{items.map((item, index) => {
				const isFirst = index === 0;
				const isItemBusy = busyItem !== null && busyItem.id === item.id;
				const isPending = isPendingRowID(item.id);
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
						<div className="flex items-center gap-2 rounded-lg border border-solid border-border bg-surface-secondary px-3 py-2 font-sans text-sm leading-relaxed text-content-primary shadow-xs">
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
											size="sm"
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
							{isFirst && showEnterToSendHint && (
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
											disabled={isBusy || isPending}
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
								<Tooltip>
									<TooltipTrigger asChild>
										<Button
											variant="subtle"
											size="icon"
											aria-label="Remove from queue"
											disabled={isBusy || isPending}
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
	showEnterToSendHint: boolean;
	showActions: boolean;
	pendingAction: QueuedMessageAction | undefined;
	disabled: boolean;
	isChatPaused: boolean;
	editDisabledReason: string | undefined;
	onMouseEnter: () => void;
	onMouseLeave: () => void;
	onEdit: (() => void) | undefined;
	onEndEdit: (() => void) | undefined;
	onPromote: (() => void) | undefined;
	onDelete: (() => void) | undefined;
};

const QueuedMessageRow: React.FC<QueuedMessageRowProps> = ({
	item,
	showEnterToSendHint,
	showActions,
	pendingAction,
	disabled,
	isChatPaused,
	editDisabledReason,
	onMouseEnter,
	onMouseLeave,
	onEdit,
	onEndEdit,
	onPromote,
	onDelete,
}) => (
	<div
		className={cn(
			"my-1 transition-opacity",
			item.isWaitingForEdit
				? "opacity-25 focus-within:opacity-60 hover:opacity-60"
				: "opacity-40 focus-within:opacity-80 hover:opacity-80",
		)}
		onMouseEnter={onMouseEnter}
		onMouseLeave={onMouseLeave}
	>
		<div className="flex items-center gap-2 rounded-lg border bg-surface-secondary px-3 py-2 font-sans text-sm leading-relaxed text-content-primary shadow-xs">
			<span className="min-w-0 flex-1 truncate">
				<QueuedMessagePreview parts={item.previewParts} />
			</span>
			{item.badge && (
				<Tooltip>
					<TooltipTrigger asChild>
						<Badge
							asChild
							variant={item.isUnderEdit ? "warning" : "default"}
							size="sm"
							className="shrink-0 cursor-default"
						>
							<button type="button">{item.badge.label}</button>
						</Badge>
					</TooltipTrigger>
					<TooltipContent side="top">{item.badge.tooltip}</TooltipContent>
				</Tooltip>
			)}
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
			{showEnterToSendHint && onPromote && (
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
			{(onEndEdit || onEdit || onPromote || onDelete) && (
				<div
					className={cn(
						"flex shrink-0 items-center gap-0.5 transition-opacity focus-within:opacity-100",
						showActions ? "opacity-100" : "opacity-0",
					)}
				>
					{onEndEdit && (
						<QueuedMessageActionButton
							label="Cancel edit"
							tooltip={
								isChatPaused ? "Cancel edit and send unchanged" : "Cancel edit"
							}
							icon={<XIcon className="size-3.5" />}
							busy={pendingAction === "end_edit"}
							disabled={disabled}
							onClick={onEndEdit}
						/>
					)}
					{onEdit && (
						<QueuedMessageActionButton
							label="Edit"
							icon={<PencilIcon className="size-3.5" />}
							busy={false}
							disabled={disabled}
							disabledReason={editDisabledReason}
							onClick={onEdit}
						/>
					)}
					{onPromote && (
						<QueuedMessageActionButton
							label="Send now"
							icon={<ArrowUpIcon className="size-3.5" />}
							busy={pendingAction === "promote"}
							disabled={disabled}
							onClick={onPromote}
						/>
					)}
					{onDelete && (
						<QueuedMessageActionButton
							label="Remove from queue"
							tooltip="Remove"
							icon={<Trash2Icon className="size-3.5" />}
							busy={pendingAction === "delete"}
							disabled={disabled}
							destructive
							onClick={onDelete}
						/>
					)}
				</div>
			)}
		</div>
	</div>
);

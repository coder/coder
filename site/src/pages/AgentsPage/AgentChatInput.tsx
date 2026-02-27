import type { ChatQueuedMessage } from "api/typesGenerated";
import {
	ModelSelector,
	type ModelSelectorOption,
} from "components/ai-elements";
import { Button } from "components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import {
	ArrowUpIcon,
	ListPlusIcon,
	Loader2Icon,
	Square,
	XIcon,
} from "lucide-react";
import {
	memo,
	type ReactNode,
	useCallback,
	useEffect,
	useRef,
	useState,
} from "react";
import TextareaAutosize from "react-textarea-autosize";
import { formatProviderLabel } from "./modelOptions";
import { QueuedMessagesList } from "./QueuedMessagesList";

export interface AgentContextUsage {
	readonly usedTokens?: number;
	readonly contextLimitTokens?: number;
	readonly inputTokens?: number;
	readonly outputTokens?: number;
	readonly cacheReadTokens?: number;
	readonly cacheCreationTokens?: number;
	readonly reasoningTokens?: number;
	// Percentage (0–100) at which the context will be compacted.
	readonly compressionThreshold?: number;
}

interface AgentChatInputProps {
	onSend: (message: string, editedMessageID?: number) => Promise<void>;
	placeholder?: string;
	isDisabled: boolean;
	isLoading: boolean;
	// Optional initial value for the textarea (e.g. restored from
	// localStorage on the create page).
	initialValue?: string;
	// Fires whenever the textarea value changes, useful for persisting
	// the draft externally.
	onInputChange?: (value: string) => void;
	// Model selector.
	selectedModel: string;
	onModelChange: (value: string) => void;
	modelOptions: readonly ModelSelectorOption[];
	modelSelectorPlaceholder: string;
	hasModelOptions: boolean;
	// Status messages.
	inputStatusText: string | null;
	modelCatalogStatusMessage: string | null;
	// Streaming controls (optional, for the detail page).
	isStreaming?: boolean;
	onInterrupt?: () => void;
	isInterruptPending?: boolean;
	// Extra controls rendered in the left action area (e.g. workspace
	// selector on the create page).
	leftActions?: ReactNode;
	// Queued user messages rendered above the textarea.
	queuedMessages?: readonly ChatQueuedMessage[];
	onDeleteQueuedMessage?: (id: number) => Promise<void> | void;
	onPromoteQueuedMessage?: (id: number) => Promise<void> | void;

	// Optional context-usage summary shown to the left of the send button.
	// Pass `null` to render fallback values (e.g. when limit is unknown).
	// Omit entirely to hide the indicator.
	contextUsage?: AgentContextUsage | null;
	// When true the entire input sticks to the bottom of the scroll
	// container (used in the detail page).
	sticky?: boolean;
	// External edit request — when set, replaces the input text and
	// focuses the textarea. Use a unique `key` to allow re-editing the
	// same text.
	editRequest?: { text: string; messageId?: number; key: number } | null;
	// Called when the user cancels or completes a history edit so the
	// parent can clear the editing highlight.
	onEditCleared?: () => void;
}

const hasFiniteTokenValue = (value: number | undefined): value is number =>
	typeof value === "number" && Number.isFinite(value) && value >= 0;

const formatTokenCount = (value: number | undefined): string =>
	hasFiniteTokenValue(value) ? value.toLocaleString() : "--";

const formatTokenCountCompact = (value: number | undefined): string => {
	if (!hasFiniteTokenValue(value)) {
		return "--";
	}
	if (value >= 1_000_000) {
		const m = value / 1_000_000;
		return `${Number.isInteger(m) ? m : m.toFixed(1).replace(/\.0$/, "")}M`;
	}
	if (value >= 1_000) {
		const k = value / 1_000;
		return `${Number.isInteger(k) ? k : k.toFixed(1).replace(/\.0$/, "")}K`;
	}
	return String(value);
};

const getIndicatorToneClassName = (percentUsed: number | null): string => {
	if (percentUsed === null) {
		return "text-content-secondary/60";
	}
	if (percentUsed >= 95) {
		return "text-content-destructive";
	}
	if (percentUsed >= 85) {
		return "text-content-warning";
	}
	return "text-content-secondary/60";
};

const RING_SIZE = 18;
const RING_STROKE = 2.5;
const RING_RADIUS = (RING_SIZE - RING_STROKE) / 2;
const RING_CIRCUMFERENCE = 2 * Math.PI * RING_RADIUS;

const ContextUsageIndicator = memo<{ usage: AgentContextUsage | null }>(
	({ usage }) => {
		const usedTokens = hasFiniteTokenValue(usage?.usedTokens)
			? usage.usedTokens
			: undefined;
		const contextLimitTokens = hasFiniteTokenValue(usage?.contextLimitTokens)
			? usage.contextLimitTokens
			: undefined;
		const percentUsed =
			usedTokens !== undefined &&
			contextLimitTokens !== undefined &&
			contextLimitTokens > 0
				? (usedTokens / contextLimitTokens) * 100
				: null;
		const hasPercent = percentUsed !== null;
		const percentLabel =
			percentUsed === null ? "--" : `${Math.round(percentUsed)}%`;
		const indicatorLabel = null;
		const clampedPercent = hasPercent
			? Math.min(Math.max(percentUsed, 0), 100)
			: 100;
		const dashOffset =
			RING_CIRCUMFERENCE - (clampedPercent / 100) * RING_CIRCUMFERENCE;
		const toneClassName = getIndicatorToneClassName(percentUsed);
		const ariaLabel = hasPercent
			? `Context usage ${percentLabel}. ${formatTokenCount(usedTokens)} of ${formatTokenCount(contextLimitTokens)} tokens used.`
			: "Context usage";

		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<span
						role="button"
						tabIndex={0}
						aria-label={ariaLabel}
						className="relative inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full outline-none transition-colors hover:bg-surface-secondary/60 focus-visible:ring-2 focus-visible:ring-content-link/40"
					>
						<svg
							className={`h-5 w-5 -rotate-90 ${toneClassName}`}
							viewBox={`0 0 ${RING_SIZE} ${RING_SIZE}`}
							aria-hidden
						>
							<circle
								cx={RING_SIZE / 2}
								cy={RING_SIZE / 2}
								r={RING_RADIUS}
								fill="none"
								strokeWidth={RING_STROKE}
								className="stroke-content-secondary/25"
							/>
							<circle
								cx={RING_SIZE / 2}
								cy={RING_SIZE / 2}
								r={RING_RADIUS}
								fill="none"
								strokeWidth={RING_STROKE}
								strokeLinecap="round"
								className="stroke-current transition-all duration-300 ease-out"
								style={{
									strokeDasharray: `${RING_CIRCUMFERENCE} ${RING_CIRCUMFERENCE}`,
									strokeDashoffset: dashOffset,
								}}
							/>
						</svg>
						{indicatorLabel !== null && (
							<span className="pointer-events-none absolute text-[7px] font-semibold tabular-nums text-content-secondary">
								{indicatorLabel}
							</span>
						)}
					</span>
				</TooltipTrigger>
				<TooltipContent side="top">
					<div className="text-xs text-content-primary">
						{hasPercent
							? `${percentLabel} – ${formatTokenCountCompact(usedTokens)} / ${formatTokenCountCompact(contextLimitTokens)} context used`
							: "Context usage unavailable"}
						{hasPercent &&
							usage?.compressionThreshold !== undefined &&
							usage.compressionThreshold > 0 && (
								<div className="mt-1 text-content-secondary">
									Compacts at {usage.compressionThreshold}%
								</div>
							)}
					</div>
				</TooltipContent>
			</Tooltip>
		);
	},
);
ContextUsageIndicator.displayName = "ContextUsageIndicator";

export const AgentChatInput = memo<AgentChatInputProps>(
	({
		onSend,
		placeholder = "Type a message...",
		isDisabled,
		isLoading,
		initialValue = "",
		onInputChange,
		selectedModel,
		onModelChange,
		modelOptions,
		modelSelectorPlaceholder,
		hasModelOptions,
		inputStatusText,
		modelCatalogStatusMessage,
		isStreaming = false,
		onInterrupt,
		isInterruptPending = false,
		leftActions,
		queuedMessages = [],
		onDeleteQueuedMessage,
		onPromoteQueuedMessage,
		contextUsage,
		sticky = false,
		editRequest = null,
		onEditCleared,
	}) => {
		const [input, setInput] = useState(initialValue);
		const [editingQueuedMessageID, setEditingQueuedMessageID] = useState<
			number | null
		>(null);
		const [draftBeforeQueueEdit, setDraftBeforeQueueEdit] = useState<
			string | null
		>(null);
		const textareaRef = useRef<HTMLTextAreaElement>(null);

		// Handle external edit requests (e.g. clicking a historical
		// user message's edit icon).
		const lastEditKeyRef = useRef<number | null>(null);
		const [isEditingHistoryMessage, setIsEditingHistoryMessage] =
			useState(false);
		const [editingHistoryMessageID, setEditingHistoryMessageID] = useState<
			number | null
		>(null);
		const [draftBeforeHistoryEdit, setDraftBeforeHistoryEdit] = useState<
			string | null
		>(null);
		useEffect(() => {
			if (!editRequest || editRequest.key === lastEditKeyRef.current) {
				return;
			}
			lastEditKeyRef.current = editRequest.key;
			setDraftBeforeHistoryEdit((current) =>
				isEditingHistoryMessage ? current : input,
			);
			setIsEditingHistoryMessage(true);
			setEditingHistoryMessageID(editRequest.messageId ?? null);
			setInput(editRequest.text);
			onInputChange?.(editRequest.text);
			textareaRef.current?.focus();
		}, [editRequest, input, isEditingHistoryMessage, onInputChange]);

		const handleCancelHistoryEdit = useCallback(() => {
			if (!isEditingHistoryMessage) {
				return;
			}
			const restored = draftBeforeHistoryEdit ?? "";
			setIsEditingHistoryMessage(false);
			setEditingHistoryMessageID(null);
			setDraftBeforeHistoryEdit(null);
			setInput(restored);
			onInputChange?.(restored);
			onEditCleared?.();
			textareaRef.current?.focus();
		}, [
			draftBeforeHistoryEdit,
			isEditingHistoryMessage,
			onEditCleared,
			onInputChange,
		]);

		useEffect(() => {
			if (editingQueuedMessageID === null) {
				return;
			}
			const stillQueued = queuedMessages.some(
				(message) => message.id === editingQueuedMessageID,
			);
			if (stillQueued) {
				return;
			}
			setEditingQueuedMessageID(null);
			setDraftBeforeQueueEdit(null);
		}, [editingQueuedMessageID, queuedMessages]);

		const handleSubmit = useCallback(async () => {
			const text = input.trim();
			if (!text || isDisabled || !hasModelOptions) {
				return;
			}

			const queueEditID = editingQueuedMessageID;
			const editedMessageID =
				isEditingHistoryMessage && editingHistoryMessageID !== null
					? editingHistoryMessageID
					: undefined;
			try {
				await onSend(input, editedMessageID);
				if (queueEditID !== null && onDeleteQueuedMessage) {
					await onDeleteQueuedMessage(queueEditID);
				}
				setInput("");
				onInputChange?.("");
				if (queueEditID !== null) {
					setEditingQueuedMessageID(null);
					setDraftBeforeQueueEdit(null);
				}
				if (isEditingHistoryMessage) {
					setIsEditingHistoryMessage(false);
					setEditingHistoryMessageID(null);
					setDraftBeforeHistoryEdit(null);
					onEditCleared?.();
				}
			} catch {
				// Keep input on failure so the user can retry.
			} finally {
				// Re-focus the textarea so the user can keep typing.
				textareaRef.current?.focus();
			}
		}, [
			editingQueuedMessageID,
			editingHistoryMessageID,
			hasModelOptions,
			input,
			isDisabled,
			isEditingHistoryMessage,
			onDeleteQueuedMessage,
			onEditCleared,
			onInputChange,
			onSend,
		]);

		const handleStartQueueEdit = useCallback(
			(id: number, text: string) => {
				setDraftBeforeQueueEdit((current) =>
					editingQueuedMessageID === null ? input : current,
				);
				setEditingQueuedMessageID(id);
				setInput(text);
				onInputChange?.(text);
				textareaRef.current?.focus();
			},
			[editingQueuedMessageID, input, onInputChange],
		);

		const handleCancelQueueEdit = useCallback(() => {
			if (editingQueuedMessageID === null) {
				return;
			}
			const restored = draftBeforeQueueEdit ?? "";
			setEditingQueuedMessageID(null);
			setDraftBeforeQueueEdit(null);
			setInput(restored);
			onInputChange?.(restored);
			textareaRef.current?.focus();
		}, [draftBeforeQueueEdit, editingQueuedMessageID, onInputChange]);

		const sendButtonLabel =
			isStreaming && editingQueuedMessageID === null ? "Queue message" : "Send";

		const handleKeyDown = useCallback(
			(e: React.KeyboardEvent) => {
				if (e.key === "Escape") {
					if (editingQueuedMessageID !== null) {
						e.preventDefault();
						handleCancelQueueEdit();
					} else if (isEditingHistoryMessage) {
						e.preventDefault();
						handleCancelHistoryEdit();
					} else if (isStreaming && onInterrupt && !isInterruptPending) {
						e.preventDefault();
						onInterrupt();
					}
					return;
				}
				if (e.key === "Enter" && !e.shiftKey) {
					e.preventDefault();
					// If the input is empty and there are queued messages,
					// promote the first one instead of submitting.
					if (
						!input.trim() &&
						queuedMessages.length > 0 &&
						onPromoteQueuedMessage
					) {
						void onPromoteQueuedMessage(queuedMessages[0].id);
						return;
					}
					void handleSubmit();
				}
			},
			[
				editingQueuedMessageID,
				handleCancelHistoryEdit,
				handleCancelQueueEdit,
				handleSubmit,
				input,
				isEditingHistoryMessage,
				isInterruptPending,
				isStreaming,
				onInterrupt,
				onPromoteQueuedMessage,
				queuedMessages,
			],
		);

		const content = (
			<div className="mx-auto w-full max-w-3xl pb-4">
				{queuedMessages.length > 0 && (
					<QueuedMessagesList
						messages={queuedMessages}
						onDelete={(id) => {
							if (id === editingQueuedMessageID) {
								handleCancelQueueEdit();
							}
							void onDeleteQueuedMessage?.(id);
						}}
						onPromote={(id) => {
							if (id === editingQueuedMessageID) {
								handleCancelQueueEdit();
							}
							void onPromoteQueuedMessage?.(id);
						}}
						onEdit={handleStartQueueEdit}
						editingMessageID={editingQueuedMessageID}
						className="mb-2"
					/>
				)}
				<div className="rounded-2xl border border-border-default/80 bg-surface-secondary/45 p-1 shadow-sm has-[textarea:focus]:ring-2 has-[textarea:focus]:ring-content-link/40">
					{editingQueuedMessageID !== null && (
						<div className="flex items-center justify-between border-b border-border-default/70 bg-surface-primary/25 px-3 py-1.5">
							<span className="text-sm text-content-secondary">
								Editing queued message
							</span>
							<Button
								type="button"
								variant="subtle"
								size="sm"
								onClick={handleCancelQueueEdit}
								className="h-7 px-2 text-content-secondary hover:text-content-primary"
							>
								Cancel
							</Button>
						</div>
					)}
					{isEditingHistoryMessage && editingQueuedMessageID === null && (
						<div className="flex items-center justify-between border-b border-border-default/70 px-3 py-1.5">
							<span className="flex items-center gap-1.5 text-sm text-content-secondary">
								{isLoading && (
									<Loader2Icon className="h-3.5 w-3.5 animate-spin" />
								)}
								{isLoading ? "Saving edit..." : "Editing message"}
							</span>
							<Button
								type="button"
								variant="subtle"
								size="icon"
								aria-label="Cancel editing"
								onClick={handleCancelHistoryEdit}
								disabled={isLoading}
								className="size-6 rounded text-content-secondary hover:text-content-primary"
							>
								<XIcon className="h-3.5 w-3.5" />
							</Button>
						</div>
					)}
					<TextareaAutosize
						ref={textareaRef}
						aria-label="Chat message"
						className="min-h-[120px] w-full resize-none border-none bg-transparent px-3 py-2 font-sans text-[15px] leading-6 text-content-primary outline-none placeholder:text-content-secondary disabled:cursor-not-allowed disabled:opacity-70"
						placeholder={placeholder}
						value={input}
						onChange={(e) => {
							setInput(e.target.value);
							onInputChange?.(e.target.value);
						}}
						onKeyDown={handleKeyDown}
						disabled={isDisabled}
						minRows={4}
					/>
					<div className="flex items-center justify-between gap-2 px-2.5 pb-1.5">
						<div className="flex min-w-0 items-center gap-2">
							<ModelSelector
								value={selectedModel}
								onValueChange={onModelChange}
								options={modelOptions}
								disabled={isDisabled}
								placeholder={modelSelectorPlaceholder}
								formatProviderLabel={formatProviderLabel}
								dropdownSide="top"
								dropdownAlign="center"
								className=""
							/>
							{leftActions}
							{inputStatusText && (
								<span className="hidden text-xs text-content-secondary sm:inline">
									{inputStatusText}
								</span>
							)}
						</div>
						<div className="flex items-center gap-2">
							{contextUsage !== undefined && (
								<ContextUsageIndicator usage={contextUsage} />
							)}
							{isStreaming && onInterrupt && (
								<Button
									size="icon"
									variant="default"
									className="size-7 rounded-full transition-colors"
									onClick={onInterrupt}
									disabled={isInterruptPending}
								>
									<Square className="h-3 w-3 fill-current" />
									<span className="sr-only">Stop</span>
								</Button>
							)}
							<Button
								size="icon"
								variant="default"
								className="size-7 rounded-full transition-colors [&>svg]:!size-6 flex items-center justify-center"
								onClick={() => void handleSubmit()}
								disabled={isDisabled || !hasModelOptions || !input.trim()}
								title={sendButtonLabel}
							>
								{isLoading ? (
									<Loader2Icon className="animate-spin" />
								) : isStreaming && editingQueuedMessageID === null ? (
									<ListPlusIcon />
								) : (
									<ArrowUpIcon />
								)}
								<span className="sr-only">{sendButtonLabel}</span>
							</Button>
						</div>
					</div>
					{inputStatusText && (
						<div className="px-2.5 pb-1 text-xs text-content-secondary sm:hidden">
							{inputStatusText}
						</div>
					)}
					{modelCatalogStatusMessage && (
						<div className="px-2.5 pb-1 text-2xs text-content-secondary">
							{modelCatalogStatusMessage}
						</div>
					)}
				</div>
			</div>
		);

		if (sticky) {
			return (
				<div className="sticky bottom-0 z-50 bg-surface-primary">{content}</div>
			);
		}

		return content;
	},
);
AgentChatInput.displayName = "AgentChatInput";

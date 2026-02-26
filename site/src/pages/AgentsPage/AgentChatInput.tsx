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
import { ArrowUpIcon, ListPlusIcon, Loader2Icon, Square } from "lucide-react";
import { PromptControls, PromptShell, PromptTextarea } from "modules/prompts";
import { memo, type ReactNode, useCallback, useRef, useState } from "react";
import { formatProviderLabel } from "./modelOptions";

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
	onSend: (message: string) => Promise<void>;
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

	// Optional context-usage summary shown to the left of the send button.
	// Pass `null` to render fallback values (e.g. when limit is unknown).
	// Omit entirely to hide the indicator.
	contextUsage?: AgentContextUsage | null;
	// When true the entire input sticks to the bottom of the scroll
	// container (used in the detail page).
	sticky?: boolean;
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
		const breakdown = [
			{ key: "input", label: "Input", value: usage?.inputTokens },
			{ key: "output", label: "Output", value: usage?.outputTokens },
			{ key: "cache-read", label: "Cache read", value: usage?.cacheReadTokens },
			{
				key: "cache-create",
				label: "Cache creation",
				value: usage?.cacheCreationTokens,
			},
			{ key: "reasoning", label: "Reasoning", value: usage?.reasoningTokens },
		].filter((entry): entry is { key: string; label: string; value: number } =>
			hasFiniteTokenValue(entry.value),
		);
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
		contextUsage,
		sticky = false,
	}) => {
		const [input, setInput] = useState(initialValue);
		const textareaRef = useRef<HTMLTextAreaElement>(null);

		const handleSubmit = useCallback(async () => {
			const text = input.trim();
			if (!text || isDisabled || !hasModelOptions) {
				return;
			}
			try {
				await onSend(input);
				setInput("");
				onInputChange?.("");
			} catch {
				// Keep input on failure so the user can retry.
			} finally {
				// Re-focus the textarea so the user can keep typing.
				textareaRef.current?.focus();
			}
		}, [input, isDisabled, hasModelOptions, onSend, onInputChange]);

		const handleKeyDown = useCallback(
			(e: React.KeyboardEvent) => {
				if (e.key === "Enter" && !e.shiftKey) {
					e.preventDefault();
					void handleSubmit();
				}
			},
			[handleSubmit],
		);

		const sendIcon = isLoading ? (
			<Loader2Icon className="animate-spin" />
		) : isStreaming ? (
			<ListPlusIcon />
		) : (
			<ArrowUpIcon />
		);

		return (
			<div className="mx-auto w-full max-w-3xl pb-4">
				<PromptShell
					sticky={sticky}
					disabled={isDisabled}
					className="bg-surface-primary"
				>
					<PromptTextarea
						ref={textareaRef}
						aria-label="Chat message"
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
					<PromptControls
						leftActions={
							<>
								<ModelSelector
									value={selectedModel}
									onValueChange={onModelChange}
									options={modelOptions}
									disabled={isDisabled}
									placeholder={modelSelectorPlaceholder}
									formatProviderLabel={formatProviderLabel}
									dropdownSide="top"
									dropdownAlign="center"
									className="[&>span]:!text-content-secondary"
								/>
								{leftActions}
								{inputStatusText && (
									<span className="hidden text-xs text-content-secondary sm:inline">
										{inputStatusText}
									</span>
								)}
							</>
						}
						rightActions={
							<>
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
									title={isStreaming ? "Queue message" : "Send"}
								>
									{sendIcon}
									<span className="sr-only">
										{isStreaming ? "Queue message" : "Send"}
									</span>
								</Button>
							</>
						}
						statusMessages={
							<>
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
							</>
						}
					/>
				</PromptShell>
			</div>
		);
	},
);
AgentChatInput.displayName = "AgentChatInput";

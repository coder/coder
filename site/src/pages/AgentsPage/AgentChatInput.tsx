import {
	ModelSelector,
	type ModelSelectorOption,
} from "components/ai-elements";
import { Button } from "components/Button/Button";
import { Input } from "components/Input/Input";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { ArrowUpIcon, ListPlusIcon, Loader2Icon, Square } from "lucide-react";
import { memo, type ReactNode, useCallback, useState } from "react";
import TextareaAutosize from "react-textarea-autosize";
import { formatProviderLabel } from "./modelOptions";

export interface AgentContextUsage {
	readonly usedTokens?: number;
	readonly contextLimitTokens?: number;
	readonly inputTokens?: number;
	readonly outputTokens?: number;
	readonly cacheReadTokens?: number;
	readonly cacheCreationTokens?: number;
	readonly reasoningTokens?: number;
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
	// Whether there are already queued messages waiting.
	hasQueuedMessages?: boolean;
	// Extra controls rendered in the left action area (e.g. workspace
	// selector on the create page).
	leftActions?: ReactNode;
	// Optional context compression threshold percentage input (0-100).
	contextCompressionThreshold?: string;
	onContextCompressionThresholdChange?: (value: string) => void;
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
	return "text-content-link";
};

const RING_SIZE = 22;
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
		const indicatorLabel = hasPercent
			? percentUsed >= 100
				? "100+"
				: `${Math.round(percentUsed)}`
			: null;
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
		].filter(
			(entry): entry is { key: string; label: string; value: number } =>
				hasFiniteTokenValue(entry.value),
		);
		const ariaLabel = hasPercent
			? `Context usage ${percentLabel}. ${formatTokenCount(usedTokens)} of ${formatTokenCount(contextLimitTokens)} tokens used.`
			: "Context usage";

		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<span
						tabIndex={0}
						aria-label={ariaLabel}
						className="relative inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-full outline-none transition-colors hover:bg-surface-secondary/60 focus-visible:ring-2 focus-visible:ring-content-link/40"
					>
						<svg
							className={`h-6 w-6 -rotate-90 ${toneClassName}`}
							viewBox={`0 0 ${RING_SIZE} ${RING_SIZE}`}
							aria-hidden
						>
							<circle
								cx={RING_SIZE / 2}
								cy={RING_SIZE / 2}
								r={RING_RADIUS}
								fill="none"
								strokeWidth={RING_STROKE}
								className="stroke-border-default/70"
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
				<TooltipContent
					side="top"
					className="min-w-[10rem] max-w-[14rem] rounded-lg border-border-default/60 bg-surface-primary px-3 py-2.5 shadow-md"
				>
					<div className="space-y-1.5">
						<div className="flex items-center justify-between gap-4">
							<span className="text-2xs font-medium text-content-secondary">
								Context
							</span>
							{hasPercent && (
								<span className="font-mono text-2xs tabular-nums text-content-primary">
									{percentLabel}
								</span>
							)}
						</div>
						{(usedTokens !== undefined || contextLimitTokens !== undefined) && (
							<div className="font-mono text-2xs tabular-nums text-content-secondary">
								{formatTokenCount(usedTokens)} / {formatTokenCount(contextLimitTokens)} tokens
							</div>
						)}
						{breakdown.length > 0 && (
							<div className="space-y-px border-t border-border-default/40 pt-1.5">
								{breakdown.map((entry) => (
									<div
										key={entry.key}
										className="flex items-center justify-between gap-3 text-2xs"
									>
										<span className="text-content-secondary">{entry.label}</span>
										<span className="font-mono tabular-nums text-content-primary">
											{formatTokenCount(entry.value)}
										</span>
									</div>
								))}
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
		hasQueuedMessages = false,
		leftActions,
		contextCompressionThreshold,
		onContextCompressionThresholdChange,
		contextUsage,
		sticky = false,
	}) => {
		const [input, setInput] = useState(initialValue);

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

		const content = (
			<div className="mx-auto w-full max-w-3xl pb-4">
				<div className="rounded-2xl border border-border-default/80 bg-surface-secondary/45 p-1 shadow-sm focus-within:ring-2 focus-within:ring-content-link/40">
					<TextareaAutosize
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
								dropdownAlign="start"
								className="h-8 w-auto justify-start border-none bg-transparent px-1 text-xs shadow-none hover:bg-transparent [&>span]:!text-content-secondary"
							/>
							{leftActions}
							{onContextCompressionThresholdChange &&
								contextCompressionThreshold !== undefined && (
									<div className="flex items-center gap-1">
										<Input
											type="number"
											min={0}
											max={100}
											step={1}
											value={contextCompressionThreshold}
											onChange={(event) =>
												onContextCompressionThresholdChange(
													event.target.value,
												)
											}
											className="h-7 w-16 border-border-default/70 bg-transparent px-2 text-xs"
											disabled={isDisabled}
										/>
										<span className="text-xs text-content-secondary">%</span>
									</div>
								)}
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
								className="size-7 rounded-full transition-colors [&>svg]:!size-3.5"
								onClick={() => void handleSubmit()}
								disabled={isDisabled || !hasModelOptions || !input.trim()}
								title={isStreaming ? "Queue message" : "Send"}
							>
								{isLoading ? (
									<Loader2Icon className="animate-spin" />
								) : isStreaming ? (
									<ListPlusIcon />
								) : (
									<ArrowUpIcon />
								)}
								<span className="sr-only">
									{isStreaming ? "Queue message" : "Send"}
								</span>
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

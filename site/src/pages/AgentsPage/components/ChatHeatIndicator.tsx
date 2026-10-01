import { ClockIcon, FlameIcon } from "lucide-react";
import { useState } from "react";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useMediaQuery } from "#/hooks/useMediaQuery";
import { useTime } from "#/hooks/useTime";
import { coarsePointerMediaQuery, isMobileViewport } from "#/utils/mobile";
import {
	CACHE_IDLE_TTL_MS,
	type ChatHeat,
	isCacheLikelyExpired,
} from "./ChatConversation/chatHeat";
import { formatTokenCountCompact } from "./ContextUsageIndicator";

const HEAT_LABELS: Record<ChatHeat["label"], string> = {
	cool: "Cool",
	warm: "Warm",
	hot: "Hot",
};

const EXPIRY_CHECK_INTERVAL_MS = 15_000;
const CACHE_IDLE_TTL_MINUTES = CACHE_IDLE_TTL_MS / 60_000;

// Blends cool to warm over the first half of the heat range and warm to hot
// over the second half.
const heatColor = (heat: number): string => {
	const clamped = Math.min(Math.max(heat, 0), 1);
	if (clamped <= 0.5) {
		const stop = Math.round((clamped / 0.5) * 100);
		return `color-mix(in oklab, var(--color-heat-cool), var(--color-heat-warm) ${stop}%)`;
	}
	const stop = Math.round(((clamped - 0.5) / 0.5) * 100);
	return `color-mix(in oklab, var(--color-heat-warm), var(--color-heat-hot) ${stop}%)`;
};

// The fill carries the hue; the outline is pulled toward the primary text
// colour so even the light yellow stop keeps at least 3:1 contrast against
// the composer.
const heatOutlineColor = (fill: string): string =>
	`color-mix(in oklab, ${fill}, var(--color-content-primary) 50%)`;

const formatPercent = (value: number): string => `${Math.round(value * 100)}%`;

type ChatHeatIndicatorProps = {
	heat: ChatHeat;
	isCacheExpired: boolean;
};

export const ChatHeatIndicator: React.FC<ChatHeatIndicatorProps> = ({
	heat,
	isCacheExpired,
}) => {
	const isCoarsePointer = useMediaQuery(coarsePointerMediaQuery);
	const fillColor = heatColor(heat.heat);
	const label = HEAT_LABELS[heat.label];
	const ariaLabel = `Chat heat: ${label}, ${formatPercent(heat.heat)}.${
		isCacheExpired ? " Cache likely expired." : ""
	}`;
	const actionHint = isCacheExpired
		? `Reply within ${CACHE_IDLE_TTL_MINUTES} minutes to reuse the cache, or compact before stepping away.`
		: heat.label === "hot"
			? `Replies after ${CACHE_IDLE_TTL_MINUTES} minutes re-send the context.`
			: undefined;

	const panelContent = (
		<div className="flex max-w-64 flex-col gap-1 text-xs text-content-primary">
			<span className="font-medium">{`Chat heat: ${label} (${formatPercent(heat.heat)})`}</span>
			<span className="text-content-secondary">
				{`Recent cache miss rate: ${formatPercent(heat.missRate)}`}
			</span>
			<span className="text-content-secondary">
				{`Last turn: ${heat.lastTurnRequestCount} ${heat.lastTurnRequestCount === 1 ? "request" : "requests"}, re-sent ${formatTokenCountCompact(heat.lastTurnMissedTokens)} of ${formatTokenCountCompact(heat.lastTurnReusableTokens)} cacheable tokens`}
			</span>
			{heat.lastTurnHasSegmentStart ? (
				<span className="text-content-secondary">
					The first request after the chat starts or is compacted has no cache
					to reuse, so it does not add heat.
				</span>
			) : (
				<span className="text-content-secondary">
					Heat rises when recent turns re-send much of the context window
					instead of reading it from the prompt cache.
				</span>
			)}
			{isCacheExpired && (
				<span className="text-content-warning">
					{`Cache likely expired. Your next message will likely resend about ${formatTokenCountCompact(heat.lastPromptTokens)} tokens without the cache.`}
				</span>
			)}
			{actionHint && (
				<span className="text-content-secondary">{actionHint}</span>
			)}
		</div>
	);

	const triggerButton = (
		<button
			type="button"
			aria-label={ariaLabel}
			className="relative inline-flex size-7 shrink-0 items-center justify-center rounded-full border-none bg-transparent p-0 outline-hidden transition-colors hover:bg-surface-secondary/60 focus-visible:ring-2 focus-visible:ring-content-link/40"
		>
			<FlameIcon
				className="size-4"
				fill={fillColor}
				style={{ color: heatOutlineColor(fillColor) }}
				aria-hidden="true"
			/>
			{isCacheExpired && (
				<span
					aria-hidden="true"
					className="absolute bottom-0 right-0 flex size-3.5 items-center justify-center rounded-full border border-solid border-surface-primary bg-content-primary text-surface-primary"
				>
					<ClockIcon className="size-2.5" strokeWidth={3} />
				</span>
			)}
		</button>
	);

	// Tooltips do not open on tap, so touch devices get a popover.
	if (isMobileViewport() || isCoarsePointer) {
		return (
			<Popover>
				<PopoverTrigger asChild>{triggerButton}</PopoverTrigger>
				<PopoverContent
					side="top"
					className="mobile-full-width-dropdown mobile-full-width-dropdown-above-composer w-auto max-w-72 px-3 py-2"
				>
					{panelContent}
				</PopoverContent>
			</Popover>
		);
	}

	return (
		<Tooltip>
			<TooltipTrigger asChild>{triggerButton}</TooltipTrigger>
			<TooltipContent side="top" className="px-3 py-2">
				{panelContent}
			</TooltipContent>
		</Tooltip>
	);
};

type LiveChatHeatIndicatorProps = {
	heat: ChatHeat;
	isStreaming: boolean;
};

/** Rechecks cache expiry on a timer while the chat is idle. */
export const LiveChatHeatIndicator: React.FC<LiveChatHeatIndicatorProps> = ({
	heat,
	isStreaming,
}) => {
	const nowMs = useTime(() => Date.now(), {
		interval: EXPIRY_CHECK_INTERVAL_MS,
		disabled: isStreaming,
	});
	const [wasStreaming, setWasStreaming] = useState(isStreaming);
	const [lastStreamEndedAtMs, setLastStreamEndedAtMs] = useState<number>();
	if (wasStreaming !== isStreaming) {
		setWasStreaming(isStreaming);
		if (!isStreaming) {
			setLastStreamEndedAtMs(Date.now());
		}
	}
	return (
		<ChatHeatIndicator
			heat={heat}
			isCacheExpired={isCacheLikelyExpired(
				heat.lastRequestAt,
				lastStreamEndedAtMs,
				nowMs,
				isStreaming,
			)}
		/>
	);
};

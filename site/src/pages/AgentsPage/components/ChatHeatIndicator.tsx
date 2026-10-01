import { ClockIcon } from "lucide-react";
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

const LEVEL_LABELS: Record<ChatHeat["label"], string> = {
	low: "Low",
	moderate: "Moderate",
	high: "High",
};

const EXPIRY_CHECK_INTERVAL_MS = 15_000;
const CACHE_IDLE_TTL_MINUTES = CACHE_IDLE_TTL_MS / 60_000;

// Blends the low stop to the moderate stop over the first half of the range
// and the moderate stop to the high stop over the second half.
const heatColor = (heat: number): string => {
	const clamped = Math.min(Math.max(heat, 0), 1);
	if (clamped <= 0.5) {
		const stop = Math.round((clamped / 0.5) * 100);
		return `color-mix(in oklab, var(--color-heat-low), var(--color-heat-moderate) ${stop}%)`;
	}
	const stop = Math.round(((clamped - 0.5) / 0.5) * 100);
	return `color-mix(in oklab, var(--color-heat-moderate), var(--color-heat-high) ${stop}%)`;
};

// The fill carries the hue; the outline is pulled toward the primary text
// colour so even the light yellow stop keeps at least 3:1 contrast against
// the composer.
const heatOutlineColor = (fill: string): string =>
	`color-mix(in oklab, ${fill}, var(--color-content-primary) 50%)`;

const formatPercent = (value: number): string => `${Math.round(value * 100)}%`;

// Gauge geometry in a 24x24 viewBox. The pivot sits low so the arc and
// needle stay clear of the expiry badge in the bottom right corner.
const GAUGE_CENTER_X = 12;
const GAUGE_CENTER_Y = 14;
const GAUGE_OUTER_RADIUS = 10.5;
const GAUGE_INNER_RADIUS = 5;
const GAUGE_NEEDLE_LENGTH = 9;

// Position 0 is the left end of the arc, 0.5 the top and 1 the right end.
const gaugePoint = (position: number, radius: number) => {
	const angle = Math.PI * (1 - position);
	return {
		x: GAUGE_CENTER_X + radius * Math.cos(angle),
		y: GAUGE_CENTER_Y - radius * Math.sin(angle),
	};
};

const formatGaugePoint = (position: number, radius: number): string => {
	const { x, y } = gaugePoint(position, radius);
	return `${x.toFixed(2)} ${y.toFixed(2)}`;
};

// The band of the arc from the left end up to position.
const gaugeBandPath = (position: number): string =>
	[
		`M ${formatGaugePoint(0, GAUGE_OUTER_RADIUS)}`,
		`A ${GAUGE_OUTER_RADIUS} ${GAUGE_OUTER_RADIUS} 0 0 1 ${formatGaugePoint(position, GAUGE_OUTER_RADIUS)}`,
		`L ${formatGaugePoint(position, GAUGE_INNER_RADIUS)}`,
		`A ${GAUGE_INNER_RADIUS} ${GAUGE_INNER_RADIUS} 0 0 0 ${formatGaugePoint(0, GAUGE_INNER_RADIUS)}`,
		"Z",
	].join(" ");

type GaugeIconProps = {
	level: number;
	fillColor: string;
	outlineColor: string;
};

const GaugeIcon: React.FC<GaugeIconProps> = ({
	level,
	fillColor,
	outlineColor,
}) => {
	const position = Math.min(Math.max(level, 0), 1);
	const tip = gaugePoint(position, GAUGE_NEEDLE_LENGTH);
	return (
		<svg
			viewBox="0 0 24 24"
			className="size-4"
			fill="none"
			strokeLinecap="round"
			strokeLinejoin="round"
			aria-hidden="true"
		>
			{position > 0 && <path d={gaugeBandPath(position)} fill={fillColor} />}
			<path d={gaugeBandPath(1)} stroke={outlineColor} strokeWidth={1.5} />
			<line
				x1={GAUGE_CENTER_X}
				y1={GAUGE_CENTER_Y}
				x2={tip.x}
				y2={tip.y}
				stroke={outlineColor}
				strokeWidth={2}
			/>
			<circle
				cx={GAUGE_CENTER_X}
				cy={GAUGE_CENTER_Y}
				r={1.75}
				fill={outlineColor}
			/>
		</svg>
	);
};

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
	const label = LEVEL_LABELS[heat.label];
	const ariaLabel = `Cache misses: ${label}, ${formatPercent(heat.heat)}.${
		isCacheExpired ? " Cache likely expired." : ""
	}`;
	const actionHint = isCacheExpired
		? `Reply within ${CACHE_IDLE_TTL_MINUTES} minutes to reuse the cache, or compact before stepping away.`
		: heat.label === "high"
			? `Replies after ${CACHE_IDLE_TTL_MINUTES} minutes re-send the context.`
			: undefined;

	const panelContent = (
		<div className="flex max-w-64 flex-col gap-1 text-xs text-content-primary">
			<span className="font-medium">{`Cache misses: ${label} (${formatPercent(heat.heat)})`}</span>
			<span className="text-content-secondary">
				{`Recent cache miss rate: ${formatPercent(heat.missRate)}`}
			</span>
			<span className="text-content-secondary">
				{`Last turn: ${heat.lastTurnRequestCount} ${heat.lastTurnRequestCount === 1 ? "request" : "requests"}, re-sent ${formatTokenCountCompact(heat.lastTurnMissedTokens)} of ${formatTokenCountCompact(heat.lastTurnReusableTokens)} cacheable tokens`}
			</span>
			{heat.lastTurnHasSegmentStart ? (
				<span className="text-content-secondary">
					The first request after the chat starts or is compacted has no cache
					to reuse, so it does not count as a miss.
				</span>
			) : (
				<span className="text-content-secondary">
					The meter rises when recent turns re-send much of the context window
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
			<GaugeIcon
				level={heat.heat}
				fillColor={fillColor}
				outlineColor={heatOutlineColor(fillColor)}
			/>
			{isCacheExpired && (
				<span
					aria-hidden="true"
					className="absolute -bottom-0.5 right-0 flex size-3.5 items-center justify-center rounded-full border border-solid border-surface-primary bg-content-primary text-surface-primary"
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

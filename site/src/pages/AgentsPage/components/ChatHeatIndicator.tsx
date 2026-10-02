import { ArrowRightLeftIcon, ClockIcon } from "lucide-react";
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
// and the moderate stop to the high stop over the second half. The blend
// follows the oklch hue arc, so the tints between two stops stay as vivid
// as the stops. Expects a level in [0, 1].
const heatColor = (level: number): string => {
	if (level <= 0.5) {
		const stop = Math.round((level / 0.5) * 100);
		return `color-mix(in oklch, var(--color-heat-low), var(--color-heat-moderate) ${stop}%)`;
	}
	const stop = Math.round(((level - 0.5) / 0.5) * 100);
	return `color-mix(in oklch, var(--color-heat-moderate), var(--color-heat-high) ${stop}%)`;
};

// The fill carries the hue; the outline is pulled toward the primary text
// colour so even the light yellow stop keeps at least 3:1 contrast against
// the composer.
const heatOutlineColor = (fill: string): string =>
	`color-mix(in oklab, ${fill}, var(--color-content-primary) 50%)`;

const formatPercent = (value: number): string => `${Math.round(value * 100)}%`;

// Gauge geometry in a 24x24 viewBox: a 225 degree dial that starts at the
// lower left and sweeps clockwise over the top to 3 o'clock. The empty
// lower right sector is where the state badge sits, so it never covers the
// band or the needle.
const GAUGE_CENTER_X = 11.5;
const GAUGE_CENTER_Y = 12.5;
const GAUGE_OUTER_RADIUS = 10.5;
const GAUGE_INNER_RADIUS = 6;
// Longer than the outer radius so the tip lands on the composer background
// outside the band.
const GAUGE_NEEDLE_LENGTH = 11.5;
const GAUGE_SWEEP_DEGREES = 225;
const GAUGE_START_DEGREES = 225;
// Keeps the needle off the flat ends of the band outline at 0 and 1.
const GAUGE_NEEDLE_MIN_POSITION = 1 / 30;
const GAUGE_TRACK_OPACITY = 0.2;
// The needle has its own colour and a halo in the page colour so it stays
// crisp over the High fill, whose hue it shares. It is drawn over the band
// outline, so the halo notches the arcs where the needle crosses them.
const GAUGE_NEEDLE_COLOR = "var(--color-heat-needle)";
const GAUGE_NEEDLE_HALO_COLOR = "var(--color-surface-primary)";
const GAUGE_NEEDLE_WIDTH = 2;
const GAUGE_NEEDLE_HALO_WIDTH = 4.5;

// Position 0 is the lower left end of the dial and 1 the right end. Angles
// are counter-clockwise from the positive x axis with y pointing up.
const gaugePoint = (position: number, radius: number) => {
	const angle =
		((GAUGE_START_DEGREES - GAUGE_SWEEP_DEGREES * position) * Math.PI) / 180;
	return {
		x: GAUGE_CENTER_X + radius * Math.cos(angle),
		y: GAUGE_CENTER_Y - radius * Math.sin(angle),
	};
};

const formatGaugePoint = (position: number, radius: number): string => {
	const { x, y } = gaugePoint(position, radius);
	return `${x.toFixed(2)} ${y.toFixed(2)}`;
};

// The band of the dial from the lower left end up to position.
const gaugeBandPath = (position: number): string => {
	const largeArc = GAUGE_SWEEP_DEGREES * position > 180 ? 1 : 0;
	return [
		`M ${formatGaugePoint(0, GAUGE_OUTER_RADIUS)}`,
		`A ${GAUGE_OUTER_RADIUS} ${GAUGE_OUTER_RADIUS} 0 ${largeArc} 1 ${formatGaugePoint(position, GAUGE_OUTER_RADIUS)}`,
		`L ${formatGaugePoint(position, GAUGE_INNER_RADIUS)}`,
		`A ${GAUGE_INNER_RADIUS} ${GAUGE_INNER_RADIUS} 0 ${largeArc} 0 ${formatGaugePoint(0, GAUGE_INNER_RADIUS)}`,
		"Z",
	].join(" ");
};

// The share is the turn's miss rate: missed tokens over the largest
// cacheable prompt. Missed tokens sum over every request in the turn, so
// they can exceed that prompt.
const formatLastTurn = (heat: ChatHeat, requests: string): string => {
	const missed = formatTokenCountCompact(heat.lastTurnMissedTokens);
	const reusable = formatTokenCountCompact(heat.lastTurnReusableTokens);
	if (heat.lastTurnMissedTokens > heat.lastTurnReusableTokens) {
		return `Last turn: ${requests} re-sent ${missed} tokens against a ${reusable} cacheable prompt.`;
	}
	const share = formatPercent(
		heat.lastTurnMissedTokens / heat.lastTurnReusableTokens,
	);
	return `Last turn: ${requests} re-sent ${missed} tokens (${share} of ${reusable} cacheable).`;
};

type CacheMissGaugeProps = {
	// Undefined when there is no reading, which draws an empty dashed grey
	// gauge without a needle.
	level: number | undefined;
};

const gaugeReading = (level: number) => {
	const position = Math.min(Math.max(level, 0), 1);
	const fillColor = heatColor(position);
	return {
		position,
		fillColor,
		outlineColor: heatOutlineColor(fillColor),
		tip: gaugePoint(
			Math.min(
				Math.max(position, GAUGE_NEEDLE_MIN_POSITION),
				1 - GAUGE_NEEDLE_MIN_POSITION,
			),
			GAUGE_NEEDLE_LENGTH,
		),
	};
};

// The track carries a faint tint of the level colour so the hue shows even
// when little of the band is filled.
const CacheMissGauge: React.FC<CacheMissGaugeProps> = ({ level }) => {
	const reading = level === undefined ? undefined : gaugeReading(level);
	const outlineColor =
		reading?.outlineColor ?? "var(--color-content-secondary)";
	const fullBand = gaugeBandPath(1);
	return (
		<svg
			viewBox="0 0 24 24"
			className="size-4 overflow-visible"
			fill="none"
			strokeLinecap="round"
			strokeLinejoin="round"
			aria-hidden="true"
		>
			{reading && (
				<path
					d={fullBand}
					fill={reading.fillColor}
					fillOpacity={GAUGE_TRACK_OPACITY}
				/>
			)}
			{reading && reading.position > 0 && (
				<path d={gaugeBandPath(reading.position)} fill={reading.fillColor} />
			)}
			<path
				d={fullBand}
				stroke={outlineColor}
				strokeWidth={1.5}
				strokeDasharray={reading ? undefined : "2.5 2.5"}
			/>
			{reading && (
				<>
					<line
						x1={GAUGE_CENTER_X}
						y1={GAUGE_CENTER_Y}
						x2={reading.tip.x}
						y2={reading.tip.y}
						stroke={GAUGE_NEEDLE_HALO_COLOR}
						strokeWidth={GAUGE_NEEDLE_HALO_WIDTH}
					/>
					<line
						x1={GAUGE_CENTER_X}
						y1={GAUGE_CENTER_Y}
						x2={reading.tip.x}
						y2={reading.tip.y}
						stroke={GAUGE_NEEDLE_COLOR}
						strokeWidth={GAUGE_NEEDLE_WIDTH}
					/>
				</>
			)}
			<circle
				cx={GAUGE_CENTER_X}
				cy={GAUGE_CENTER_Y}
				r={1.75}
				fill={reading ? GAUGE_NEEDLE_COLOR : outlineColor}
			/>
		</svg>
	);
};

type ChatHeatIndicatorProps = {
	heat: ChatHeat;
	isCacheExpired: boolean;
	// True when the next request's model differs from the last request's, so
	// it cannot read that cache. An expired cache takes precedence.
	isModelChanged: boolean;
	// False when the last request's model is no longer offered, so the user
	// cannot switch back to it.
	canSwitchModelBack: boolean;
};

export const ChatHeatIndicator: React.FC<ChatHeatIndicatorProps> = ({
	heat,
	isCacheExpired,
	isModelChanged,
	canSwitchModelBack,
}) => {
	const isCoarsePointer = useMediaQuery(coarsePointerMediaQuery);
	const hasReading = !heat.lastTurnIsPartial;
	const showModelChanged = isModelChanged && !isCacheExpired;
	const summary = hasReading
		? `Cache misses: ${LEVEL_LABELS[heat.label]} (${formatPercent(heat.heat)})`
		: "Cache misses: unknown";
	const resendTokens = formatTokenCountCompact(heat.lastPromptTokens);
	const coldCacheNote = isCacheExpired
		? `Cache likely expired. Your next message will re-send about ${resendTokens} tokens without the cache.`
		: showModelChanged
			? `The selected model differs from the last request's, so your next message will re-send about ${resendTokens} tokens without the cache.`
			: undefined;
	const ariaNotes = [`${summary}.`];
	if (!hasReading) {
		ariaNotes.push("Older messages are not loaded.");
	}
	if (isCacheExpired) {
		ariaNotes.push("Cache likely expired.");
	} else if (showModelChanged) {
		ariaNotes.push("Model changed; the next message will not use the cache.");
	}
	const actionHint = isCacheExpired
		? `Your next message rebuilds the cache. Reply within ${CACHE_IDLE_TTL_MINUTES} minutes after each response to keep it, or compact before stepping away.`
		: showModelChanged
			? canSwitchModelBack
				? "Switch back to the previous model to reuse its cache."
				: undefined
			: hasReading && heat.label === "high"
				? `To cut misses, reply within ${CACHE_IDLE_TTL_MINUTES} minutes after each response, or compact before stepping away.`
				: undefined;
	const lastTurnRequests = `${heat.lastTurnRequestCount} ${heat.lastTurnRequestCount === 1 ? "request" : "requests"}`;

	const panelContent = (
		<div className="flex max-w-64 flex-col gap-1 text-xs text-content-primary">
			<span className="font-medium">{summary}</span>
			{hasReading ? (
				<>
					<span className="text-content-secondary">
						{`Cacheable context re-sent recently: ${formatPercent(heat.missRate)}`}
					</span>
					{heat.lastTurnReusableTokens > 0 && (
						<span className="text-content-secondary">
							{formatLastTurn(heat, lastTurnRequests)}
						</span>
					)}
					{heat.lastTurnHasSegmentStart ? (
						<span className="text-content-secondary">
							The first request after the chat starts or is compacted has no
							cache to reuse, so it does not count as a miss.
						</span>
					) : (
						<span className="text-content-secondary">
							The needle moves right as recent turns re-send more of the context
							instead of reading it from the prompt cache. The latest turn
							counts most.
						</span>
					)}
				</>
			) : (
				<span className="text-content-secondary">
					{`Older messages are not loaded, so the latest turn (${lastTurnRequests} loaded) cannot be scored yet. Scroll up to load them.`}
				</span>
			)}
			{coldCacheNote && (
				<span className="text-content-warning">{coldCacheNote}</span>
			)}
			{actionHint && (
				<span className="text-content-secondary">{actionHint}</span>
			)}
		</div>
	);

	const triggerButton = (
		<button
			type="button"
			aria-label={ariaNotes.join(" ")}
			className="relative inline-flex size-7 shrink-0 items-center justify-center rounded-full border-none bg-transparent p-0 outline-hidden transition-colors hover:bg-surface-secondary/60 focus-visible:ring-2 focus-visible:ring-content-link/40"
		>
			<CacheMissGauge level={hasReading ? heat.heat : undefined} />
			{coldCacheNote && (
				<span
					aria-hidden="true"
					className="absolute -bottom-0.5 right-0 flex size-3.5 items-center justify-center rounded-full border border-solid border-surface-primary bg-content-primary text-surface-primary"
				>
					{showModelChanged ? (
						<ArrowRightLeftIcon className="size-2.5" strokeWidth={3} />
					) : (
						<ClockIcon className="size-2.5" strokeWidth={3} />
					)}
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
	selectedModelConfigId: string;
	selectableModelConfigIds: readonly string[];
};

/** Rechecks cache expiry on a timer while the chat is idle. */
export const LiveChatHeatIndicator: React.FC<LiveChatHeatIndicatorProps> = ({
	heat,
	isStreaming,
	selectedModelConfigId,
	selectableModelConfigIds,
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
			isModelChanged={Boolean(
				!isStreaming &&
					selectedModelConfigId &&
					heat.lastModelConfigId &&
					selectedModelConfigId !== heat.lastModelConfigId,
			)}
			canSwitchModelBack={
				heat.lastModelConfigId !== undefined &&
				selectableModelConfigIds.includes(heat.lastModelConfigId)
			}
		/>
	);
};

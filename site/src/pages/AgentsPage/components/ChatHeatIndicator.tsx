import { ArrowRightLeftIcon, ClockIcon } from "lucide-react";
import { useEffect, useState } from "react";
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
import { coarsePointerMediaQuery, isMobileViewport } from "#/utils/mobile";
import {
	CACHE_IDLE_TTL_MS,
	type ChatHeat,
	type ChatHeatTurn,
	getCacheExpiresAtMs,
	getRemainingMinutes,
	type NextMessageProjection,
	projectNextMessage,
} from "./ChatConversation/chatHeat";
import { formatTokenCountCompact } from "./ContextUsageIndicator";

const LEVEL_LABELS: Record<NextMessageProjection["label"], string> = {
	low: "Low",
	moderate: "Moderate",
	high: "High",
};

const LAST_MINUTE_MS = 60_000;

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

const formatCountdown = (remainingMs: number): string => {
	const totalSeconds = Math.max(0, Math.ceil(remainingMs / 1000));
	const minutes = Math.floor(totalSeconds / 60);
	const seconds = totalSeconds % 60;
	return `${minutes}:${seconds.toString().padStart(2, "0")}`;
};

const formatRemainingMinutes = (remainingMs: number): string => {
	const minutes = getRemainingMinutes(remainingMs);
	return minutes === 1 ? "under a minute" : `about ${minutes} minutes`;
};

// The projection is an estimate from a single ratio, so it is shown to the
// nearest thousand tokens.
const formatProjectedTokens = (tokens: number): string =>
	formatTokenCountCompact(
		tokens >= 1_000 ? Math.round(tokens / 1_000) * 1_000 : Math.round(tokens),
	);

const formatMinutesAgo = (elapsedMs: number): string => {
	const minutes = Math.floor(elapsedMs / 60_000);
	if (minutes < 1) {
		return "under a minute ago";
	}
	if (minutes < 60) {
		return `${minutes} ${minutes === 1 ? "minute" : "minutes"} ago`;
	}
	const hours = Math.floor(minutes / 60);
	return `${hours} ${hours === 1 ? "hour" : "hours"} ago`;
};

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
// The at-stake segment shows how far the needle will jump when the cache
// expires. A neutral grey: the band's blue to yellow to red axis and the red
// needle leave no hue that stays distinct under red/green colour blindness.
const GAUGE_AT_STAKE_COLOR = "var(--color-heat-at-stake)";

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

// The band of the dial from position `from` up to position `to`.
const gaugeBandPath = (from: number, to: number): string => {
	const largeArc = GAUGE_SWEEP_DEGREES * (to - from) > 180 ? 1 : 0;
	return [
		`M ${formatGaugePoint(from, GAUGE_OUTER_RADIUS)}`,
		`A ${GAUGE_OUTER_RADIUS} ${GAUGE_OUTER_RADIUS} 0 ${largeArc} 1 ${formatGaugePoint(to, GAUGE_OUTER_RADIUS)}`,
		`L ${formatGaugePoint(to, GAUGE_INNER_RADIUS)}`,
		`A ${GAUGE_INNER_RADIUS} ${GAUGE_INNER_RADIUS} 0 ${largeArc} 0 ${formatGaugePoint(from, GAUGE_INNER_RADIUS)}`,
		"Z",
	].join(" ");
};

const clampPosition = (level: number): number =>
	Math.min(Math.max(level, 0), 1);

type NextMessageGaugeProps = {
	// Undefined when there is no reading, which draws an empty dashed grey
	// gauge without a needle.
	level: number | undefined;
	// Where the needle would sit once the cache is cold; drawn as a faded
	// segment beyond the needle while it is above the level.
	atStakeLevel?: number;
};

const NextMessageGauge: React.FC<NextMessageGaugeProps> = ({
	level,
	atStakeLevel,
}) => {
	const position = level === undefined ? undefined : clampPosition(level);
	const fillColor = position === undefined ? undefined : heatColor(position);
	const outlineColor = fillColor
		? heatOutlineColor(fillColor)
		: "var(--color-content-secondary)";
	const atStake =
		position !== undefined && atStakeLevel !== undefined
			? clampPosition(atStakeLevel)
			: undefined;
	const needleTip =
		position === undefined
			? undefined
			: gaugePoint(
					Math.min(
						Math.max(position, GAUGE_NEEDLE_MIN_POSITION),
						1 - GAUGE_NEEDLE_MIN_POSITION,
					),
					GAUGE_NEEDLE_LENGTH,
				);
	const fullBand = gaugeBandPath(0, 1);
	return (
		<svg
			viewBox="0 0 24 24"
			className="size-4 overflow-visible"
			fill="none"
			strokeLinecap="round"
			strokeLinejoin="round"
			aria-hidden="true"
		>
			{fillColor && (
				<path d={fullBand} fill={fillColor} fillOpacity={GAUGE_TRACK_OPACITY} />
			)}
			{position !== undefined &&
				atStake !== undefined &&
				atStake > position && (
					<path
						data-testid="at-stake"
						d={gaugeBandPath(position, atStake)}
						fill={GAUGE_AT_STAKE_COLOR}
					/>
				)}
			{fillColor && position !== undefined && position > 0 && (
				<path d={gaugeBandPath(0, position)} fill={fillColor} />
			)}
			<path
				d={fullBand}
				stroke={outlineColor}
				strokeWidth={1.5}
				strokeDasharray={position === undefined ? "2.5 2.5" : undefined}
			/>
			{needleTip && (
				<>
					<line
						x1={GAUGE_CENTER_X}
						y1={GAUGE_CENTER_Y}
						x2={needleTip.x}
						y2={needleTip.y}
						stroke={GAUGE_NEEDLE_HALO_COLOR}
						strokeWidth={GAUGE_NEEDLE_HALO_WIDTH}
					/>
					<line
						x1={GAUGE_CENTER_X}
						y1={GAUGE_CENTER_Y}
						x2={needleTip.x}
						y2={needleTip.y}
						stroke={GAUGE_NEEDLE_COLOR}
						strokeWidth={GAUGE_NEEDLE_WIDTH}
					/>
				</>
			)}
			<circle
				cx={GAUGE_CENTER_X}
				cy={GAUGE_CENTER_Y}
				r={1.75}
				fill={needleTip ? GAUGE_NEEDLE_COLOR : outlineColor}
			/>
		</svg>
	);
};

// The share is the turn's miss rate: missed tokens over the largest
// cacheable prompt. Missed tokens sum over every request in the turn, so
// they can exceed that prompt.
const formatLastTurn = (turn: ChatHeatTurn): string => {
	const requests = `${turn.requestCount} ${turn.requestCount === 1 ? "request" : "requests"}`;
	const missed = formatTokenCountCompact(turn.missedTokens);
	const reusable = formatTokenCountCompact(turn.reusableTokens);
	if (turn.missedTokens > turn.reusableTokens) {
		return `Last turn: ${requests} re-sent ${missed} tokens against a ${reusable} cacheable prompt.`;
	}
	const share = formatPercent(turn.missedTokens / turn.reusableTokens);
	return `Last turn: ${requests} re-sent ${missed} tokens (${share} of ${reusable} cacheable).`;
};

type ChatHeatIndicatorProps = {
	heat: ChatHeat;
	// Milliseconds until the cache expires; negative once it has. Undefined
	// while the chat is generating or when no timing is known, which hides
	// the countdown and treats the cache as warm.
	remainingMs: number | undefined;
	// True when the next request's model differs from the last request's, so
	// it cannot read that cache. An expired cache takes precedence.
	isModelChanged: boolean;
	// False when the last request's model is no longer offered, so the user
	// cannot switch back to it.
	canSwitchModelBack: boolean;
	onOpenChange?: (open: boolean) => void;
};

export const ChatHeatIndicator: React.FC<ChatHeatIndicatorProps> = ({
	heat,
	remainingMs,
	isModelChanged,
	canSwitchModelBack,
	onOpenChange,
}) => {
	const isCoarsePointer = useMediaQuery(coarsePointerMediaQuery);
	const hasReading = heat.boundary === undefined;
	const isExpired = remainingMs !== undefined && remainingMs <= 0;
	const showModelChanged = isModelChanged && !isExpired;
	const isCold = isExpired || showModelChanged;
	const isLastMinute =
		!isCold && remainingMs !== undefined && remainingMs <= LAST_MINUTE_MS;
	const projection = projectNextMessage(heat.lastPromptTokens, isCold);
	const cold = projectNextMessage(heat.lastPromptTokens, true);
	const promptTokens = formatTokenCountCompact(heat.lastPromptTokens);
	const level = LEVEL_LABELS[projection.label];
	const countdown =
		remainingMs !== undefined && remainingMs > 0
			? formatCountdown(remainingMs)
			: undefined;

	const summary = hasReading
		? `Next message: ${level}`
		: "Next message: fresh cache";
	const readingLine = !hasReading
		? heat.boundary === "cleared"
			? "Cleared: the next message writes a fresh cache from the new context."
			: "Compacted: the next message writes a fresh cache from the summary."
		: isCold
			? `Next message: re-writes about ${promptTokens} tokens without the cache (${level}).`
			: `Next message: reads ${promptTokens} tokens from the cache, about ${formatProjectedTokens(projection.tokens)} tokens' worth of a re-write (${level}).`;
	const timerLine = !hasReading
		? undefined
		: isExpired
			? `Cache likely expired ${formatMinutesAgo(-remainingMs)}.`
			: showModelChanged
				? "The selected model differs from the last request's, so it cannot use this cache."
				: countdown
					? `Cache expires in ${countdown}.`
					: undefined;
	const hint = !hasReading
		? undefined
		: showModelChanged && canSwitchModelBack
			? countdown
				? `Switch back within ${countdown} to reuse its cache.`
				: "Switch back to the previous model to reuse its cache."
			: isCold && projection.label !== "low"
				? "Before sending, type /compact to summarise the chat or /clear to start fresh, so later replies re-write far less."
				: isLastMinute
					? "Reply now to keep it."
					: undefined;
	const lastTurnLine = heat.lastTurn
		? heat.lastTurn.isPartial
			? "Earlier requests in this turn are not loaded."
			: heat.lastTurn.reusableTokens > 0
				? formatLastTurn(heat.lastTurn)
				: undefined
		: undefined;

	// The accessible name uses whole minutes so it does not change every
	// second while the control is focused; mm:ss stays in the panel.
	const ariaNotes = [`${summary}.`];
	if (!hasReading) {
		ariaNotes.push(readingLine);
	} else {
		ariaNotes.push(
			isCold
				? `Re-writes about ${promptTokens} tokens without the cache.`
				: `Reads ${promptTokens} tokens from the cache.`,
		);
	}
	if (isExpired) {
		ariaNotes.push("Cache likely expired.");
	} else if (showModelChanged) {
		ariaNotes.push("Model changed; the next message will not use the cache.");
	} else if (countdown && remainingMs !== undefined) {
		ariaNotes.push(`Cache expires in ${formatRemainingMinutes(remainingMs)}.`);
	}

	const minutesLeft =
		hasReading && !isCold && remainingMs !== undefined && remainingMs > 0
			? getRemainingMinutes(remainingMs)
			: undefined;
	const badge = !hasReading ? null : isExpired ? (
		<ClockIcon className="size-2.5" strokeWidth={3} />
	) : showModelChanged ? (
		<ArrowRightLeftIcon className="size-2.5" strokeWidth={3} />
	) : minutesLeft !== undefined ? (
		<span className="text-[10px] font-semibold leading-none">
			{minutesLeft}
		</span>
	) : null;
	const badgeInverted = isCold || isLastMinute;

	const isPopover = isMobileViewport() || isCoarsePointer;
	const panelContent = (
		<div
			className={`flex flex-col gap-1 text-xs text-content-primary ${isPopover ? "" : "max-w-64"}`}
		>
			<span className="font-medium">{summary}</span>
			<span className="text-content-secondary">{readingLine}</span>
			{timerLine && (
				<span
					className={isCold ? "text-content-warning" : "text-content-secondary"}
				>
					{timerLine}
				</span>
			)}
			{hint && <span className="text-content-secondary">{hint}</span>}
			{lastTurnLine && (
				<span className="text-content-secondary">{lastTurnLine}</span>
			)}
		</div>
	);

	const triggerButton = (
		<button
			type="button"
			aria-label={ariaNotes.join(" ")}
			className="relative inline-flex size-7 shrink-0 items-center justify-center rounded-full border-none bg-transparent p-0 outline-hidden transition-colors hover:bg-surface-secondary/60 focus-visible:ring-2 focus-visible:ring-content-link/40"
		>
			<NextMessageGauge
				level={hasReading ? projection.heat : undefined}
				atStakeLevel={hasReading && !isCold ? cold.heat : undefined}
			/>
			{badge && (
				<span
					aria-hidden="true"
					data-testid="heat-badge"
					className={`absolute -bottom-0.5 right-0 flex size-3.5 items-center justify-center rounded-full border border-solid ${
						badgeInverted
							? "border-surface-primary bg-content-primary text-surface-primary"
							: "border-content-primary bg-surface-primary text-content-primary"
					} ${isLastMinute ? "animate-pulse motion-reduce:animate-none" : ""}`}
				>
					{badge}
				</span>
			)}
		</button>
	);

	// Tooltips do not open on tap, so touch devices get a popover.
	if (isPopover) {
		return (
			<Popover onOpenChange={onOpenChange}>
				<PopoverTrigger asChild>{triggerButton}</PopoverTrigger>
				<PopoverContent
					side="top"
					aria-label={summary}
					className="mobile-full-width-dropdown mobile-full-width-dropdown-above-composer w-auto max-w-72 px-3 py-2"
				>
					{panelContent}
				</PopoverContent>
			</Popover>
		);
	}

	return (
		<Tooltip onOpenChange={onOpenChange}>
			<TooltipTrigger asChild>{triggerButton}</TooltipTrigger>
			<TooltipContent side="top" className="px-3 py-2">
				{panelContent}
			</TooltipContent>
		</Tooltip>
	);
};

const MINUTE_MS = 60_000;

type CountdownTick = {
	expiresAtMs: number | undefined;
	perSecond: boolean;
	nowMs: number;
};

/**
 * The current time, re-read when the countdown display would change: at
 * each whole-minute boundary before expiry and at expiry itself, or every
 * second while the panel is open and shows mm:ss. Once expired the panel
 * shows elapsed minutes, so the open cadence drops to once a minute. The
 * clock is also re-read when the panel opens or closes and when the tab
 * becomes visible again, since a hidden tab's timers may have been
 * throttled.
 */
const useCountdownNow = (
	expiresAtMs: number | undefined,
	perSecond: boolean,
): number => {
	const [tick, setTick] = useState<CountdownTick>(() => ({
		expiresAtMs,
		perSecond,
		nowMs: Date.now(),
	}));
	// A new expiry (a request landed, or a stream ended) restarts the clock,
	// since the stored time may date from before a long idle period. Opening
	// the panel re-reads it too, so mm:ss starts from the real time rather
	// than the last minute tick.
	if (tick.expiresAtMs !== expiresAtMs || tick.perSecond !== perSecond) {
		setTick({ expiresAtMs, perSecond, nowMs: Date.now() });
	}
	const nowMs = tick.nowMs;
	useEffect(() => {
		if (expiresAtMs === undefined) {
			return;
		}
		const reread = () => setTick({ expiresAtMs, perSecond, nowMs: Date.now() });
		const onVisibilityChange = () => {
			if (document.visibilityState === "visible") {
				reread();
			}
		};
		document.addEventListener("visibilitychange", onVisibilityChange);
		// Measured from the real clock rather than the stored reading, so a
		// late effect does not shift the boundary the timer aims for.
		const remaining = expiresAtMs - Date.now();
		let delay: number | undefined;
		if (remaining <= 0) {
			if (perSecond) {
				delay = MINUTE_MS - (-remaining % MINUTE_MS);
			}
		} else if (perSecond) {
			delay = remaining % 1000 || 1000;
		} else {
			delay = remaining % MINUTE_MS || MINUTE_MS;
		}
		if (delay === undefined) {
			return () => {
				document.removeEventListener("visibilitychange", onVisibilityChange);
			};
		}
		// A few milliseconds past the boundary so the new reading is taken.
		const handle = setTimeout(reread, delay + 20);
		return () => {
			clearTimeout(handle);
			document.removeEventListener("visibilitychange", onVisibilityChange);
		};
		// Each re-read changes nowMs, which re-runs the effect to schedule the
		// next tick.
	}, [expiresAtMs, perSecond, nowMs]);
	return nowMs;
};

type LiveChatHeatIndicatorProps = {
	heat: ChatHeat;
	isStreaming: boolean;
	selectedModelConfigId: string;
	selectableModelConfigIds: readonly string[];
};

/** Counts down the cache lifetime while the chat is idle. */
export const LiveChatHeatIndicator: React.FC<LiveChatHeatIndicatorProps> = ({
	heat,
	isStreaming,
	selectedModelConfigId,
	selectableModelConfigIds,
}) => {
	const [wasStreaming, setWasStreaming] = useState(isStreaming);
	const [lastStreamEndedAtMs, setLastStreamEndedAtMs] = useState<number>();
	if (wasStreaming !== isStreaming) {
		setWasStreaming(isStreaming);
		if (!isStreaming) {
			setLastStreamEndedAtMs(Date.now());
		}
	}
	const [isOpen, setIsOpen] = useState(false);
	const expiresAtMs =
		isStreaming || heat.boundary !== undefined
			? undefined
			: getCacheExpiresAtMs(heat.lastRequestAt, lastStreamEndedAtMs);
	const nowMs = useCountdownNow(expiresAtMs, isOpen);
	return (
		<ChatHeatIndicator
			heat={heat}
			// Clamped to the lifetime in case the client clock runs behind the
			// server timestamp.
			remainingMs={
				expiresAtMs === undefined
					? undefined
					: Math.min(CACHE_IDLE_TTL_MS, expiresAtMs - nowMs)
			}
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
			onOpenChange={setIsOpen}
		/>
	);
};

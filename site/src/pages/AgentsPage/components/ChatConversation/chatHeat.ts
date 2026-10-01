import type * as TypesGen from "#/api/typesGenerated";
import { findContextBoundaryPart } from "./chatHelpers";

/**
 * Chat heat measures how much of the context window recent user turns sent
 * again instead of reading it from the prompt cache. Each request can reuse
 * the previous request's prompt; the part of that prompt it did not read
 * from the cache is its missed prefix. New tokens (tool output, the user's
 * message) are not misses. The first request after the chat starts or after
 * a compaction or clear boundary has no previous prompt, so it misses
 * nothing. A turn is every model request between two user messages, and its
 * sample is its summed missed prefix divided by a reference size: the usable
 * context (context limit times the compaction threshold), capped at
 * HEAT_REFERENCE_TOKENS so that re-sending a mid-size context in a large
 * window still reads high. Samples are weighted toward the newest turn and
 * shaped by a logistic curve into a displayed heat in [0, 1]. No prices are
 * involved.
 */
export type ChatHeat = {
	readonly heat: number;
	readonly label: ChatHeatLabel;
	readonly missRate: number;
	readonly lastTurnRequestCount: number;
	readonly lastTurnMissedTokens: number;
	readonly lastTurnReusableTokens: number;
	readonly lastTurnHasSegmentStart: boolean;
	readonly lastPromptTokens: number;
	readonly lastRequestAt: string;
};

type ChatHeatLabel = "low" | "moderate" | "high";

const HEAT_WINDOW_SIZE = 6;
const HEAT_WINDOW_DECAY = 0.25;
// Tuning constants: x is the weighted missed share of the reference size.
export const HEAT_REFERENCE_TOKENS = 70_000;
export const HEAT_CURVE_MIDPOINT = 0.3;
const HEAT_CURVE_STEEPNESS = 8;
// A single idle threshold for all providers. Anthropic's default ephemeral
// cache lives 5 minutes; automatic caches elsewhere are similar or longer.
export const CACHE_IDLE_TTL_MS = 5 * 60 * 1000;

type HeatRequest = {
	readonly promptTokens: number;
	readonly cacheReadTokens: number;
	readonly usesCache: boolean;
	readonly contextLimit: number;
	readonly createdAt: string;
};

const toTokenCount = (value: number | undefined): number =>
	value !== undefined && Number.isFinite(value) && value > 0 ? value : 0;

const toHeatRequest = (
	message: TypesGen.ChatMessage,
	activeContextLimit: number | undefined,
): HeatRequest | null => {
	const usage = message.usage;
	if (message.role !== "assistant" || !usage) {
		return null;
	}
	const contextLimit = usage.context_limit ?? activeContextLimit;
	if (
		contextLimit === undefined ||
		!Number.isFinite(contextLimit) ||
		contextLimit <= 0
	) {
		return null;
	}
	const cacheCreationTokens = toTokenCount(usage.cache_creation_tokens);
	const cacheReadTokens = toTokenCount(usage.cache_read_tokens);
	const promptTokens =
		toTokenCount(usage.input_tokens) + cacheCreationTokens + cacheReadTokens;
	if (promptTokens === 0) {
		return null;
	}
	return {
		promptTokens,
		cacheReadTokens,
		usesCache: cacheReadTokens > 0 || cacheCreationTokens > 0,
		contextLimit,
		createdAt: message.created_at,
	};
};

const logistic = (x: number): number =>
	1 / (1 + Math.exp(-HEAT_CURVE_STEEPNESS * (x - HEAT_CURVE_MIDPOINT)));

/** Maps a normalized missed share onto a displayed heat with heatCurve(0) = 0. */
export const heatCurve = (x: number): number => {
	if (!Number.isFinite(x) || x <= 0) {
		return 0;
	}
	const floor = logistic(0);
	return Math.min(1, (logistic(x) - floor) / (1 - floor));
};

export const getChatHeatLabel = (heat: number): ChatHeatLabel => {
	if (heat < 1 / 3) {
		return "low";
	}
	if (heat < 2 / 3) {
		return "moderate";
	}
	return "high";
};

type ScoredRequest = HeatRequest & {
	readonly reusableTokens: number;
	readonly missedTokens: number;
	readonly isSegmentStart: boolean;
};

// Returns turns newest first; requests within a turn are oldest first.
const scoreTurns = (
	messages: readonly TypesGen.ChatMessage[],
	activeContextLimit: number | undefined,
	historyComplete: boolean,
): ScoredRequest[][] => {
	const boundaryIndex = messages.findLastIndex((message) =>
		findContextBoundaryPart(message),
	);
	const reachedSegmentStart = historyComplete || boundaryIndex >= 0;
	const turns: ScoredRequest[][] = [];
	let current: ScoredRequest[] = [];
	let previousPromptTokens: number | undefined;
	for (const message of messages.slice(boundaryIndex + 1)) {
		if (message.role === "user") {
			if (current.length > 0) {
				turns.push(current);
				current = [];
			}
			continue;
		}
		const request = toHeatRequest(message, activeContextLimit);
		if (!request) {
			continue;
		}
		const reusableTokens = previousPromptTokens ?? 0;
		current.push({
			...request,
			reusableTokens,
			missedTokens: Math.max(0, reusableTokens - request.cacheReadTokens),
			isSegmentStart: reachedSegmentStart && previousPromptTokens === undefined,
		});
		previousPromptTokens = request.promptTokens;
	}
	if (current.length > 0) {
		turns.push(current);
	}
	// Without the segment start, the oldest loaded turn may be the tail of a
	// longer turn whose earlier requests are not loaded.
	if (!reachedSegmentStart && turns.length > 1) {
		turns.shift();
	}
	return turns.reverse();
};

const sumMissedTokens = (requests: readonly ScoredRequest[]): number =>
	requests.reduce((total, request) => total + request.missedTokens, 0);

// The largest prefix any request in the turn could have read from the cache.
const maxReusableTokens = (requests: readonly ScoredRequest[]): number =>
	Math.max(0, ...requests.map((request) => request.reusableTokens));

export const getChatHeat = (
	messages: readonly TypesGen.ChatMessage[],
	compressionThreshold: number | undefined,
	activeContextLimit?: number,
	// False when older messages are not loaded, so the oldest loaded turn
	// may not be the first of its segment.
	historyComplete = true,
): ChatHeat | null => {
	const turns = scoreTurns(messages, activeContextLimit, historyComplete).slice(
		0,
		HEAT_WINDOW_SIZE,
	);
	const latestTurn = turns[0];
	const latest = latestTurn?.at(-1);
	// A window with no cache reads or writes means the route does not use
	// prompt caching, so the meter would read high with nothing to act on.
	if (
		!latestTurn ||
		!latest ||
		!turns.some((turn) => turn.some((request) => request.usesCache))
	) {
		return null;
	}

	const thresholdPercent =
		compressionThreshold !== undefined &&
		Number.isFinite(compressionThreshold) &&
		compressionThreshold > 0
			? Math.min(compressionThreshold, 100)
			: 100;

	let weightTotal = 0;
	let weightedSample = 0;
	let missRateWeightTotal = 0;
	let weightedMissRate = 0;
	for (const [index, turn] of turns.entries()) {
		const missedTokens = sumMissedTokens(turn);
		const reusableTokens = maxReusableTokens(turn);
		const referenceTokens = Math.min(
			(turn.at(-1)?.contextLimit ?? 1) * (thresholdPercent / 100),
			HEAT_REFERENCE_TOKENS,
		);
		const weight = HEAT_WINDOW_DECAY ** index;
		weightTotal += weight;
		weightedSample += weight * (missedTokens / referenceTokens);
		if (reusableTokens > 0) {
			missRateWeightTotal += weight;
			weightedMissRate += weight * Math.min(1, missedTokens / reusableTokens);
		}
	}

	const heat = heatCurve(weightedSample / weightTotal);

	return {
		heat,
		label: getChatHeatLabel(heat),
		missRate:
			missRateWeightTotal > 0 ? weightedMissRate / missRateWeightTotal : 0,
		lastTurnRequestCount: latestTurn.length,
		lastTurnMissedTokens: sumMissedTokens(latestTurn),
		lastTurnReusableTokens: maxReusableTokens(latestTurn),
		lastTurnHasSegmentStart: latestTurn.some(
			(request) => request.isSegmentStart,
		),
		lastPromptTokens: latest.promptTokens,
		lastRequestAt: latest.createdAt,
	};
};

/**
 * Reports whether the provider cache has likely expired. Idle time runs from
 * the later of the last counted request (server clock) and the moment the
 * client last saw the chat stop generating, which covers turns that ended
 * without a counted request and limits the effect of clock skew.
 */
export const isCacheLikelyExpired = (
	lastRequestAt: string,
	lastStreamEndedAtMs: number | undefined,
	nowMs: number,
	isStreaming: boolean,
): boolean => {
	if (isStreaming) {
		return false;
	}
	const lastRequestMs = Date.parse(lastRequestAt);
	const lastActivityMs = Math.max(
		Number.isFinite(lastRequestMs) ? lastRequestMs : 0,
		lastStreamEndedAtMs ?? 0,
	);
	return lastActivityMs > 0 && nowMs - lastActivityMs > CACHE_IDLE_TTL_MS;
};

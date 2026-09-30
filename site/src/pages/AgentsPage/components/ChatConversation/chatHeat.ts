import type * as TypesGen from "#/api/typesGenerated";
import { findContextBoundaryPart } from "./chatHelpers";

/**
 * Chat heat measures how much of the context window recent user turns paid
 * for again instead of reading from the prompt cache. A turn is every model
 * request between two user messages. Its sample is the fresh prompt tokens
 * (uncached input plus cache writes) summed over its requests, divided by
 * the context limit. The first request after the chat starts or after a
 * compaction or clear boundary has no earlier prefix to reuse, so its fresh
 * tokens are not counted. Samples are weighted toward the newest turn,
 * normalized by the compaction threshold, and shaped by a logistic curve
 * into a displayed heat in [0, 1]. No prices are involved.
 */
export type ChatHeat = {
	readonly heat: number;
	readonly label: ChatHeatLabel;
	readonly missRate: number;
	readonly lastTurnRequestCount: number;
	readonly lastTurnFreshTokens: number;
	readonly lastTurnCacheReadTokens: number;
	readonly lastTurnIsFirst: boolean;
	readonly lastPromptTokens: number;
	readonly lastRequestAt: string;
};

type ChatHeatLabel = "cool" | "warm" | "hot";

const HEAT_WINDOW_SIZE = 6;
const HEAT_WINDOW_DECAY = 0.5;
// Tuning constants: x is the weighted fresh share of the context window
// relative to the compaction threshold.
export const HEAT_CURVE_MIDPOINT = 0.3;
const HEAT_CURVE_STEEPNESS = 12;
// A single idle threshold for all providers. Anthropic's default ephemeral
// cache lives 5 minutes; automatic caches elsewhere are similar or longer.
export const CACHE_IDLE_TTL_MS = 5 * 60 * 1000;

type HeatRequest = {
	readonly freshTokens: number;
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
	const freshTokens = toTokenCount(usage.input_tokens) + cacheCreationTokens;
	const cacheReadTokens = toTokenCount(usage.cache_read_tokens);
	if (freshTokens + cacheReadTokens === 0) {
		return null;
	}
	return {
		freshTokens,
		cacheReadTokens,
		usesCache: cacheReadTokens > 0 || cacheCreationTokens > 0,
		contextLimit,
		createdAt: message.created_at,
	};
};

const logistic = (x: number): number =>
	1 / (1 + Math.exp(-HEAT_CURVE_STEEPNESS * (x - HEAT_CURVE_MIDPOINT)));

/** Maps a normalized fresh share onto a displayed heat with heatCurve(0) = 0. */
export const heatCurve = (x: number): number => {
	if (!Number.isFinite(x) || x <= 0) {
		return 0;
	}
	const floor = logistic(0);
	return Math.min(1, (logistic(x) - floor) / (1 - floor));
};

export const getChatHeatLabel = (heat: number): ChatHeatLabel => {
	if (heat < 1 / 3) {
		return "cool";
	}
	if (heat < 2 / 3) {
		return "warm";
	}
	return "hot";
};

type HeatTurn = {
	// Newest first.
	readonly requests: HeatRequest[];
	isFirst: boolean;
};

const splitTurns = (
	messages: readonly TypesGen.ChatMessage[],
	activeContextLimit: number | undefined,
	historyComplete: boolean,
): HeatTurn[] => {
	const turns: HeatTurn[] = [];
	let current: HeatRequest[] = [];
	const closeTurn = () => {
		if (current.length > 0) {
			turns.push({ requests: current, isFirst: false });
			current = [];
		}
	};
	let reachedSegmentStart = historyComplete;
	for (const message of messages.toReversed()) {
		if (findContextBoundaryPart(message)) {
			reachedSegmentStart = true;
			break;
		}
		if (message.role === "user") {
			closeTurn();
			continue;
		}
		const request = toHeatRequest(message, activeContextLimit);
		if (request) {
			current.push(request);
		}
	}
	closeTurn();
	const oldest = turns.at(-1);
	if (oldest && reachedSegmentStart) {
		oldest.isFirst = true;
	}
	return turns;
};

export const getChatHeat = (
	messages: readonly TypesGen.ChatMessage[],
	compressionThreshold: number | undefined,
	activeContextLimit?: number,
	// False when older messages are not loaded, so the oldest loaded turn
	// may not be the first of its segment.
	historyComplete = true,
): ChatHeat | null => {
	const turns = splitTurns(messages, activeContextLimit, historyComplete).slice(
		0,
		HEAT_WINDOW_SIZE,
	);
	const latestTurn = turns[0];
	const latest = latestTurn?.requests[0];
	// A window with no cache reads or writes means the route does not use
	// prompt caching, so the meter would read hot with nothing to act on.
	if (
		!latestTurn ||
		!latest ||
		!turns.some((turn) => turn.requests.some((request) => request.usesCache))
	) {
		return null;
	}

	let weightTotal = 0;
	let weightedSample = 0;
	let weightedMissRate = 0;
	for (const [index, turn] of turns.entries()) {
		let freshTokens = 0;
		let promptTokens = 0;
		for (const [requestIndex, request] of turn.requests.entries()) {
			const isSegmentStart =
				turn.isFirst && requestIndex === turn.requests.length - 1;
			if (!isSegmentStart) {
				freshTokens += request.freshTokens;
			}
			promptTokens += request.freshTokens + request.cacheReadTokens;
		}
		const contextLimit = turn.requests[0]?.contextLimit ?? 1;
		const weight = HEAT_WINDOW_DECAY ** index;
		weightTotal += weight;
		weightedSample += weight * (freshTokens / contextLimit);
		weightedMissRate += weight * (freshTokens / promptTokens);
	}

	const threshold =
		compressionThreshold !== undefined &&
		Number.isFinite(compressionThreshold) &&
		compressionThreshold > 0
			? Math.min(compressionThreshold, 100)
			: 100;
	const heat = heatCurve(weightedSample / weightTotal / (threshold / 100));

	return {
		heat,
		label: getChatHeatLabel(heat),
		missRate: weightedMissRate / weightTotal,
		lastTurnRequestCount: latestTurn.requests.length,
		lastTurnFreshTokens: latestTurn.requests.reduce(
			(total, request) => total + request.freshTokens,
			0,
		),
		lastTurnCacheReadTokens: latestTurn.requests.reduce(
			(total, request) => total + request.cacheReadTokens,
			0,
		),
		lastTurnIsFirst: latestTurn.isFirst,
		lastPromptTokens: latest.freshTokens + latest.cacheReadTokens,
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

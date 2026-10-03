import type * as TypesGen from "#/api/typesGenerated";
import { findContextBoundaryPart } from "./chatHelpers";

/**
 * Chat heat looks forward: it estimates what the user's next message will
 * cost in prompt-cache terms. Each request can reuse the previous request's
 * prompt from the cache. While the cache is warm the next message reads that
 * prompt at a fraction of the write price; once it expires, or when a
 * different model is selected, the whole prompt is written again. The
 * reading is that prompt size, scaled by CACHE_READ_TO_WRITE_RATIO while
 * warm, mapped onto a fixed 150K-token scale by a logistic curve. No prices
 * are involved; the ratio is a list-price multiplier, not a currency amount.
 */
export type ChatHeat = {
	// Prompt tokens (input, cache writes and cache reads) of the latest
	// counted request; 0 when a compaction or clear followed it.
	readonly lastPromptTokens: number;
	readonly lastRequestAt: string;
	readonly lastModelConfigId: string | undefined;
	// Set when a compaction or clear boundary follows the latest counted
	// request, so that request no longer describes the next prompt.
	readonly boundary: ChatContextBoundary | undefined;
	// Undefined when no turn since the segment start could be scored.
	readonly lastTurn: ChatHeatTurn | undefined;
};

type ChatContextBoundary = "compacted" | "cleared";

export type ChatHeatTurn = {
	readonly requestCount: number;
	// Cacheable tokens the turn's requests were billed for without a cache
	// read, summed over the turn.
	readonly missedTokens: number;
	// The largest prefix any request in the turn could have read from the
	// cache; 0 for a segment's first request.
	readonly reusableTokens: number;
	// True when older messages are not loaded and this is the only loaded
	// turn, so its first loaded request has no known previous prompt.
	readonly isPartial: boolean;
};

type ChatHeatLabel = "low" | "moderate" | "high";

export type NextMessageProjection = {
	// Tokens the next message is expected to cost in cache-write terms.
	readonly tokens: number;
	readonly heat: number;
	readonly label: ChatHeatLabel;
};

// Anthropic list multipliers: a cache read bills at 0.1x the base input
// price and a 5-minute cache write at 1.25x, so a warm read costs 0.08 of
// the re-write it avoids. One guess for every model.
export const CACHE_READ_TO_WRITE_RATIO = 0.1 / 1.25;
// Tuning constants: x is the projected tokens over the reference size.
export const HEAT_REFERENCE_TOKENS = 150_000;
export const HEAT_CURVE_MIDPOINT = 0.5;
const HEAT_CURVE_STEEPNESS = 4;
// A single idle threshold for all providers. Anthropic's default ephemeral
// cache lives 5 minutes; automatic caches elsewhere are similar or longer.
export const CACHE_IDLE_TTL_MS = 5 * 60 * 1000;

type HeatRequest = {
	readonly promptTokens: number;
	readonly cacheReadTokens: number;
	// Input and cache-write tokens: the part of the prompt billed at or above
	// the base price.
	readonly uncachedTokens: number;
	readonly usesCache: boolean;
	readonly createdAt: string;
	readonly modelConfigId: string | undefined;
};

const toTokenCount = (value: number | undefined): number =>
	value !== undefined && Number.isFinite(value) && value > 0 ? value : 0;

const toHeatRequest = (message: TypesGen.ChatMessage): HeatRequest | null => {
	const usage = message.usage;
	if (message.role !== "assistant" || !usage) {
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
		uncachedTokens: promptTokens - cacheReadTokens,
		usesCache: cacheReadTokens > 0 || cacheCreationTokens > 0,
		createdAt: message.created_at,
		modelConfigId: message.model_config_id,
	};
};

const logistic = (x: number): number =>
	1 / (1 + Math.exp(-HEAT_CURVE_STEEPNESS * (x - HEAT_CURVE_MIDPOINT)));

/** Maps projected tokens over the reference size onto [0, 1] with heatCurve(0) = 0. */
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

/**
 * Projects the next message from the latest prompt. Cold means the cache
 * cannot be read: it has expired or the selected model differs.
 */
export const projectNextMessage = (
	lastPromptTokens: number,
	isCold: boolean,
): NextMessageProjection => {
	const tokens = isCold
		? lastPromptTokens
		: lastPromptTokens * CACHE_READ_TO_WRITE_RATIO;
	const heat = heatCurve(tokens / HEAT_REFERENCE_TOKENS);
	return { tokens, heat, label: getChatHeatLabel(heat) };
};

type ScoredRequest = HeatRequest & {
	readonly reusableTokens: number;
	readonly missedTokens: number;
};

type ScoredSegment = {
	// Newest first; requests within a turn are oldest first.
	readonly turns: readonly (readonly ScoredRequest[])[];
	readonly latestIsPartial: boolean;
	readonly boundary: ChatContextBoundary | undefined;
};

const toBoundary = (
	message: TypesGen.ChatMessage,
): ChatContextBoundary | undefined => {
	const part = findContextBoundaryPart(message);
	if (!part || (part.type !== "tool-call" && part.type !== "tool-result")) {
		return undefined;
	}
	return part.tool_name === "chat_cleared" ? "cleared" : "compacted";
};

const scoreSegment = (
	messages: readonly TypesGen.ChatMessage[],
	historyComplete: boolean,
): ScoredSegment => {
	const boundaryIndex = messages.findLastIndex((message) =>
		findContextBoundaryPart(message),
	);
	const boundaryMessage = messages[boundaryIndex];
	const reachedSegmentStart = historyComplete || boundaryIndex >= 0;
	const turns: ScoredRequest[][] = [];
	let current: ScoredRequest[] = [];
	let previousPromptTokens: number | undefined;
	let oldestTurnHasUserMessage = false;
	for (const message of messages.slice(boundaryIndex + 1)) {
		if (message.role === "user") {
			if (current.length > 0) {
				turns.push(current);
				current = [];
			} else if (turns.length === 0) {
				oldestTurnHasUserMessage = true;
			}
			continue;
		}
		const request = toHeatRequest(message);
		if (!request) {
			continue;
		}
		const reusableTokens = previousPromptTokens ?? 0;
		current.push({
			...request,
			reusableTokens,
			// A prompt shorter than the previous one misses the tail of that
			// prefix without paying for it, so the miss is capped at the tokens
			// the request was actually billed uncached.
			missedTokens: Math.min(
				Math.max(0, reusableTokens - request.cacheReadTokens),
				request.uncachedTokens,
			),
		});
		previousPromptTokens = request.promptTokens;
	}
	if (current.length > 0) {
		turns.push(current);
	}
	// Without the segment start, the oldest loaded turn may be the tail of a
	// longer turn whose earlier requests are not loaded, and its first loaded
	// request cannot be scored because its previous prompt is unknown. A turn
	// whose user message is loaded is complete.
	const latestIsPartial =
		!reachedSegmentStart && turns.length === 1 && !oldestTurnHasUserMessage;
	if (!reachedSegmentStart && turns.length > 1) {
		turns.shift();
	}
	return {
		turns: turns.reverse(),
		latestIsPartial,
		boundary: boundaryMessage ? toBoundary(boundaryMessage) : undefined,
	};
};

const sumMissedTokens = (requests: readonly ScoredRequest[]): number =>
	requests.reduce((total, request) => total + request.missedTokens, 0);

const maxReusableTokens = (requests: readonly ScoredRequest[]): number =>
	Math.max(0, ...requests.map((request) => request.reusableTokens));

export const getChatHeat = (
	messages: readonly TypesGen.ChatMessage[],
	// False when older messages are not loaded, so the oldest loaded turn
	// may not be the first of its segment.
	historyComplete = true,
): ChatHeat | null => {
	const segment = scoreSegment(messages, historyComplete);
	const latestTurn = segment.turns[0];
	const latest = latestTurn?.at(-1);
	if (!latestTurn || !latest) {
		// A boundary with nothing counted after it: the next message starts
		// a fresh cache whose size is not known yet.
		if (segment.boundary) {
			return {
				lastPromptTokens: 0,
				lastRequestAt: "",
				lastModelConfigId: undefined,
				boundary: segment.boundary,
				lastTurn: undefined,
			};
		}
		return null;
	}
	// A segment with no cache reads or writes means the route does not use
	// prompt caching, so there is nothing to project.
	if (!segment.turns.some((turn) => turn.some((r) => r.usesCache))) {
		return null;
	}
	return {
		lastPromptTokens: latest.promptTokens,
		lastRequestAt: latest.createdAt,
		lastModelConfigId: latest.modelConfigId,
		boundary: undefined,
		lastTurn: {
			requestCount: latestTurn.length,
			missedTokens: sumMissedTokens(latestTurn),
			reusableTokens: maxReusableTokens(latestTurn),
			isPartial: segment.latestIsPartial,
		},
	};
};

/**
 * When the provider cache is expected to expire. Idle time runs from the
 * later of the last counted request (server clock) and the moment the client
 * last saw the chat stop generating, which covers turns that ended without a
 * counted request and limits the effect of clock skew. Undefined when
 * neither is known.
 */
export const getCacheExpiresAtMs = (
	lastRequestAt: string,
	lastStreamEndedAtMs: number | undefined,
): number | undefined => {
	const lastRequestMs = Date.parse(lastRequestAt);
	const lastActivityMs = Math.max(
		Number.isFinite(lastRequestMs) ? lastRequestMs : 0,
		lastStreamEndedAtMs ?? 0,
	);
	return lastActivityMs > 0 ? lastActivityMs + CACHE_IDLE_TTL_MS : undefined;
};

/** Whole minutes left, rounded up, so the last minute reads 1 and never 0. */
export const getRemainingMinutes = (remainingMs: number): number =>
	Math.min(
		Math.ceil(CACHE_IDLE_TTL_MS / 60_000),
		Math.max(1, Math.ceil(remainingMs / 60_000)),
	);

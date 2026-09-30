import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatCompactionMessage,
	MockChatMessage,
} from "#/testHelpers/chatEntities";
import {
	CACHE_IDLE_TTL_MS,
	getChatHeat,
	getChatHeatLabel,
	HEAT_CURVE_MIDPOINT,
	heatCurve,
	isCacheLikelyExpired,
} from "./chatHeat";

const CONTEXT_LIMIT = 100_000;

let nextId = 100;
const request = (
	usage: TypesGen.ChatMessageUsage,
	createdAt = "2026-01-01T00:00:00Z",
): TypesGen.ChatMessage => ({
	...MockChatMessage,
	id: nextId++,
	role: "assistant",
	created_at: createdAt,
	usage: { context_limit: CONTEXT_LIMIT, ...usage },
});

// A request whose fresh share of the context window equals `share`.
const requestWithShare = (share: number): TypesGen.ChatMessage =>
	request({
		input_tokens: 0,
		cache_creation_tokens: share * CONTEXT_LIMIT,
		cache_read_tokens: 1_000,
	});

const userMessage = (): TypesGen.ChatMessage => ({
	...MockChatMessage,
	id: nextId++,
	role: "user",
});

// One user turn with the given requests, oldest first.
const turn = (...requests: TypesGen.ChatMessage[]) => [
	userMessage(),
	...requests,
];

describe("heatCurve", () => {
	it("maps zero and negative input to zero", () => {
		expect(heatCurve(0)).toBe(0);
		expect(heatCurve(-1)).toBe(0);
		expect(heatCurve(Number.NaN)).toBe(0);
	});

	it("is monotonic", () => {
		let previous = heatCurve(0);
		for (let x = 0.02; x <= 1.5; x += 0.02) {
			const value = heatCurve(x);
			expect(value).toBeGreaterThan(previous);
			previous = value;
		}
	});

	it("passes about halfway at the midpoint", () => {
		expect(heatCurve(HEAT_CURVE_MIDPOINT)).toBeCloseTo(0.49, 2);
	});

	it("saturates before the compaction threshold", () => {
		expect(heatCurve(0.7)).toBeGreaterThan(0.99);
		expect(heatCurve(10)).toBeLessThanOrEqual(1);
	});

	it("stays cool for small fresh shares", () => {
		expect(heatCurve(0.05)).toBeLessThan(0.05);
	});
});

describe("getChatHeatLabel", () => {
	it("derives labels from displayed heat", () => {
		expect(getChatHeatLabel(0)).toBe("cool");
		expect(getChatHeatLabel(0.5)).toBe("warm");
		expect(getChatHeatLabel(0.9)).toBe("hot");
	});
});

describe("getChatHeat", () => {
	// Most cases load a partial history so the oldest turn is not treated as
	// the first of its segment.
	const partial = (
		messages: TypesGen.ChatMessage[],
		threshold: number | undefined = 100,
	) => getChatHeat(messages, threshold, undefined, false);

	it("returns null without usable requests", () => {
		expect(getChatHeat([], 70)).toBeNull();
		expect(getChatHeat([MockChatMessage], 70)).toBeNull();
		expect(
			getChatHeat(
				[{ ...request({ input_tokens: 10 }), usage: { input_tokens: 10 } }],
				70,
			),
		).toBeNull();
	});

	it("uses the active context limit when the message has none", () => {
		const message: TypesGen.ChatMessage = {
			...request({}),
			usage: { cache_creation_tokens: 21_000 },
		};
		expect(
			getChatHeat(turn(message), 70, CONTEXT_LIMIT, false)?.heat,
		).toBeCloseTo(heatCurve(0.3));
	});

	it("sums fresh tokens over the requests of a turn", () => {
		const heat = partial(
			turn(
				request({ input_tokens: 1_000, cache_creation_tokens: 20_000 }),
				request({ input_tokens: 2_000, cache_read_tokens: 21_000 }),
				request({ input_tokens: 3_000, cache_read_tokens: 23_000 }),
			),
			70,
		);
		expect(heat?.heat).toBeCloseTo(heatCurve(0.26 / 0.7));
		expect(heat?.missRate).toBeCloseTo(26_000 / 70_000);
		expect(heat?.lastTurnRequestCount).toBe(3);
		expect(heat?.lastTurnFreshTokens).toBe(26_000);
		expect(heat?.lastTurnCacheReadTokens).toBe(44_000);
		expect(heat?.lastPromptTokens).toBe(26_000);
	});

	it("keeps a cold first request hot despite cached tool steps", () => {
		const coldTurn = turn(
			request({ cache_creation_tokens: 40_000 }),
			request({ input_tokens: 500, cache_read_tokens: 40_000 }),
			request({ input_tokens: 500, cache_read_tokens: 40_500 }),
			request({ input_tokens: 500, cache_read_tokens: 41_000 }),
		);
		expect(partial(coldTurn, 70)?.label).toBe("hot");
	});

	it("weights the newest turn most", () => {
		const coldThenWarm = partial([
			...turn(requestWithShare(0.4)),
			...turn(requestWithShare(0)),
		]);
		const warmThenCold = partial([
			...turn(requestWithShare(0)),
			...turn(requestWithShare(0.4)),
		]);
		// Weights 0.5 and 0.25 normalize to 2/3 and 1/3.
		expect(coldThenWarm?.heat).toBeCloseTo(heatCurve(0.4 / 3));
		expect(warmThenCold?.heat).toBeCloseTo(heatCurve((0.4 * 2) / 3));
	});

	it("ignores turns beyond the newest six", () => {
		const recent = Array.from({ length: 6 }, () =>
			turn(requestWithShare(0.1)),
		).flat();
		expect(
			partial([...turn(requestWithShare(0.9)), ...recent])?.heat,
		).toBeCloseTo(heatCurve(0.1));
	});

	it("does not count the first request of the chat", () => {
		const heat = getChatHeat(
			turn(
				request({ input_tokens: 500, cache_creation_tokens: 80_000 }),
				request({ input_tokens: 1_000, cache_read_tokens: 80_500 }),
			),
			100,
		);
		expect(heat?.heat).toBeCloseTo(heatCurve(0.01));
		expect(heat?.lastTurnIsFirst).toBe(true);
		expect(heat?.lastTurnFreshTokens).toBe(81_500);
	});

	it("counts the oldest loaded turn when older history is unloaded", () => {
		const messages = turn(request({ cache_creation_tokens: 80_000 }));
		expect(getChatHeat(messages, 100)?.heat).toBe(0);
		expect(partial(messages)?.heat).toBeCloseTo(heatCurve(0.8));
		expect(partial(messages)?.lastTurnIsFirst).toBe(false);
	});

	it("restarts the window at a compaction or clear boundary", () => {
		const cleared: TypesGen.ChatMessage = {
			...MockChatMessage,
			id: nextId++,
			role: "tool",
			content: [{ type: "tool-call", tool_name: "chat_cleared" }],
		};
		for (const boundary of [MockChatCompactionMessage, cleared]) {
			const heat = partial([
				...turn(requestWithShare(0.9)),
				boundary,
				...turn(requestWithShare(0.5)),
				...turn(requestWithShare(0.1)),
			]);
			// The turn after the boundary is its segment's first, so its only
			// request is not counted.
			expect(heat?.heat).toBeCloseTo(heatCurve((0.1 * 2) / 3));
			expect(partial([...turn(requestWithShare(0.9)), boundary])).toBeNull();
		}
	});

	it("returns null when no request in the window uses the prompt cache", () => {
		const uncached = () => request({ input_tokens: 50_000 });
		expect(partial([...turn(uncached()), ...turn(uncached())])).toBeNull();
		expect(
			partial([...turn(requestWithShare(0.1)), ...turn(uncached())])?.heat,
		).toBeGreaterThan(0);
	});

	it("normalizes by the compaction threshold", () => {
		const messages = turn(requestWithShare(0.15));
		expect(partial(messages, 50)?.heat).toBeCloseTo(heatCurve(0.3));
		expect(partial(messages, undefined)?.heat).toBeCloseTo(heatCurve(0.15));
		expect(partial(messages, 0)?.heat).toBeCloseTo(heatCurve(0.15));
	});

	it("reports the newest request's timestamp", () => {
		const heat = partial(
			turn(
				request({ cache_read_tokens: 1 }, "2026-01-01T00:00:00Z"),
				request({ cache_read_tokens: 1 }, "2026-01-01T00:10:00Z"),
			),
		);
		expect(heat?.lastRequestAt).toBe("2026-01-01T00:10:00Z");
	});
});

describe("isCacheLikelyExpired", () => {
	const last = "2026-01-01T00:00:00Z";
	const lastMs = Date.parse(last);

	it("expires after the idle lifetime", () => {
		expect(
			isCacheLikelyExpired(last, undefined, lastMs + CACHE_IDLE_TTL_MS, false),
		).toBe(false);
		expect(
			isCacheLikelyExpired(
				last,
				undefined,
				lastMs + CACHE_IDLE_TTL_MS + 1,
				false,
			),
		).toBe(true);
	});

	it("measures idle time from the later of request and stream end", () => {
		const streamEndedMs = lastMs + 10 * 60 * 1000;
		expect(
			isCacheLikelyExpired(last, streamEndedMs, streamEndedMs + 1_000, false),
		).toBe(false);
		expect(
			isCacheLikelyExpired(
				last,
				streamEndedMs,
				streamEndedMs + CACHE_IDLE_TTL_MS + 1,
				false,
			),
		).toBe(true);
	});

	it("treats a request timestamp ahead of the client clock as recent", () => {
		expect(isCacheLikelyExpired(last, undefined, lastMs - 60_000, false)).toBe(
			false,
		);
	});

	it("never expires while streaming", () => {
		expect(
			isCacheLikelyExpired(last, undefined, lastMs + 60 * 60 * 1000, true),
		).toBe(false);
	});
});

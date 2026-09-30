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
	HEAT_REFERENCE_TOKENS,
	heatCurve,
	isCacheLikelyExpired,
} from "./chatHeat";

const CONTEXT_LIMIT = 50_000;

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

// Builds consecutive requests. Each one reads the previous prompt from the
// cache except for `missed` tokens, and adds `added` new tokens.
const requestChain = (
	initialPromptTokens = 0,
	contextLimit = CONTEXT_LIMIT,
) => {
	let promptTokens = initialPromptTokens;
	return (missed = 0, added = 100): TypesGen.ChatMessage => {
		const cacheRead = Math.max(0, promptTokens - missed);
		const cacheCreation = promptTokens - cacheRead + added;
		promptTokens = cacheRead + cacheCreation;
		return request({
			cache_read_tokens: cacheRead,
			cache_creation_tokens: cacheCreation,
			context_limit: contextLimit,
		});
	};
};

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
	// Most cases load a partial history, so the oldest loaded turn only
	// seeds the previous prompt and is not scored.
	const partial = (
		messages: TypesGen.ChatMessage[],
		threshold: number | undefined = 100,
	) => getChatHeat(messages, threshold, undefined, false);

	// A seed turn followed by one turn per missed share of the context limit.
	const turnsMissing = (...shares: number[]) => {
		const next = requestChain(40_000);
		return [
			...turn(next()),
			...shares.flatMap((share) => turn(next(share * CONTEXT_LIMIT))),
		];
	};

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
		const withoutLimit = (usage: TypesGen.ChatMessageUsage) => ({
			...request({}),
			usage,
		});
		const messages = [
			...turn(withoutLimit({ cache_creation_tokens: 20_000 })),
			...turn(
				withoutLimit({
					cache_read_tokens: 5_000,
					cache_creation_tokens: 15_000,
				}),
			),
		];
		expect(getChatHeat(messages, 100, CONTEXT_LIMIT, false)?.heat).toBeCloseTo(
			heatCurve(15_000 / CONTEXT_LIMIT),
		);
	});

	it("sums the missed prefix over the requests of a turn", () => {
		const heat = getChatHeat(
			[
				...turn(request({ cache_creation_tokens: 20_000 })),
				...turn(
					request({ cache_read_tokens: 5_000, cache_creation_tokens: 16_000 }),
					request({ cache_read_tokens: 21_000, input_tokens: 2_000 }),
					request({ cache_read_tokens: 20_000, input_tokens: 4_000 }),
				),
			],
			100,
		);
		// The newest turn misses 15K + 0 + 3K and weighs 2/3; the first turn
		// misses nothing.
		expect(heat?.heat).toBeCloseTo(
			heatCurve(((18_000 / CONTEXT_LIMIT) * 2) / 3),
		);
		expect(heat?.missRate).toBeCloseTo(18_000 / 23_000);
		expect(heat?.lastTurnRequestCount).toBe(3);
		expect(heat?.lastTurnMissedTokens).toBe(18_000);
		expect(heat?.lastTurnReusableTokens).toBe(23_000);
		expect(heat?.lastTurnHasSegmentStart).toBe(false);
		expect(heat?.lastPromptTokens).toBe(24_000);
	});

	it("does not count new tokens in a warm tool loop as misses", () => {
		const next = requestChain(10_000);
		const seed = turn(next());
		const loop = turn(...Array.from({ length: 20 }, () => next(0, 3_000)));
		const heat = partial([...seed, ...loop]);
		expect(heat?.heat).toBe(0);
		expect(heat?.missRate).toBe(0);
	});

	it("keeps a cold first request hot despite cached tool steps", () => {
		const next = requestChain(40_000);
		const seed = turn(next());
		const coldTurn = turn(
			next(Number.POSITIVE_INFINITY),
			next(),
			next(),
			next(),
		);
		expect(partial([...seed, ...coldTurn], 70)?.label).toBe("hot");
	});

	it("weights the newest turn most", () => {
		// Weights 0.5 and 0.25 normalize to 2/3 and 1/3.
		expect(partial(turnsMissing(0.4, 0))?.heat).toBeCloseTo(heatCurve(0.4 / 3));
		expect(partial(turnsMissing(0, 0.4))?.heat).toBeCloseTo(
			heatCurve((0.4 * 2) / 3),
		);
	});

	it("ignores turns beyond the newest six", () => {
		expect(
			partial(turnsMissing(0.8, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1))?.heat,
		).toBeCloseTo(heatCurve(0.1));
	});

	it("does not count the first request of the chat", () => {
		const next = requestChain();
		const heat = getChatHeat(turn(next(0, 40_000), next(0, 1_000)), 100);
		expect(heat?.heat).toBe(0);
		expect(heat?.lastTurnHasSegmentStart).toBe(true);
		expect(heat?.lastTurnReusableTokens).toBe(40_000);
	});

	it("drops the oldest loaded turn when older history is unloaded", () => {
		const messages = turnsMissing(0.8);
		expect(partial(messages)?.heat).toBeCloseTo(heatCurve(0.8));
		// With the full history, the seed turn is the segment's first.
		expect(getChatHeat(messages, 100)?.heat).toBeCloseTo(
			heatCurve((0.8 * 2) / 3),
		);
	});

	it("scores only known misses when a single partial turn is loaded", () => {
		const next = requestChain(40_000);
		const heat = partial(turn(next(Number.POSITIVE_INFINITY), next(5_000)));
		expect(heat?.heat).toBeCloseTo(heatCurve(5_000 / CONTEXT_LIMIT));
		expect(heat?.lastTurnHasSegmentStart).toBe(false);
	});

	it("restarts the window at a compaction or clear boundary", () => {
		const cleared: TypesGen.ChatMessage = {
			...MockChatMessage,
			id: nextId++,
			role: "tool",
			content: [{ type: "tool-call", tool_name: "chat_cleared" }],
		};
		for (const boundary of [MockChatCompactionMessage, cleared]) {
			const next = requestChain(10_000);
			const heat = partial([
				...turnsMissing(0.9),
				boundary,
				...turn(next(Number.POSITIVE_INFINITY)),
				...turn(next(0.1 * CONTEXT_LIMIT)),
			]);
			// The request after the boundary starts its segment and misses
			// nothing.
			expect(heat?.heat).toBeCloseTo(heatCurve((0.1 * 2) / 3));
			expect(partial([...turnsMissing(0.9), boundary])).toBeNull();
		}
	});

	it("returns null when no request in the window uses the prompt cache", () => {
		const uncached = () => request({ input_tokens: 20_000 });
		expect(
			partial([...turn(uncached()), ...turn(uncached()), ...turn(uncached())]),
		).toBeNull();
		expect(
			partial([...turnsMissing(0.1), ...turn(uncached())])?.heat,
		).toBeGreaterThan(0);
	});

	it("normalizes by the compaction threshold", () => {
		const messages = turnsMissing(0.15);
		expect(partial(messages, 50)?.heat).toBeCloseTo(heatCurve(0.3));
		expect(partial(messages, undefined)?.heat).toBeCloseTo(heatCurve(0.15));
		expect(partial(messages, 0)?.heat).toBeCloseTo(heatCurve(0.15));
	});

	it("caps the reference size for large context windows", () => {
		const next = requestChain(40_000, 1_000_000);
		const messages = [...turn(next()), ...turn(next(21_000))];
		expect(partial(messages, 30)?.heat).toBeCloseTo(
			heatCurve(21_000 / HEAT_REFERENCE_TOKENS),
		);
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

// Chats observed during user testing, on a 200K window with the default
// compaction threshold of 70%.
describe("getChatHeat scenarios", () => {
	const heatOf = (messages: TypesGen.ChatMessage[]) =>
		getChatHeat(messages, 70)?.label;
	const toolSteps = (next: ReturnType<typeof requestChain>, count: number) =>
		Array.from({ length: count }, () => next(0, 1_000));

	it("reads hot when each slow turn re-writes an 83K context", () => {
		const next = requestChain(0, 200_000);
		const messages = [...turn(next(0, 80_000), ...toolSteps(next, 3))];
		for (let i = 0; i < 5; i++) {
			messages.push(
				...turn(next(Number.POSITIVE_INFINITY, 500), ...toolSteps(next, 3)),
			);
		}
		expect(heatOf(messages)).toBe("hot");
		// The miss rate agrees with the flame despite the cached tool steps.
		expect(getChatHeat(messages, 70)?.missRate).toBeGreaterThan(0.95);
	});

	it("reads cool for a warm 30-step tool loop", () => {
		const next = requestChain(0, 200_000);
		const messages = [...turn(next(0, 20_000))];
		messages.push(...turn(...Array.from({ length: 30 }, () => next(0, 3_000))));
		expect(heatOf(messages)).toBe("cool");
	});

	it("reads cool on the first turn of a chat", () => {
		const next = requestChain(0, 200_000);
		expect(heatOf(turn(next(0, 83_000), ...toolSteps(next, 3)))).toBe("cool");
	});

	it("reads hot when slow turns miss most of a 46K context", () => {
		const next = requestChain(0, 200_000);
		const messages = [...turn(next(0, 46_000), ...toolSteps(next, 2))];
		for (let i = 0; i < 5; i++) {
			messages.push(...turn(next(39_000, 500), ...toolSteps(next, 2)));
		}
		expect(heatOf(messages)).toBe("hot");
	});

	it("reads cool for a rapid back-and-forth", () => {
		const next = requestChain(0, 200_000);
		const messages = [...turn(next(0, 20_000))];
		for (let i = 0; i < 10; i++) {
			messages.push(...turn(next(0, 2_000)));
		}
		expect(heatOf(messages)).toBe("cool");
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

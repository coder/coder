import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatCompactionMessage,
	MockChatMessage,
} from "#/testHelpers/chatEntities";
import {
	CACHE_IDLE_TTL_MS,
	CACHE_READ_TO_WRITE_RATIO,
	getCacheExpiresAtMs,
	getChatHeat,
	getChatHeatLabel,
	getRemainingMinutes,
	HEAT_CURVE_MIDPOINT,
	HEAT_REFERENCE_TOKENS,
	heatCurve,
	projectNextMessage,
} from "./chatHeat";

let nextId = 100;
const request = (
	usage: TypesGen.ChatMessageUsage,
	createdAt = "2026-01-01T00:00:00Z",
): TypesGen.ChatMessage => ({
	...MockChatMessage,
	id: nextId++,
	role: "assistant",
	created_at: createdAt,
	usage,
});

// Builds consecutive requests. Each one reads the previous prompt from the
// cache except for `missed` tokens, and adds `added` new tokens.
const requestChain = (initialPromptTokens = 0) => {
	let promptTokens = initialPromptTokens;
	return (missed = 0, added = 100): TypesGen.ChatMessage => {
		const cacheRead = Math.max(0, promptTokens - missed);
		const cacheCreation = promptTokens - cacheRead + added;
		promptTokens = cacheRead + cacheCreation;
		return request({
			cache_read_tokens: cacheRead,
			cache_creation_tokens: cacheCreation,
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

const clearedMessage: TypesGen.ChatMessage = {
	...MockChatMessage,
	id: nextId++,
	role: "tool",
	content: [{ type: "tool-call", tool_name: "chat_cleared" }],
};

describe("heatCurve", () => {
	it("maps zero and negative input to zero", () => {
		expect(heatCurve(0)).toBe(0);
		expect(heatCurve(-1)).toBe(0);
		expect(heatCurve(Number.NaN)).toBe(0);
	});

	it("is monotonic", () => {
		let previous = 0;
		for (let x = 0.05; x <= 3; x += 0.05) {
			const value = heatCurve(x);
			expect(value).toBeGreaterThanOrEqual(previous);
			previous = value;
		}
	});

	it("passes about halfway at the midpoint", () => {
		expect(heatCurve(HEAT_CURVE_MIDPOINT)).toBeCloseTo(0.43, 2);
	});

	it("matches the agreed sample points on the 150K scale", () => {
		const at = (tokens: number) =>
			Math.round(heatCurve(tokens / HEAT_REFERENCE_TOKENS) * 100) / 100;
		expect(HEAT_REFERENCE_TOKENS).toBe(150_000);
		expect(at(25_000)).toBe(0.1);
		expect(at(50_000)).toBe(0.25);
		expect(at(75_000)).toBe(0.43);
		expect(at(100_000)).toBe(0.61);
		expect(at(150_000)).toBe(0.86);
		expect(at(200_000)).toBe(0.96);
		expect(at(300_000)).toBe(1);
	});
});

describe("getChatHeatLabel", () => {
	it("derives labels from displayed heat", () => {
		expect(getChatHeatLabel(0)).toBe("low");
		expect(getChatHeatLabel(0.4)).toBe("moderate");
		expect(getChatHeatLabel(0.9)).toBe("high");
	});
});

describe("projectNextMessage", () => {
	it("re-writes the whole prompt when cold", () => {
		const cold = projectNextMessage(165_000, true);
		expect(cold.tokens).toBe(165_000);
		expect(cold.heat).toBeCloseTo(0.91, 2);
		expect(cold.label).toBe("high");
	});

	it("scales the prompt by the read to write ratio when warm", () => {
		expect(CACHE_READ_TO_WRITE_RATIO).toBeCloseTo(0.08);
		const warm = projectNextMessage(165_000, false);
		expect(warm.tokens).toBeCloseTo(13_200);
		expect(warm.heat).toBeCloseTo(0.048, 2);
		expect(warm.label).toBe("low");
		expect(projectNextMessage(1_000_000, false).label).toBe("moderate");
	});

	it("places the band edges near 62K and 108K", () => {
		expect(projectNextMessage(61_000, true).label).toBe("low");
		expect(projectNextMessage(63_000, true).label).toBe("moderate");
		expect(projectNextMessage(107_000, true).label).toBe("moderate");
		expect(projectNextMessage(109_000, true).label).toBe("high");
	});
});

describe("getChatHeat", () => {
	it("returns null without usable requests", () => {
		expect(getChatHeat([])).toBeNull();
		expect(getChatHeat([MockChatMessage])).toBeNull();
		expect(getChatHeat([request({ output_tokens: 10 })])).toBeNull();
	});

	it("returns null when no request in the segment uses the prompt cache", () => {
		const uncached = () => request({ input_tokens: 20_000 });
		expect(getChatHeat([...turn(uncached()), ...turn(uncached())])).toBeNull();
	});

	it("reports the latest request's prompt, time and model", () => {
		const next = requestChain(40_000);
		const messages = [
			...turn(next()),
			...turn(next(0), {
				...next(0, 500),
				created_at: "2026-01-01T00:05:00Z",
				model_config_id: "model-b",
			}),
		];
		const heat = getChatHeat(messages);
		expect(heat?.lastPromptTokens).toBe(40_000 + 100 + 100 + 500);
		expect(heat?.lastRequestAt).toBe("2026-01-01T00:05:00Z");
		expect(heat?.lastModelConfigId).toBe("model-b");
		expect(heat?.boundary).toBeUndefined();
	});

	it("sums the missed prefix over the requests of a turn", () => {
		const next = requestChain(10_000);
		const heat = getChatHeat([
			...turn(next()),
			...turn(next(4_000), next(0), next(2_000)),
		]);
		expect(heat?.lastTurn).toMatchObject({
			requestCount: 3,
			missedTokens: 6_000,
			reusableTokens: 10_300,
			isPartial: false,
		});
	});

	it("caps a request's miss at the tokens it was billed uncached", () => {
		const next = requestChain(10_000);
		// The prompt shrinks to 2.5K: 8.1K of the previous prefix goes unread,
		// but only 500 tokens were written.
		const heat = getChatHeat([
			...turn(next()),
			...turn(
				request({
					input_tokens: 100,
					cache_read_tokens: 2_000,
					cache_creation_tokens: 400,
				}),
			),
		]);
		expect(heat?.lastTurn).toMatchObject({
			missedTokens: 500,
			reusableTokens: 10_100,
		});
	});

	it("does not count the first request of the chat as a miss", () => {
		const heat = getChatHeat([
			...turn(request({ cache_creation_tokens: 30_000 })),
		]);
		expect(heat?.lastTurn).toMatchObject({
			missedTokens: 0,
			reusableTokens: 0,
		});
	});

	it("drops the oldest loaded turn when older history is unloaded", () => {
		const next = requestChain(10_000);
		const heat = getChatHeat(
			[...turn(next(Number.POSITIVE_INFINITY)), ...turn(next(5_000))],
			false,
		);
		expect(heat?.lastTurn).toMatchObject({
			missedTokens: 5_000,
			isPartial: false,
		});
	});

	it("marks the latest turn partial when it is the only loaded turn", () => {
		const next = requestChain(10_000);
		const heat = getChatHeat([next(), next(3_000)], false);
		expect(heat?.lastTurn?.isPartial).toBe(true);
		expect(heat?.lastPromptTokens).toBe(10_200);
	});

	it("treats the only loaded turn as complete when its user message is loaded", () => {
		const next = requestChain(10_000);
		const heat = getChatHeat([...turn(next(), next(3_000))], false);
		expect(heat?.lastTurn).toMatchObject({
			missedTokens: 3_000,
			isPartial: false,
		});
	});

	it("starts a new segment at a compaction or clear boundary", () => {
		for (const boundary of [MockChatCompactionMessage, clearedMessage]) {
			const next = requestChain(10_000);
			const heat = getChatHeat(
				[
					...turn(next(), next(9_000)),
					boundary,
					...turn(next(Number.POSITIVE_INFINITY)),
				],
				false,
			);
			expect(heat?.lastTurn).toMatchObject({
				missedTokens: 0,
				reusableTokens: 0,
			});
			expect(heat?.boundary).toBeUndefined();
		}
	});

	it("reports a boundary with no request after it", () => {
		const next = requestChain(10_000);
		const before = turn(next(), next(9_000));
		expect(getChatHeat([...before, MockChatCompactionMessage])).toEqual({
			lastPromptTokens: 0,
			lastRequestAt: "",
			lastModelConfigId: undefined,
			boundary: "compacted",
			lastTurn: undefined,
		});
		expect(getChatHeat([...before, clearedMessage])?.boundary).toBe("cleared");
	});
});

describe("getCacheExpiresAtMs", () => {
	const requestAt = "2026-01-01T00:00:00Z";
	const requestMs = Date.parse(requestAt);

	it("expires one lifetime after the request", () => {
		expect(getCacheExpiresAtMs(requestAt, undefined)).toBe(
			requestMs + CACHE_IDLE_TTL_MS,
		);
	});

	it("measures from the later of request and stream end", () => {
		expect(getCacheExpiresAtMs(requestAt, requestMs + 30_000)).toBe(
			requestMs + 30_000 + CACHE_IDLE_TTL_MS,
		);
		expect(getCacheExpiresAtMs(requestAt, requestMs - 30_000)).toBe(
			requestMs + CACHE_IDLE_TTL_MS,
		);
	});

	it("is unknown without any timing", () => {
		expect(getCacheExpiresAtMs("", undefined)).toBeUndefined();
		expect(getCacheExpiresAtMs("not a date", undefined)).toBeUndefined();
		expect(getCacheExpiresAtMs("", 1_000)).toBe(1_000 + CACHE_IDLE_TTL_MS);
	});
});

describe("getRemainingMinutes", () => {
	it("rounds up and never reads zero", () => {
		expect(getRemainingMinutes(CACHE_IDLE_TTL_MS)).toBe(5);
		expect(getRemainingMinutes(4 * 60_000 + 1)).toBe(5);
		expect(getRemainingMinutes(4 * 60_000)).toBe(4);
		expect(getRemainingMinutes(60_000)).toBe(1);
		expect(getRemainingMinutes(1)).toBe(1);
	});

	it("clamps a clock ahead of the request to the full lifetime", () => {
		expect(getRemainingMinutes(CACHE_IDLE_TTL_MS + 90_000)).toBe(5);
	});
});

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
		expect(getChatHeat([message], 70, CONTEXT_LIMIT)?.heat).toBeCloseTo(
			heatCurve(0.3),
		);
	});

	it("computes fresh share and miss rate for one request", () => {
		const heat = getChatHeat(
			[
				request({
					input_tokens: 1_000,
					cache_creation_tokens: 20_000,
					cache_read_tokens: 9_000,
					output_tokens: 50_000,
				}),
			],
			70,
		);
		expect(heat?.missRate).toBeCloseTo(0.7);
		expect(heat?.heat).toBeCloseTo(heatCurve(0.21 / 0.7));
		expect(heat?.lastFreshTokens).toBe(21_000);
		expect(heat?.lastCacheReadTokens).toBe(9_000);
		expect(heat?.lastPromptTokens).toBe(30_000);
	});

	it("weights the newest request most", () => {
		const coldThenWarm = getChatHeat(
			[requestWithShare(0.4), requestWithShare(0)],
			100,
		);
		const warmThenCold = getChatHeat(
			[requestWithShare(0), requestWithShare(0.4)],
			100,
		);
		// Weights 0.5 and 0.25 normalize to 2/3 and 1/3.
		expect(coldThenWarm?.heat).toBeCloseTo(heatCurve(0.4 / 3));
		expect(warmThenCold?.heat).toBeCloseTo(heatCurve((0.4 * 2) / 3));
	});

	it("ignores requests beyond the newest six", () => {
		const recent = Array.from({ length: 6 }, () => requestWithShare(0.1));
		const withOld = getChatHeat([requestWithShare(0.9), ...recent], 100);
		expect(withOld?.heat).toBeCloseTo(heatCurve(0.1));
	});

	it("restarts the window at a compaction or clear boundary", () => {
		const cleared: TypesGen.ChatMessage = {
			...MockChatMessage,
			id: nextId++,
			role: "tool",
			content: [{ type: "tool-call", tool_name: "chat_cleared" }],
		};
		for (const boundary of [MockChatCompactionMessage, cleared]) {
			expect(
				getChatHeat(
					[requestWithShare(0.9), boundary, requestWithShare(0.1)],
					100,
				)?.heat,
			).toBeCloseTo(heatCurve(0.1));
			expect(getChatHeat([requestWithShare(0.9), boundary], 100)).toBeNull();
		}
	});

	it("normalizes by the compaction threshold", () => {
		const messages = [requestWithShare(0.15)];
		expect(getChatHeat(messages, 50)?.heat).toBeCloseTo(heatCurve(0.3));
		expect(getChatHeat(messages, undefined)?.heat).toBeCloseTo(heatCurve(0.15));
		expect(getChatHeat(messages, 0)?.heat).toBeCloseTo(heatCurve(0.15));
	});

	it("reports the newest request's timestamp", () => {
		const heat = getChatHeat(
			[
				request({ input_tokens: 1 }, "2026-01-01T00:00:00Z"),
				request({ input_tokens: 1 }, "2026-01-01T00:10:00Z"),
			],
			70,
		);
		expect(heat?.lastRequestAt).toBe("2026-01-01T00:10:00Z");
	});
});

describe("isCacheLikelyExpired", () => {
	const last = "2026-01-01T00:00:00Z";
	const lastMs = Date.parse(last);

	it("expires after the idle lifetime", () => {
		expect(isCacheLikelyExpired(last, lastMs + CACHE_IDLE_TTL_MS, false)).toBe(
			false,
		);
		expect(
			isCacheLikelyExpired(last, lastMs + CACHE_IDLE_TTL_MS + 1, false),
		).toBe(true);
	});

	it("never expires while streaming", () => {
		expect(isCacheLikelyExpired(last, lastMs + 60 * 60 * 1000, true)).toBe(
			false,
		);
	});
});

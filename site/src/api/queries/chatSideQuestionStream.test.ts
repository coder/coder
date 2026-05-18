import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import {
	type ChatSideQuestionStreamEvent,
	createChatSideQuestionStreamParser,
	streamChatSideQuestion,
} from "./chats";

afterEach(() => {
	vi.restoreAllMocks();
	delete API.getAxiosInstance().defaults.headers.common["Coder-Session-Token"];
	delete API.getAxiosInstance().defaults.headers.common["X-CSRF-TOKEN"];
	API.setHost(undefined);
});

describe("createChatSideQuestionStreamParser", () => {
	it("parses NDJSON records split across chunks", () => {
		const events: ChatSideQuestionStreamEvent[] = [];
		const parser = createChatSideQuestionStreamParser((event) => {
			events.push(event);
		});

		parser.push('{"type":"answer_delta","delta":"hel');
		parser.push('lo"}\n{"type":"answer_reset"');
		parser.push(',"reason":"retry"}\n');
		parser.finish();

		expect(events).toEqual([
			{ type: "answer_delta", delta: "hello" },
			{ type: "answer_reset", reason: "retry" },
		]);
	});

	it("rejects malformed stream events", () => {
		const parser = createChatSideQuestionStreamParser(() => undefined);

		expect(() => parser.push('{"type":"answer_delta"}\n')).toThrow(
			"Malformed side question stream event.",
		);
	});
});

describe("streamChatSideQuestion", () => {
	it("streams with API auth headers and returns the completed event", async () => {
		API.setHost("https://coder.example.test");
		API.setSessionToken("session-token");
		API.getAxiosInstance().defaults.headers.common["X-CSRF-TOKEN"] =
			"csrf-token";
		const fetchSpy = vi
			.spyOn(globalThis, "fetch")
			.mockResolvedValue(
				new Response(
					'{"type":"answer_delta","delta":"hel"}\n{"type":"completed","answer":"hello","usage":{"input_tokens":1}}\n',
					{ status: 200 },
				),
			);
		const events: ChatSideQuestionStreamEvent[] = [];

		const completed = await streamChatSideQuestion("chat 1").mutationFn({
			req: { question: "what changed?" },
			onEvent: (event) => events.push(event),
		});

		expect(completed).toEqual({
			type: "completed",
			answer: "hello",
			usage: { input_tokens: 1 },
		});
		expect(events).toEqual([
			{ type: "answer_delta", delta: "hel" },
			{ type: "completed", answer: "hello", usage: { input_tokens: 1 } },
		]);
		expect(fetchSpy).toHaveBeenCalledTimes(1);
		const [url, init] = fetchSpy.mock.calls[0];
		expect(url).toBe(
			"https://coder.example.test/api/experimental/chats/chat%201/side-questions/stream",
		);
		expect(init?.credentials).toBe("same-origin");
		expect(init?.method).toBe("POST");
		expect(JSON.parse(String(init?.body))).toEqual({
			question: "what changed?",
		});
		const headers = init?.headers;
		expect(headers).toBeInstanceOf(Headers);
		expect((headers as Headers).get("Coder-Session-Token")).toBe(
			"session-token",
		);
		expect((headers as Headers).get("X-CSRF-TOKEN")).toBe("csrf-token");
	});
});

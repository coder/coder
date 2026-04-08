import { bench, describe } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	createParsedConversationCache,
	resolveParsedConversation,
} from "./messageParsing";

const benchChatID = "bench-chat";

const makeBenchMessage = (id: number): TypesGen.ChatMessage => {
	const role = id % 2 === 0 ? "assistant" : "user";
	if (role === "assistant") {
		return {
			id,
			chat_id: benchChatID,
			created_at: `2025-01-01T00:00:${String(id % 60).padStart(2, "0")}.000Z`,
			role,
			content: [
				{ type: "text", text: `assistant-text-${id}` },
				{ type: "tool-call", tool_call_id: `tool-${id}`, tool_name: "bash" },
				{ type: "tool-result", tool_call_id: `tool-${id}`, tool_name: "bash" },
			],
		};
	}

	return {
		id,
		chat_id: benchChatID,
		created_at: `2025-01-01T00:00:${String(id % 60).padStart(2, "0")}.000Z`,
		role,
		content: [{ type: "text", text: `user-text-${id}` }],
	};
};

const benchMessages = Array.from({ length: 400 }, (_, index) =>
	makeBenchMessage(index + 1),
);

const warmCache = createParsedConversationCache();
resolveParsedConversation({
	cache: warmCache,
	chatID: benchChatID,
	messages: benchMessages,
});

describe("resolveParsedConversation", () => {
	bench(
		"cold parse for a large chat window",
		() => {
			const coldCache = createParsedConversationCache();
			resolveParsedConversation({
				cache: coldCache,
				chatID: benchChatID,
				messages: benchMessages,
			});
		},
		{ iterations: 20 },
	);

	bench(
		"warm cache hit for a large chat window",
		() => {
			resolveParsedConversation({
				cache: warmCache,
				chatID: benchChatID,
				messages: benchMessages,
			});
		},
		{ iterations: 20 },
	);
});

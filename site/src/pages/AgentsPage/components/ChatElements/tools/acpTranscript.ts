import type {
	ChatACPTranscriptMessage,
	ChatMessagePart,
} from "#/api/typesGenerated";
import type { MergedTool } from "../../ChatConversation/types";
import { asRecord, asString, parseArgs } from "./utils";

type ACPOutputBlock =
	| { type: "response" | "thinking"; text: string }
	| { type: "tool"; tool: MergedTool };

const isTranscriptPart = (value: unknown): value is ChatMessagePart => {
	const part = asRecord(value);
	if (!part) return false;
	switch (part.type) {
		case "text":
		case "reasoning":
			return typeof part.text === "string";
		case "tool-call":
		case "tool-result":
			return (
				typeof part.tool_call_id === "string" &&
				(part.tool_name === undefined || typeof part.tool_name === "string")
			);
		default:
			return false;
	}
};

const transcriptMessages = (value: unknown): ChatACPTranscriptMessage[] => {
	if (!Array.isArray(value)) return [];
	return value.flatMap((item) => {
		const message = asRecord(item);
		if (
			!message ||
			(message.role !== "assistant" && message.role !== "tool") ||
			!Array.isArray(message.content)
		) {
			return [];
		}
		return [
			{ role: message.role, content: message.content.filter(isTranscriptPart) },
		];
	});
};

/** Accumulates newline-delimited ACP parts, retaining only an unfinished record. */
export function mergeACPTranscriptDelta(
	existingValue: unknown,
	pending: string | undefined,
	delta: string,
): { value: { messages: ChatACPTranscriptMessage[] }; rawText: string } {
	const messages = transcriptMessages(asRecord(existingValue)?.messages);
	const records = `${pending ?? ""}${delta}`.split("\n");
	const rawText = records.pop() ?? "";
	for (const line of records) {
		const record = parseArgs(line);
		if (
			!record ||
			typeof record.message_index !== "number" ||
			!Number.isSafeInteger(record.message_index) ||
			record.message_index < 0 ||
			(record.role !== "assistant" && record.role !== "tool") ||
			!isTranscriptPart(record.part)
		) {
			continue;
		}
		const { message_index: index, role, part } = record;
		// A tool result's position can be reserved before later messages arrive.
		while (messages.length <= index) {
			messages.push({ role: "tool", content: [] });
		}
		const content = [...messages[index].content];
		const last = content.at(-1);
		if (
			(part.type === "text" || part.type === "reasoning") &&
			last?.type === part.type
		) {
			content[content.length - 1] = { ...last, text: last.text + part.text };
		} else if (part.type === "tool-call" || part.type === "tool-result") {
			const existingIndex = content.findIndex(
				(item) =>
					item.type === part.type && item.tool_call_id === part.tool_call_id,
			);
			if (existingIndex < 0) content.push(part);
			else content[existingIndex] = part;
		} else {
			content.push(part);
		}
		messages[index] = { role, content };
	}
	return { value: { messages }, rawText };
}

/** Converts the user-visible ACP transcript into inline activity blocks. */
export function parseACPTranscript(messages: unknown): ACPOutputBlock[] {
	const parts = transcriptMessages(messages).flatMap(
		(message) => message.content,
	);
	const results = new Map(
		parts
			.filter((part) => part.type === "tool-result")
			.map((part) => [part.tool_call_id, part]),
	);
	const blocks: ACPOutputBlock[] = [];
	for (const part of parts) {
		if (part.type === "text" || part.type === "reasoning") {
			const text = part.text;
			if (text) {
				blocks.push({
					type: part.type === "text" ? "response" : "thinking",
					text,
				});
			}
		} else if (part.type === "tool-call") {
			const id = part.tool_call_id;
			const resultPart = results.get(id);
			const result = parseArgs(resultPart?.result);
			const failed =
				resultPart?.is_error === true || result?.status === "failed";
			const content = Array.isArray(result?.content)
				? result.content
						.map((item) => asRecord(asRecord(item)?.content))
						.filter((item) => item?.type === "text")
						.map((item) => asString(item?.text))
						.join("\n\n")
				: "";
			blocks.push({
				type: "tool",
				tool: {
					id: id || "",
					name: part.tool_name || "Tool",
					args: part.args,
					result: content || (result?.output ?? resultPart?.result),
					isError: failed,
					status: failed
						? "error"
						: result?.status === "completed"
							? "completed"
							: "running",
				},
			});
		}
	}
	return blocks;
}

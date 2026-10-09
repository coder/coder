import type { ChatACPTranscriptMessage } from "#/api/typesGenerated";
import type { MergedTool } from "../../ChatConversation/types";
import { asRecord, asString, parseArgs } from "./utils";

type ACPOutputBlock =
	| { type: "response" | "thinking"; text: string }
	| { type: "tool"; tool: MergedTool };

/** Converts the user-visible ACP transcript into inline activity blocks. */
export function parseACPTranscript(
	messages: readonly ChatACPTranscriptMessage[],
): ACPOutputBlock[] {
	const parts = messages
		.filter(
			(message) => message.role === "assistant" || message.role === "tool",
		)
		.flatMap((message) => message.content);
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

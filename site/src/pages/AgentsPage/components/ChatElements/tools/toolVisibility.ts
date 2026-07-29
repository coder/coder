import { getSubagentChatId, getSubagentDescriptor } from "./subagentDescriptor";
import {
	asNumber,
	asRecord,
	asString,
	parseArgs,
	type ToolStatus,
} from "./utils";

export type ExecuteTranscriptBlock = {
	kind: "output" | "error";
	text: string;
};

type ExecuteRenderData = {
	command: string;
	transcriptBlocks: ExecuteTranscriptBlock[];
	durationMs?: number;
	isBackgrounded: boolean;
};

/**
 * Execute payloads can arrive partially populated, so args and results are
 * normalized here into the fields the row renders.
 */
export const getExecuteRenderData = (
	args: unknown,
	result: unknown,
): ExecuteRenderData => {
	const parsedArgs = parseArgs(args);
	const command = parsedArgs ? asString(parsedArgs.command) : "";
	const rec = asRecord(result);
	const output = rec ? asString(rec.output).trim() : "";
	const error = rec ? asString(rec.error).trim() : "";
	const fallbackMessage = rec && !error ? asString(rec.message).trim() : "";
	const errorText = error || fallbackMessage;
	const transcriptBlocks: ExecuteTranscriptBlock[] = [];
	if (output) {
		transcriptBlocks.push({ kind: "output", text: output });
	}
	if (errorText) {
		transcriptBlocks.push({ kind: "error", text: errorText });
	}
	const durationMs = rec
		? (asNumber(rec.wall_duration_ms, { parseString: true }) ??
			asNumber(rec.duration_ms, { parseString: true }))
		: undefined;
	const isBackgrounded = Boolean(
		rec && asString(rec.background_process_id).trim(),
	);
	return {
		command,
		transcriptBlocks,
		durationMs,
		isBackgrounded,
	};
};

const shouldRenderSubagentLifecycleTool = ({
	name,
	status,
	args,
	result,
}: {
	name: string;
	status: ToolStatus;
	args?: unknown;
	result?: unknown;
}): boolean => {
	const descriptor = getSubagentDescriptor({ name, args, result });
	if (!descriptor || status !== "running") {
		return true;
	}

	if (
		descriptor.action !== "wait" &&
		descriptor.action !== "message" &&
		descriptor.action !== "interrupt"
	) {
		return true;
	}

	// Wait, message, and interrupt rows can stream before their target chat_id
	// arrives. They get silence rather than a placeholder, because generic
	// lifecycle copy is wrong until the transcript resolves the real title.
	return Boolean(getSubagentChatId({ args, result }));
};

/**
 * True while an `execute` row has no command and no error explaining its
 * absence: nothing can render from it, and `toTimelineBlocks` holds its place
 * with an `unresolved-tool` row while the stream is live. An errored row is
 * never pending, so that error still reaches the transcript.
 */
export const isExecutePendingCommand = ({
	name,
	status,
	args,
}: {
	name: string;
	status: ToolStatus;
	args?: unknown;
}): boolean =>
	name === "execute" &&
	status !== "error" &&
	asString(parseArgs(args)?.command).trim().length === 0;

/**
 * Centralize tool-row visibility so transcript message hiding stays in sync
 * with <Tool> row rendering and hidden rows never leave empty gaps behind.
 */
export const shouldRenderTool = ({
	name,
	status,
	args,
	result,
}: {
	name: string;
	status: ToolStatus;
	args?: unknown;
	result?: unknown;
}): boolean => {
	if (isExecutePendingCommand({ name, status, args })) {
		return false;
	}

	return shouldRenderSubagentLifecycleTool({ name, status, args, result });
};

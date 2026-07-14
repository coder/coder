import { getSubagentChatId, getSubagentDescriptor } from "./subagentDescriptor";
import {
	asNumber,
	asRecord,
	asString,
	parseArgs,
	type ToolStatus,
	toProviderLabel,
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
	authenticateURL: string;
	providerLabel: string;
};

/**
 * Execute payloads can arrive partially populated, so visibility and rendering
 * share one defensive, normalized interpretation of args and results here.
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
	const authenticateURL = rec?.auth_required
		? asString(rec.authenticate_url).trim()
		: "";
	const providerLabel = toProviderLabel(
		rec ? asString(rec.provider_display_name).trim() : "",
		rec ? asString(rec.provider_id).trim() : "",
		rec ? asString(rec.provider_type).trim() : "",
	);

	return {
		command,
		transcriptBlocks,
		durationMs,
		isBackgrounded,
		authenticateURL,
		providerLabel,
	};
};

const shouldRenderExecuteTool = (data: ExecuteRenderData): boolean => {
	return data.command.trim().length > 0 || Boolean(data.authenticateURL);
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

	// Wait, message, and interrupt rows can stream before their target
	// chat_id arrives. Hiding them until that id exists avoids flashing generic
	// lifecycle copy before the transcript can resolve the real title.
	return Boolean(getSubagentChatId({ args, result }));
};

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
	if (name === "execute") {
		return shouldRenderExecuteTool(getExecuteRenderData(args, result));
	}

	return shouldRenderSubagentLifecycleTool({ name, status, args, result });
};

import { getSubagentChatId, getSubagentDescriptor } from "./subagentDescriptor";
import {
	asBoolean,
	asNumber,
	asRecord,
	asString,
	parseArgs,
	type ToolStatus,
} from "./utils";

export type ExecuteTranscriptBlock = {
	kind: "output" | "error" | "status";
	text: string;
};

type ExecuteRenderData = {
	command: string;
	transcriptBlocks: ExecuteTranscriptBlock[];
	errorText: string;
	durationMs?: number;
	isBackgrounded: boolean;
	/** The tracked process ID when the process outlived the call. */
	processId?: string;
	/** A foreground wait expired without the process finishing. */
	timedOut: boolean;
	/** Whether the result confirmed the process was still alive. */
	processRunning: boolean;
	/** The model's requested wait limit, e.g. "30s". */
	waitLimit?: string;
};

const timedOutErrorPrefix = "command timed out after";
const snapshotFailureFragment = "failed to get output";

/**
 * Execute payloads can arrive partially populated, so visibility and rendering
 * share one defensive, normalized interpretation of args and results here.
 *
 * A foreground timeout is classified structurally: the process outlived the
 * call, so the result carries a process ID the call never intended to track.
 * Newer results carry timed_out/running directly; the legacy branch only
 * matches the narrow error signature the old timeout paths produced.
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
	const processId = rec ? asString(rec.background_process_id).trim() : "";
	const exitCode = rec
		? (asNumber(rec.exit_code, { parseString: true }) ?? null)
		: null;
	// Foreground timeouts also set background_process_id, so fall
	// back to the call args for older transcripts without the flag.
	// That includes trailing-& commands, which the tool promotes to
	// background without adding run_in_background to the args. The
	// args record intent, not outcome, so require a process ID as
	// evidence the launch actually happened.
	const trimmedCommand = command.trimEnd();
	const hasTrailingAmp =
		trimmedCommand.endsWith("&") &&
		!trimmedCommand.endsWith("&&") &&
		!trimmedCommand.endsWith("|&");
	const hasProcessID = Boolean(processId);
	const isBackgrounded =
		rec?.backgrounded === true ||
		(rec?.backgrounded === undefined &&
			hasProcessID &&
			(parsedArgs?.run_in_background === true || hasTrailingAmp));

	// A deliberate background launch reports no process state, so only
	// foreground results classify as timeouts. Path C (the recovery
	// snapshot also failed) carries the process ID but cannot confirm
	// liveness; its error string distinguishes it.
	const timedOutFlag = asBoolean(rec?.timed_out);
	const runningFlag = asBoolean(rec?.running);
	const legacyTimedOut =
		timedOutFlag === undefined &&
		hasProcessID &&
		!isBackgrounded &&
		exitCode === -1 &&
		error.startsWith(timedOutErrorPrefix);
	const timedOut = timedOutFlag ?? legacyTimedOut;
	const processRunning =
		runningFlag ??
		(timedOut &&
			!error.includes(snapshotFailureFragment) &&
			!errorText.includes(snapshotFailureFragment));

	const transcriptBlocks: ExecuteTranscriptBlock[] = [];
	if (output) {
		transcriptBlocks.push({ kind: "output", text: output });
	}
	// The timeout message is tool-control metadata for the model, not
	// process output. It moves out of the error channel once the result
	// is classified; the unknown-liveness path keeps it as a neutral
	// status line because it carries real diagnostics.
	if (errorText && !(timedOut && processRunning)) {
		transcriptBlocks.push({
			kind: timedOut ? "status" : "error",
			text: errorText,
		});
	}

	const durationMs = rec
		? (asNumber(rec.wall_duration_ms, { parseString: true }) ??
			asNumber(rec.duration_ms, { parseString: true }))
		: undefined;
	const waitLimit = parsedArgs ? asString(parsedArgs.timeout).trim() : "";

	return {
		command,
		transcriptBlocks,
		errorText,
		durationMs,
		isBackgrounded,
		processId: processId || undefined,
		timedOut,
		processRunning,
		waitLimit: waitLimit || undefined,
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
		return getExecuteRenderData(args, result).command.trim().length > 0;
	}

	return shouldRenderSubagentLifecycleTool({ name, status, args, result });
};

import type { ChatDebugRun } from "#/api/typesGenerated";

// Keep in sync with maxDebugRuns in coderd/exp_chats.go.
export const DEBUG_RUN_LIST_LIMIT = 100;

const DEBUG_ID_PREFIX_LENGTH = 8;

export interface ChatDebugRunFetchFailure {
	readonly run_id: string;
	readonly message: string;
}

interface DebugRunExport {
	readonly version: 1;
	readonly scope: "run";
	readonly exported_at: string;
	readonly chat_id: string;
	readonly run_id: string;
	readonly run: ChatDebugRun;
}

interface DebugChatExport {
	readonly version: 1;
	readonly scope: "chat";
	readonly exported_at: string;
	readonly chat_id: string;
	readonly run_count: number;
	readonly requested_run_count: number;
	readonly limited_to_most_recent: number;
	readonly failed_runs?: readonly ChatDebugRunFetchFailure[];
	readonly runs: readonly ChatDebugRun[];
}

type ChatDebugExport = DebugRunExport | DebugChatExport;

export type DownloadDebugFile = (
	blob: Blob,
	filename: string,
) => void | Promise<void>;

export const buildRunDebugExport = (
	chatId: string,
	run: ChatDebugRun,
	exportedAt = new Date(),
): DebugRunExport => ({
	version: 1,
	scope: "run",
	exported_at: exportedAt.toISOString(),
	chat_id: chatId,
	run_id: run.id,
	run,
});

export const buildChatDebugExport = (
	chatId: string,
	runs: readonly ChatDebugRun[],
	exportedAt = new Date(),
	options: {
		readonly failedRuns?: readonly ChatDebugRunFetchFailure[];
		readonly requestedRunCount?: number;
	} = {},
): DebugChatExport => {
	const failedRuns = options.failedRuns ?? [];
	return {
		version: 1,
		scope: "chat",
		exported_at: exportedAt.toISOString(),
		chat_id: chatId,
		run_count: runs.length,
		requested_run_count: options.requestedRunCount ?? runs.length,
		limited_to_most_recent: DEBUG_RUN_LIST_LIMIT,
		...(failedRuns.length > 0 ? { failed_runs: failedRuns } : {}),
		runs,
	};
};

export const buildDebugExportBlob = (payload: ChatDebugExport): Blob => {
	return new Blob([JSON.stringify(payload, null, 2)], {
		type: "application/json",
	});
};

export const debugExportFilename = ({
	chatId,
	runId,
	exportedAt = new Date(),
}: {
	chatId: string;
	runId?: string;
	exportedAt?: Date;
}): string => {
	const timestamp = exportedAt.toISOString().replace(/[:.]/g, "-");
	const idPrefix = (runId ?? chatId).slice(0, DEBUG_ID_PREFIX_LENGTH);
	const scope = runId ? "run" : "chat";
	return `coder-agents-debug-${scope}-${idPrefix}-${timestamp}.json`;
};

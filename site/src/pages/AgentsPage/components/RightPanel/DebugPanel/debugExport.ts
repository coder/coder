import type { ChatDebugRun } from "#/api/typesGenerated";

export const DEBUG_RUN_EXPORT_LIST_LIMIT = 100;

interface ChatDebugRunExport {
	readonly version: 1;
	readonly scope: "run";
	readonly exported_at: string;
	readonly chat_id: string;
	readonly run_id: string;
	readonly run: ChatDebugRun;
}

interface ChatDebugChatExport {
	readonly version: 1;
	readonly scope: "chat";
	readonly exported_at: string;
	readonly chat_id: string;
	readonly run_count: number;
	readonly limited_to_most_recent: number;
	readonly runs: readonly ChatDebugRun[];
}

type ChatDebugExport = ChatDebugRunExport | ChatDebugChatExport;

export type DownloadDebugFile = (
	blob: Blob,
	filename: string,
) => void | Promise<void>;

export const buildRunDebugExport = (
	chatId: string,
	run: ChatDebugRun,
	exportedAt = new Date(),
): ChatDebugRunExport => ({
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
): ChatDebugChatExport => ({
	version: 1,
	scope: "chat",
	exported_at: exportedAt.toISOString(),
	chat_id: chatId,
	run_count: runs.length,
	limited_to_most_recent: DEBUG_RUN_EXPORT_LIST_LIMIT,
	runs,
});

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
	const idPrefix = (runId ?? chatId).slice(0, 8);
	const scope = runId ? "run" : "chat";
	return `coder-agents-debug-${scope}-${idPrefix}-${timestamp}.json`;
};

import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

const debugRunTerminalStatuses = new Set([
	"completed",
	"error",
	"failed",
	"interrupted",
	"cancelled",
	"canceled",
]);

const debugRunRefetchInterval = (
	run: Pick<TypesGen.ChatDebugRun, "status"> | undefined,
	hasError?: boolean,
): number | false => {
	if (hasError) {
		return false;
	}
	if (run?.status && debugRunTerminalStatuses.has(run.status.toLowerCase())) {
		return false;
	}
	return 5_000;
};

export const chatDebugRunsKey = (chatId: string) =>
	["chats", chatId, "debug-runs"] as const;

export const chatDebugRuns = (chatId: string) => ({
	queryKey: chatDebugRunsKey(chatId),
	queryFn: () => API.experimental.getChatDebugRuns(chatId),
	refetchInterval: ({
		state,
	}: {
		state: { data?: unknown; status: string };
	}): number | false => {
		if (state.status === "error") {
			return false;
		}
		const runs = state.data;
		if (Array.isArray(runs) && runs.length > 0) {
			const allTerminal = runs.every((r: { status?: string }) =>
				debugRunTerminalStatuses.has((r.status ?? "").toLowerCase()),
			);
			if (allTerminal) {
				return false;
			}
		}
		return 5_000;
	},
	refetchIntervalInBackground: false,
});

export const chatDebugRun = (chatId: string, runId: string) => ({
	queryKey: [...chatDebugRunsKey(chatId), runId] as const,
	queryFn: () => API.experimental.getChatDebugRun(chatId, runId),
	refetchInterval: ({
		state,
	}: {
		state: { data: TypesGen.ChatDebugRun | undefined; status: string };
	}) => debugRunRefetchInterval(state.data, state.status === "error"),
	refetchIntervalInBackground: false,
});

export const chatDebugLogging = () => ({
	queryKey: ["chats", "config", "debug-logging"] as const,
	queryFn: () => API.experimental.getChatDebugLogging(),
});

export const chatUserDebugLogging = () => ({
	queryKey: ["chats", "config", "user-debug-logging"] as const,
	queryFn: () => API.experimental.getChatUserDebugLogging(),
});

import type { SessionInfo } from "@agentclientprotocol/sdk";
import { infiniteQueryOptions, type QueryClient } from "react-query";
import type { NativeAgentSession } from "#/modules/resources/NativeAgentSession";

const nativeAgentSessionsKey = (agentID: string) => [
	"workspaceagent",
	agentID,
	"native-sessions",
];

/** Discovers sessions created by startup scripts without opening another SSH connection. */
export const nativeAgentSessions = (session: NativeAgentSession) =>
	infiniteQueryOptions({
		queryKey: nativeAgentSessionsKey(session.agentID),
		queryFn: ({ pageParam }: { pageParam: string | undefined }) =>
			session.list(pageParam),
		initialPageParam: undefined,
		getNextPageParam: (page) => page.nextCursor ?? undefined,
		retry: false,
		refetchInterval: (query) =>
			query.state.status === "error" ? false : 10_000,
		refetchOnWindowFocus: false,
		refetchOnReconnect: false,
	});

/** ACP operations are never retried; automatic discovery may only connect and list. */
export type NativeAgentAction =
	| { type: "connect"; signal?: AbortSignal }
	| { type: "create"; cwd: string }
	| { type: "load"; session: SessionInfo }
	| { type: "mode"; mode: string }
	| { type: "join"; session: SessionInfo }
	| { type: "prompt"; text: string }
	| { type: "cancel" };

/** Refreshes discovery even when a multi-step operation partially succeeds. */
export const nativeAgentAction = (
	session: NativeAgentSession,
	queryClient: QueryClient,
) => ({
	retry: false,
	mutationFn: async (action: NativeAgentAction) => {
		switch (action.type) {
			case "connect":
				await session.connect(action.signal);
				break;
			case "create":
				await session.create(action.cwd);
				break;
			case "load":
				await session.load(action.session);
				break;
			case "mode":
				await session.setMode(action.mode);
				break;
			case "join":
				await session.load(action.session);
				await session.setMode("human");
				break;
			case "prompt":
				await session.prompt(action.text);
				break;
			case "cancel":
				await session.cancel();
				break;
		}
	},
	onSettled: (
		_data: unknown,
		_error: Error | null,
		action: NativeAgentAction,
	) => {
		if (["connect", "create", "mode", "join", "prompt"].includes(action.type)) {
			void queryClient.invalidateQueries({
				queryKey: nativeAgentSessionsKey(session.agentID),
			});
		}
	},
});

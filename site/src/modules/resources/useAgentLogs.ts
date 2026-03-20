import { watchWorkspaceAgentLogs } from "api/api";
import type { WorkspaceAgentLog } from "api/typesGenerated";
import { useEffect, useState } from "react";
import { toast } from "sonner";
import type { OneWayMessageEvent } from "utils/OneWayWebSocket";

export const MAX_LOGS = 1_000;

type UseAgentLogsOptions = Readonly<{
	agentId: string;
	enabled?: boolean;
}>;

export function useAgentLogs(
	options: UseAgentLogsOptions,
): readonly WorkspaceAgentLog[] {
	const { agentId, enabled = true } = options;
	const [logs, setLogs] = useState<readonly WorkspaceAgentLog[]>([]);

	// Clean up the logs when the agent is not enabled, using a mid-render
	// sync to remove any risk of screen flickering. Clearing the logs helps
	// ensure that if the hook flips back to being enabled, we can receive a
	// fresh set of logs from the beginning with zero risk of duplicates.
	const [prevEnabled, setPrevEnabled] = useState(enabled);
	if (!enabled && prevEnabled) {
		setLogs([]);
		setPrevEnabled(false);
	}
	if (enabled && !prevEnabled) {
		setPrevEnabled(true);
	}

	useEffect(() => {
		if (!enabled) {
			return;
		}

		let disposed = false;

		// Always fetch the logs from the beginning. We may want to optimize
		// this in the future, but it would add some complexity in the code
		// that might not be worth it.
		const socket = watchWorkspaceAgentLogs(agentId, { after: 0 });

		const handleMessage = (
			e: OneWayMessageEvent<readonly WorkspaceAgentLog[]>,
		) => {
			if (disposed) {
				return;
			}
			if (e.parseError) {
				console.warn("Error parsing agent log: ", e.parseError);
				return;
			}
			if (e.parsedMessage.length === 0) {
				return;
			}

			setLogs((logs) => {
				const nextLogs = [...logs, ...e.parsedMessage];
				nextLogs.sort((l1, l2) => {
					const d1 = new Date(l1.created_at).getTime();
					const d2 = new Date(l2.created_at).getTime();
					return d1 - d2;
				});

				// Keep the newest logs only so long-running streams do not retain an
				// unbounded log history in memory.
				return nextLogs.slice(-MAX_LOGS);
			});
		};

		const cleanupSocket = () => {
			if (disposed) {
				return;
			}

			disposed = true;
			socket.removeEventListener("message", handleMessage);
			socket.removeEventListener("error", handleError);
			socket.close();
		};

		const handleError = (error: Event) => {
			if (disposed) {
				return;
			}

			console.error("Error in agent log socket: ", error);
			toast.error(`Unable to watch "${agentId}" agent logs.`, {
				description: "Please try refreshing the browser.",
			});
			cleanupSocket();
		};

		socket.addEventListener("message", handleMessage);
		socket.addEventListener("error", handleError);

		return cleanupSocket;
	}, [agentId, enabled]);

	return logs;
}

import {
	type WatchWorkspaceAgentLogsParams,
	watchWorkspaceAgentLogs,
} from "api/api";
import type { WorkspaceAgentLog } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { useEffect, useState } from "react";
import type { OneWayWebSocket } from "utils/OneWayWebSocket";

type CreateSocket = (
	agentId: string,
	params?: WatchWorkspaceAgentLogsParams,
) => OneWayWebSocket<WorkspaceAgentLog[]>;

export type OnError = (errorEvent: Event) => void;

export function createUseAgentLogs(
	createSocket: CreateSocket,
	onError: OnError,
) {
	return function useAgentLogs(
		agentId: string,
		enabled: boolean,
	): readonly WorkspaceAgentLog[] {
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

		useEffect(() => {
			if (!enabled) {
				return;
			}

			// Always fetch the logs from the beginning. We may want to optimize
			// this in the future, but it would add some complexity in the code
			// that might not be worth it.
			const createdAtMap = new Map<string, number>();
			const socket = createSocket(agentId, { after: 0 });
			socket.addEventListener("message", (e) => {
				if (e.parseError) {
					console.warn("Error parsing agent log: ", e.parseError);
					return;
				}

				if (e.parsedMessage.length === 0) {
					return;
				}

				setLogs((logs) => {
					const newLogs = [...logs, ...e.parsedMessage];
					newLogs.sort((l1, l2) => {
						let d1 = createdAtMap.get(l1.created_at);
						if (d1 === undefined) {
							d1 = new Date(l1.created_at).getTime();
							createdAtMap.set(l1.created_at, d1);
						}
						let d2 = createdAtMap.get(l2.created_at);
						if (d2 === undefined) {
							d2 = new Date(l2.created_at).getTime();
							createdAtMap.set(l2.created_at, d2);
						}
						return d1 - d2;
					});

					return newLogs;
				});
			});

			socket.addEventListener("error", (e) => {
				onError(e);
				socket.close();
			});

			return () => socket.close();
		}, [createSocket, onError, agentId, enabled]);

		return logs;
	};
}

// The baseline implementation to use for production
export const useAgentLogs = createUseAgentLogs(
	watchWorkspaceAgentLogs,
	(errorEvent) => {
		console.error("Error in agent log socket: ", errorEvent);
		displayError(
			"Unable to watch agent logs",
			"Please try refreshing the browser",
		);
	},
);

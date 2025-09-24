import { watchWorkspaceAgentLogs } from "api/api";
import type { WorkspaceAgentLog } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { useEffect, useState } from "react";

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

		// Always fetch the logs from the beginning. We may want to optimize
		// this in the future, but it would add some complexity in the code
		// that might not be worth it.
		const socket = watchWorkspaceAgentLogs(agentId, { after: 0 });
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
					const d1 = new Date(l1.created_at).getTime();
					const d2 = new Date(l2.created_at).getTime();
					return d1 - d2;
				});
				return newLogs;
			});
		});

		socket.addEventListener("error", (e) => {
			console.error("Error in agent log socket: ", e);
			displayError(
				"Unable to watch agent logs",
				"Please try refreshing the browser",
			);
			socket.close();
		});

		return () => {
			socket.close();
		};
	}, [agentId, enabled]);

	return logs;
}

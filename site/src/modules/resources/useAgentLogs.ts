import { watchWorkspaceAgentLogs } from "api/api";
import type { WorkspaceAgent, WorkspaceAgentLog } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { useEffect, useState } from "react";

export function useAgentLogs(
	agent: WorkspaceAgent,
	enabled: boolean,
): readonly WorkspaceAgentLog[] {
	const [logs, setLogs] = useState<WorkspaceAgentLog[]>([]);

	useEffect(() => {
		if (!enabled) {
			// Clean up the logs when the agent is not enabled. So it can receive logs
			// from the beginning without duplicating the logs.
			setLogs([]);
			return;
		}

		// Always fetch the logs from the beginning. We may want to optimize this in
		// the future, but it would add some complexity in the code that maybe does
		// not worth it.
		const socket = watchWorkspaceAgentLogs(agent.id, { after: 0 });
		socket.addEventListener("message", (e) => {
			if (e.parseError) {
				console.warn("Error parsing agent log: ", e.parseError);
				return;
			}
			setLogs((logs) => [...logs, ...e.parsedMessage]);
		});

		socket.addEventListener("error", (e) => {
			console.error("Error in agent log socket: ", e);
			displayError(
				"Unable to watch the agent logs",
				"Please try refreshing the browser",
			);
			socket.close();
		});

		return () => {
			socket.close();
		};
	}, [agent.id, enabled]);

	return logs;
}

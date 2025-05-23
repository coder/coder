import { watchWorkspaceAgentLogs } from "api/api";
import type { WorkspaceAgent, WorkspaceAgentLog } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { useEffect, useState } from "react";

export function createUseAgentLogs(
	createSocket: typeof watchWorkspaceAgentLogs,
) {
	return function useAgentLogs(
		agent: WorkspaceAgent,
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
			const socket = createSocket(agent.id, { after: 0 });
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

			return () => socket.close();

			// createSocket shouldn't ever change for the lifecycle of the hook,
			// but Biome isn't smart enough to detect constant dependencies for
			// higher-order hooks. Adding it to the array (even though it
			// shouldn't ever be needed) seemed like the least fragile way to
			// resolve the warning.
		}, [createSocket, agent.id, enabled]);

		return logs;
	};
}

// The baseline implementation to use for production
export const useAgentLogs = createUseAgentLogs(watchWorkspaceAgentLogs);

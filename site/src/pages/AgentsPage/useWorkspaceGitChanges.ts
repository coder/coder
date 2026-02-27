import { watchAgentGitChanges } from "api/api";
import type {
	WorkspaceAgentGitChangesResponse,
	WorkspaceAgentRepoChanges,
} from "api/typesGenerated";
import { useEffectEvent } from "hooks/hookPolyfills";
import { useEffect, useState } from "react";

export function useWorkspaceGitChanges(
	workspaceAgentId: string | undefined,
): readonly WorkspaceAgentRepoChanges[] | undefined {
	const [repos, setRepos] = useState<
		readonly WorkspaceAgentRepoChanges[] | undefined
	>(undefined);

	const handleMessage = useEffectEvent(
		(data: WorkspaceAgentGitChangesResponse) => {
			setRepos(data.repos);
		},
	);

	useEffect(() => {
		if (!workspaceAgentId) {
			return;
		}

		const socket = watchAgentGitChanges(workspaceAgentId);

		socket.addEventListener("message", (event) => {
			if (event.parseError) {
				return;
			}
			handleMessage(event.parsedMessage);
		});

		socket.addEventListener("error", () => {
			// Silently handle errors; the diff panel will show
			// "no changes" if we never receive data.
		});

		return () => socket.close();
	}, [workspaceAgentId, handleMessage]);

	return repos;
}

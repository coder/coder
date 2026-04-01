import { watchAgentContainers } from "api/api";
import {
	workspaceAgentContainers,
	workspaceAgentContainersKey,
} from "api/queries/workspaces";
import type {
	WorkspaceAgent,
	WorkspaceAgentDevcontainer,
	WorkspaceAgentListContainersResponse,
} from "api/typesGenerated";
import { useEffectEvent } from "hooks/hookPolyfills";
import { useEffect } from "react";
import { useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";

export function useAgentContainers(
	agent: WorkspaceAgent,
): readonly WorkspaceAgentDevcontainer[] | undefined {
	const queryClient = useQueryClient();
	const queryKey = workspaceAgentContainersKey(agent.id);

	const {
		data: devcontainers,
		error: queryError,
		isLoading: queryIsLoading,
	} = useQuery({
		...workspaceAgentContainers(agent),
		select: (res) => res.devcontainers,
	});

	const updateDevcontainersCache = useEffectEvent(
		async (data: WorkspaceAgentListContainersResponse) => {
			queryClient.setQueryData(queryKey, data);
		},
	);

	useEffect(() => {
		if (agent.status !== "connected" || queryIsLoading || queryError) {
			return;
		}

		const socket = watchAgentContainers(agent.id);

		socket.addEventListener("message", (event) => {
			if (event.parseError) {
				toast.error("Failed to update containers.", {
					description: "Please try refreshing the page.",
				});
				return;
			}

			updateDevcontainersCache(event.parsedMessage);
		});

		socket.addEventListener("error", () => {
			toast.error("Failed to load containers.", {
				description: "Please try refreshing the page.",
			});
		});

		return () => socket.close();
	}, [
		agent.id,
		agent.status,
		queryIsLoading,
		queryError,
		updateDevcontainersCache,
	]);

	return devcontainers;
}

import { API, watchAgentContainers } from "api/api";
import { workspaceAgentContainersKey } from "api/queries/workspaces";
import type {
	WorkspaceAgent,
	WorkspaceAgentDevcontainer,
	WorkspaceAgentListContainersResponse,
} from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { useEffectEvent } from "hooks/hookPolyfills";
import { useEffect } from "react";
import { useQuery, useQueryClient } from "react-query";

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
		queryKey,
		queryFn: () => API.getAgentContainers(agent.id),
		enabled: agent.status === "connected",
		select: (res) => res.devcontainers,
		staleTime: Number.POSITIVE_INFINITY,
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
				displayError(
					"Failed to update containers",
					"Please try refreshing the page",
				);
				return;
			}

			updateDevcontainersCache(event.parsedMessage);
		});

		socket.addEventListener("error", () => {
			displayError(
				"Failed to load containers",
				"Please try refreshing the page",
			);
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

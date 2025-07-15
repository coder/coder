import { API, watchAgentContainers } from "api/api";
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

	const { data: devcontainers } = useQuery({
		queryKey: ["agents", agent.id, "containers"],
		queryFn: () => API.getAgentContainers(agent.id),
		enabled: agent.status === "connected",
		select: (res) => res.devcontainers,
		staleTime: Number.POSITIVE_INFINITY,
	});

	const updateDevcontainersCache = useEffectEvent(
		async (data: WorkspaceAgentListContainersResponse) => {
			const queryKey = ["agents", agent.id, "containers"];

			queryClient.setQueryData(queryKey, data);
		},
	);

	useEffect(() => {
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
	}, [agent.id, updateDevcontainersCache]);

	return devcontainers;
}

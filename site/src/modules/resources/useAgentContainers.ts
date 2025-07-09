import { API, watchAgentContainers } from "api/api";
import type {
	WorkspaceAgent,
	WorkspaceAgentDevcontainer,
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
	});

	const updateDevcontainersCache = useEffectEvent(
		async (devcontainers: WorkspaceAgentDevcontainer[]) => {
			const queryKey = ["agents", agent.id, "containers"];

			queryClient.setQueryData(queryKey, devcontainers);
			await queryClient.invalidateQueries({ queryKey });
		},
	);

	useEffect(() => {
		const socket = watchAgentContainers(agent.id);

		socket.addEventListener("message", (event) => {
			if (event.parseError) {
				displayError(
					"Unable to process latest data from the server. Please try refreshing the page.",
				);
				return;
			}

			updateDevcontainersCache(event.parsedMessage);
		});

		socket.addEventListener("error", () => {
			displayError(
				"Unable to get workspace containers. Connection has been closed.",
			);
		});

		return () => socket.close();
	}, [agent.id, updateDevcontainersCache]);

	return devcontainers;
}

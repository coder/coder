import { useCallback, useEffect, useState } from "react";
import { useQuery } from "react-query";
import { templateVersionExternalAuth } from "#/api/queries/templates";

export type ExternalAuthPollingState = "idle" | "polling" | "abandoned";

export const useExternalAuth = (versionId: string | undefined) => {
	const [pollingState, setPollingState] = useState<
		Record<string, ExternalAuthPollingState>
	>({});

	const startPollingExternalAuth = useCallback((providerId: string) => {
		setPollingState((prev) => ({ ...prev, [providerId]: "polling" }));
	}, []);

	const isAnyPolling = Object.values(pollingState).some((s) => s === "polling");

	const {
		data: externalAuth,
		isPending: isLoadingExternalAuth,
		error,
	} = useQuery({
		...templateVersionExternalAuth(versionId ?? ""),
		enabled: Boolean(versionId),
		refetchInterval: isAnyPolling ? 1000 : false,
	});

	// Stop polling individual providers once they authenticate.
	useEffect(() => {
		if (!externalAuth) {
			return;
		}
		setPollingState((prev) => {
			let changed = false;
			const next = { ...prev };
			for (const auth of externalAuth) {
				if (auth.authenticated && next[auth.id] === "polling") {
					next[auth.id] = "idle";
					changed = true;
				}
			}
			return changed ? next : prev;
		});
	}, [externalAuth]);

	// Per-provider 60-second timeout.
	useEffect(() => {
		const pollingIds = Object.entries(pollingState)
			.filter(([, authPollingState]) => authPollingState === "polling")
			.map(([id]) => id);

		if (pollingIds.length === 0) {
			return;
		}

		const timers = pollingIds.map((id) =>
			setTimeout(() => {
				setPollingState((prev) =>
					prev[id] === "polling" ? { ...prev, [id]: "abandoned" } : prev,
				);
			}, 60_000),
		);

		return () => {
			for (const t of timers) {
				clearTimeout(t);
			}
		};
	}, [pollingState]);

	return {
		startPollingExternalAuth,
		externalAuth,
		externalAuthPollingState: pollingState,
		isLoadingExternalAuth,
		externalAuthError: error,
		isPollingExternalAuth: isAnyPolling,
	};
};

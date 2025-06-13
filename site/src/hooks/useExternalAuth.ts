import { templateVersionExternalAuth } from "api/queries/templates";
import { useCallback, useEffect, useState } from "react";
import { useQuery } from "react-query";

export type ExternalAuthPollingState = "idle" | "polling" | "abandoned";

export const useExternalAuth = (versionId: string | undefined) => {
	const [externalAuthPollingState, setExternalAuthPollingState] =
		useState<ExternalAuthPollingState>("idle");

	const startPollingExternalAuth = useCallback(() => {
		setExternalAuthPollingState("polling");
	}, []);

	const {
		data: externalAuth,
		isPending: isLoadingExternalAuth,
		error,
	} = useQuery({
		...templateVersionExternalAuth(versionId ?? ""),
		enabled: !!versionId,
		refetchInterval: externalAuthPollingState === "polling" ? 1000 : false,
	});

	const allSignedIn = externalAuth?.every((it) => it.authenticated);

	useEffect(() => {
		if (allSignedIn) {
			setExternalAuthPollingState("idle");
			return;
		}

		if (externalAuthPollingState !== "polling") {
			return;
		}

		// Poll for a maximum of one minute
		const quitPolling = setTimeout(
			() => setExternalAuthPollingState("abandoned"),
			60_000,
		);
		return () => {
			clearTimeout(quitPolling);
		};
	}, [externalAuthPollingState, allSignedIn]);

	return {
		startPollingExternalAuth,
		externalAuth,
		externalAuthPollingState,
		isLoadingExternalAuth,
		externalAuthError: error,
	};
};

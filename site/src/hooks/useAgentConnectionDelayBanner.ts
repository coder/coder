import type {
	AgentConnectionTiming,
	WorkspaceBuildTimings,
} from "api/typesGenerated";
import { useEffect, useState } from "react";

export type AgentConnectionDelayBannerState = {
	shouldShow: boolean;
};

const DELAY_THRESHOLD_MS = 10_000;
const INVALID_ENDED_AT = "0001-01-01T00:00:00Z";

export const useAgentConnectionDelayBanner = (
	timings: WorkspaceBuildTimings | undefined,
	enabled: boolean,
): AgentConnectionDelayBannerState => {
	const [shouldShow, setShouldShow] = useState(false);

	useEffect(() => {
		if (!enabled) {
			setShouldShow(false);
			return;
		}

		const checkDelay = () => {
			if (!timings?.agent_connection_timings) {
				setShouldShow(false);
				return;
			}

			const now = Date.now();
			const hasDelayedConnection = timings.agent_connection_timings.some(
				(timing: AgentConnectionTiming) => {
					if (timing.ended_at !== INVALID_ENDED_AT) {
						return false;
					}

					const startedAt = new Date(timing.started_at).getTime();
					const elapsed = now - startedAt;
					return elapsed >= DELAY_THRESHOLD_MS;
				},
			);

			setShouldShow(hasDelayedConnection);
		};

		checkDelay();

		const intervalId = window.setInterval(checkDelay, 1000);

		return () => {
			window.clearInterval(intervalId);
		};
	}, [timings, enabled]);

	return { shouldShow };
};

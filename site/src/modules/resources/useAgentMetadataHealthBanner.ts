import { watchAgentMetadata } from "api/api";
import type { WorkspaceAgentMetadata } from "api/typesGenerated";
import { useEffect, useMemo, useRef, useState } from "react";
import {
	isInvalidAgentMetadataSample,
	isValidAgentMetadataSample,
} from "./agentMetadataHealth";

export type AgentMetadataHealthBannerState = {
	shouldShow: boolean;
};

const THRESHOLD_MS = 30_000;

export const useAgentMetadataHealthBanner = (
	agentIds: readonly string[],
	enabled: boolean,
): AgentMetadataHealthBannerState => {
	// Memoize agent IDs with stable reference
	const agentIdsKey = useMemo(() => agentIds.join(","), [agentIds]);
	const stableAgentIds = useMemo(() => {
		const sorted = Array.from(new Set(agentIds)).sort();
		return sorted;
	}, [agentIdsKey]);
	
	const [shouldShow, setShouldShow] = useState(false);
	const invalidSinceByAgentRef = useRef<Map<string, number>>(new Map());
	const hasValidAgentRef = useRef(false);
	const timerRef = useRef<number | null>(null);
	const previousAgentIdsRef = useRef<string>("");

	useEffect(() => {
		// Only reset if agent IDs actually changed
		if (previousAgentIdsRef.current !== agentIdsKey) {
			console.log(
				"[AgentMetadataHealthBanner] Agent IDs changed:",
				previousAgentIdsRef.current,
				"->",
				agentIdsKey,
			);
			invalidSinceByAgentRef.current = new Map();
			hasValidAgentRef.current = false;
			setShouldShow(false);
			previousAgentIdsRef.current = agentIdsKey;
		}

		if (timerRef.current !== null) {
			window.clearInterval(timerRef.current);
			timerRef.current = null;
		}

		if (!enabled || stableAgentIds.length === 0) {
			console.log(
				"[AgentMetadataHealthBanner] Skipping: enabled=",
				enabled,
				"agentIds.length=",
				stableAgentIds.length,
			);
			return;
		}

		console.log(
			"[AgentMetadataHealthBanner] Starting monitoring for agents:",
			stableAgentIds,
		);

		const sources = stableAgentIds.map((agentId) => {
			const source = watchAgentMetadata(agentId);

			source.addEventListener("data", (e) => {
				const data = JSON.parse(e.data) as WorkspaceAgentMetadata[];

				if (isValidAgentMetadataSample(data)) {
					console.log(
						"[AgentMetadataHealthBanner] Valid sample for agent:",
						agentId,
					);
					hasValidAgentRef.current = true;
					invalidSinceByAgentRef.current.delete(agentId);
					setShouldShow(false);
					return;
				}

				if (isInvalidAgentMetadataSample(data)) {
					const now = Date.now();
					if (!invalidSinceByAgentRef.current.has(agentId)) {
						console.log(
							"[AgentMetadataHealthBanner] Invalid sample detected for agent:",
							agentId,
							"at",
							now,
						);
						invalidSinceByAgentRef.current.set(agentId, now);
					}
				}
			});

			// If the stream errors, we don't want to show a misleading scheduling banner.
			source.onerror = () => {
				source.close();
				invalidSinceByAgentRef.current.delete(agentId);
			};

			return source;
		});

		timerRef.current = window.setInterval(() => {
			if (hasValidAgentRef.current) {
				setShouldShow(false);
				return;
			}
			const now = Date.now();
			// Option A: hide if any agent becomes valid (handled above).
			// Show if *all* agents have been continuously invalid for at least THRESHOLD_MS.
			const invalidSinceByAgent = invalidSinceByAgentRef.current;
			const allInvalidLongEnough = stableAgentIds.every((id) => {
				const invalidSince = invalidSinceByAgent.get(id);
				return invalidSince !== undefined && now - invalidSince >= THRESHOLD_MS;
			});
			if (allInvalidLongEnough !== shouldShow) {
				console.log(
					"[AgentMetadataHealthBanner] Banner state changed:",
					allInvalidLongEnough,
					"invalidSinceByAgent:",
					Object.fromEntries(invalidSinceByAgent),
				);
			}
			setShouldShow(allInvalidLongEnough);
		}, 1000);

		return () => {
			for (const source of sources) {
				source.close();
			}
			if (timerRef.current !== null) {
				window.clearInterval(timerRef.current);
				timerRef.current = null;
			}
		};
	}, [enabled, agentIdsKey, stableAgentIds]);

	return { shouldShow };
};


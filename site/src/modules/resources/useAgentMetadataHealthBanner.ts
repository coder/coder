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

const THRESHOLD_MS = 10_000;

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
	const sourcesRef = useRef<EventSource[]>([]);

	useEffect(() => {
		// Only reset if agent IDs actually changed
		const agentIdsChanged = previousAgentIdsRef.current !== agentIdsKey;
		if (agentIdsChanged) {
			// Clear existing timer and sources when agent IDs change
			if (timerRef.current !== null) {
				window.clearInterval(timerRef.current);
				timerRef.current = null;
			}
			invalidSinceByAgentRef.current = new Map();
			hasValidAgentRef.current = false;
			setShouldShow(false);
			previousAgentIdsRef.current = agentIdsKey;
		}

		if (!enabled || stableAgentIds.length === 0) {
			return;
		}

		// If agent IDs haven't changed and timer is already running, don't recreate
		if (!agentIdsChanged && timerRef.current !== null) {
			return;
		}

		// Close existing sources if agent IDs changed
		if (agentIdsChanged) {
			for (const source of sourcesRef.current) {
				source.close();
			}
			sourcesRef.current = [];
		}

		const sources = stableAgentIds.map((agentId) => {
			const source = watchAgentMetadata(agentId);

			source.addEventListener("data", (e) => {
				const data = JSON.parse(e.data) as WorkspaceAgentMetadata[];

				if (isValidAgentMetadataSample(data)) {
					hasValidAgentRef.current = true;
					invalidSinceByAgentRef.current.delete(agentId);
					setShouldShow(false);
					return;
				}

				if (isInvalidAgentMetadataSample(data)) {
					const now = Date.now();
					if (!invalidSinceByAgentRef.current.has(agentId)) {
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

		sourcesRef.current = sources;

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
			setShouldShow(allInvalidLongEnough);
		}, 1000);

		return () => {
			for (const source of sourcesRef.current) {
				source.close();
			}
			sourcesRef.current = [];
			if (timerRef.current !== null) {
				window.clearInterval(timerRef.current);
				timerRef.current = null;
			}
		};
	}, [enabled, agentIdsKey, stableAgentIds]);

	return { shouldShow };
};


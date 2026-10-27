import { useEffect, useRef } from "react";
import { useQuery } from "react-query";
import { chatArtifact } from "#/api/queries/chatArtifacts";

/**
 * Loads an artifact. When a tool result in the chat names a newer version
 * than the cache, the chat published again, so the hook refetches once for
 * each such version while it is enabled.
 */
export const useChatArtifact = (
	artifactId: string,
	seenVersion: number,
	{ enabled = true }: { enabled?: boolean } = {},
) => {
	const artifactQuery = useQuery({ ...chatArtifact(artifactId), enabled });
	const { data, refetch } = artifactQuery;
	const refetchedForVersionRef = useRef(0);

	useEffect(() => {
		if (
			enabled &&
			data &&
			seenVersion > data.latest_version &&
			refetchedForVersionRef.current < seenVersion
		) {
			refetchedForVersionRef.current = seenVersion;
			void refetch();
		}
	}, [enabled, data, seenVersion, refetch]);

	return artifactQuery;
};

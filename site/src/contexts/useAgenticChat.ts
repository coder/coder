import { experiments } from "api/queries/experiments";

import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useQuery } from "react-query";

interface AgenticChat {
	readonly enabled: boolean;
}

export const useAgenticChat = (): AgenticChat => {
	const { metadata } = useEmbeddedMetadata();
	const enabledExperimentsQuery = useQuery(experiments(metadata.experiments));
	return {
		enabled: enabledExperimentsQuery.data?.includes("agentic-chat") ?? false,
	};
};

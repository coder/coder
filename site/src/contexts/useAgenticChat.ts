import { experiments } from "api/queries/experiments";

import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useEffect, useState } from "react";
import { useQuery } from "react-query";

interface AgenticChat {
	readonly enabled: boolean;
}

export const useAgenticChat = (): AgenticChat => {
	const { metadata } = useEmbeddedMetadata();
	const enabledExperimentsQuery = useQuery(experiments(metadata.experiments));

	const [enabled, setEnabled] = useState<boolean>(false);
	useEffect(() => {
		if (enabledExperimentsQuery.data?.includes("agentic-chat")) {
			setEnabled(true);
		}
	});

	return { enabled };
};

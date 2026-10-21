import { useQuery } from "react-query";
import { userCompactionStrategy } from "#/api/queries/chats";
import type { ChatCompactionStrategy } from "#/api/typesGenerated";
import { useDashboard } from "#/modules/dashboard/useDashboard";

/**
 * The strategy is `summarize` while the chat-token-budget experiment is off,
 * because chatd ignores the saved choice then, and `undefined` until the
 * saved choice loads or after it fails to load.
 */
export const useUserCompactionStrategy = () => {
	const { experiments } = useDashboard();
	const isExperimentEnabled = experiments.includes("chat-token-budget");
	const query = useQuery({
		...userCompactionStrategy(),
		enabled: isExperimentEnabled,
	});
	const strategy: ChatCompactionStrategy | undefined = isExperimentEnabled
		? query.data?.strategy
		: "summarize";
	return { isExperimentEnabled, strategy, query };
};

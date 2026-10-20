import { useQuery } from "react-query";
import { userCompactionStrategy } from "#/api/queries/chats";
import type { ChatCompactionStrategy } from "#/api/typesGenerated";
import { useDashboard } from "#/modules/dashboard/useDashboard";

/**
 * Resolves the compaction strategy chatd applies to the current user's
 * chats. The strategy is `summarize` while the chat-token-budget experiment
 * is off, and `undefined` until the user's saved choice has loaded.
 */
export const useUserCompactionStrategy = () => {
	const { experiments } = useDashboard();
	const isAvailable = experiments.includes("chat-token-budget");
	const query = useQuery({
		...userCompactionStrategy(),
		enabled: isAvailable,
	});
	const strategy: ChatCompactionStrategy | undefined = isAvailable
		? query.data?.strategy
		: "summarize";
	return { isAvailable, strategy, query };
};

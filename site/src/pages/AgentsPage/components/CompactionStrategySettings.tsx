import { useMutation, useQueryClient } from "react-query";
import { getErrorMessage } from "#/api/errors";
import { updateUserCompactionStrategy } from "#/api/queries/chats";
import {
	ChatCompactionStrategies,
	type ChatCompactionStrategy,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { useUserCompactionStrategy } from "../hooks/useUserCompactionStrategy";
import { SettingsSelectRow } from "./DisplayModeSettings";

const strategyLabels: Record<ChatCompactionStrategy, string> = {
	summarize: "Summarize",
	token_budget: "Token budget",
};

const strategyOptions = ChatCompactionStrategies.map((value) => ({
	value,
	label: strategyLabels[value],
}));

/**
 * Lets the user choose how chatd keeps long chats within the context
 * window. Renders nothing while the chat-token-budget experiment is off.
 */
export const CompactionStrategySettings: React.FC = () => {
	const queryClient = useQueryClient();
	const { isAvailable, query } = useUserCompactionStrategy();
	const mutation = useMutation(updateUserCompactionStrategy(queryClient));

	if (!isAvailable) {
		return null;
	}

	return (
		<SettingsSelectRow
			title="Compaction strategy"
			description="How long conversations stay within the model's context window. 'Summarize' replaces older history with a summary at the compaction threshold. 'Token budget' asks the agent to save a checkpoint at the handoff target, then continues in a fresh context window that starts from that checkpoint."
			ariaLabel="Compaction strategy"
			value={query.data?.strategy}
			placeholder={query.error != null ? "Unavailable" : "Loading…"}
			disabled={!query.data || mutation.isPending}
			options={strategyOptions}
			onValueChange={(strategy) => mutation.mutate({ strategy })}
			alert={query.error != null && <ErrorAlert error={query.error} />}
			errorMessage={
				mutation.isError
					? getErrorMessage(
							mutation.error,
							"Failed to save your compaction strategy.",
						)
					: undefined
			}
		/>
	);
};

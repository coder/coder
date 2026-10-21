import { useMutation, useQueryClient } from "react-query";
import { getErrorMessage } from "#/api/errors";
import { updateUserCompactionStrategy } from "#/api/queries/chats";
import {
	ChatCompactionStrategies,
	type ChatCompactionStrategy,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { useUserCompactionStrategy } from "../hooks/useUserCompactionStrategy";
import { SettingsSelectRow } from "./SettingsSelectRow";

const strategyLabels = {
	summarize: "Summarize",
	token_budget: "Token budget",
} satisfies Record<ChatCompactionStrategy, string>;

/**
 * Renders nothing unless the chat-token-budget experiment is on. The choice
 * is saved per user, and chatd applies it to every chat the user owns.
 */
export const CompactionStrategySettings: React.FC = () => {
	const queryClient = useQueryClient();
	const { isExperimentEnabled, query } = useUserCompactionStrategy();
	const mutation = useMutation(updateUserCompactionStrategy(queryClient));

	if (!isExperimentEnabled) {
		return null;
	}

	return (
		<SettingsSelectRow
			title="Compaction strategy"
			description="How conversations stay within the model's context window. 'Summarize' replaces older history with a summary at the compaction threshold. 'Token budget' asks the agent to save a checkpoint at the handoff target, then continues in a fresh context window that starts from that checkpoint."
			ariaLabel="Compaction strategy"
			value={
				mutation.isPending ? mutation.variables.strategy : query.data?.strategy
			}
			placeholder={query.error != null ? "Unavailable" : "Loading…"}
			disabled={!query.data || mutation.isPending}
			options={ChatCompactionStrategies.map((value) => ({
				value,
				label: strategyLabels[value],
			}))}
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

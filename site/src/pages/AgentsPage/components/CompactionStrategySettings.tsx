import { useMutation, useQuery, useQueryClient } from "react-query";
import { getErrorMessage } from "#/api/errors";
import {
	updateUserCompactionStrategy,
	userCompactionStrategy,
} from "#/api/queries/chats";
import {
	ChatCompactionStrategies,
	type ChatCompactionStrategy,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";

const strategyLabels: Record<ChatCompactionStrategy, string> = {
	summarize: "Summarize",
	token_budget: "Token budget",
};

/**
 * Lets the user choose how chatd keeps long chats within the context
 * window. Render it only when the chat-token-budget experiment is enabled.
 */
export const CompactionStrategySettings: React.FC = () => {
	const queryClient = useQueryClient();
	const query = useQuery(userCompactionStrategy());
	const mutation = useMutation(updateUserCompactionStrategy(queryClient));

	return (
		<div className="flex flex-col gap-2">
			<h3 className="m-0 text-sm font-semibold text-content-primary">
				Compaction strategy
			</h3>
			{query.error != null && <ErrorAlert error={query.error} />}
			<div className="flex items-center justify-between gap-4">
				<p className="m-0 flex-1 text-xs text-content-secondary">
					How long conversations stay within the model&apos;s context window.
					&apos;Summarize&apos; replaces older history with a summary at the
					compaction threshold. &apos;Token budget&apos; asks the agent to save
					a checkpoint at the handoff target, then continues in a fresh context
					window that starts from that checkpoint.
				</p>
				<Select
					value={query.data?.strategy ?? ""}
					disabled={!query.data || mutation.isPending}
					onValueChange={(value: string) => {
						const strategy = ChatCompactionStrategies.find(
							(candidate) => candidate === value,
						);
						if (!strategy) return;
						mutation.mutate({ strategy });
					}}
				>
					<SelectTrigger
						className="w-44 shrink-0"
						aria-label="Compaction strategy"
					>
						<SelectValue
							placeholder={query.error != null ? "Unavailable" : "Loading…"}
						/>
					</SelectTrigger>
					<SelectContent>
						{ChatCompactionStrategies.map((strategy) => (
							<SelectItem key={strategy} value={strategy}>
								{strategyLabels[strategy]}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			</div>
			{mutation.isError && (
				<p role="alert" className="m-0 text-xs text-content-destructive">
					{getErrorMessage(
						mutation.error,
						"Failed to save your compaction strategy.",
					)}
				</p>
			)}
		</div>
	);
};

import { chatCostSummary } from "api/queries/chats";
import { Button } from "components/Button/Button";
import dayjs from "dayjs";
import { Loader2Icon, TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import { useQuery } from "react-query";
import { formatCostMicros, formatTokenCount } from "utils/analytics";

const AgentsAnalyticsPage: FC = () => {
	const end = dayjs();
	const start = dayjs().subtract(30, "day");
	const summaryQuery = useQuery(
		chatCostSummary({
			start_date: start.toISOString(),
			end_date: end.toISOString(),
		}),
	);

	const header = (
		<div>
			<h1 className="text-2xl font-semibold text-content-primary">Analytics</h1>
			<p className="text-sm text-content-secondary">
				{start.format("MMM D")} – {end.format("MMM D, YYYY")}
			</p>
		</div>
	);

	if (summaryQuery.isLoading) {
		return (
			<div className="flex flex-1 flex-col overflow-y-auto gap-6 p-6">
				{header}
				<div
					role="status"
					aria-label="Loading analytics"
					className="flex flex-1 items-center justify-center"
				>
					<Loader2Icon className="h-8 w-8 animate-spin text-content-secondary" />
				</div>
			</div>
		);
	}

	if (summaryQuery.isError) {
		return (
			<div className="flex flex-1 flex-col overflow-y-auto gap-6 p-6">
				{header}
				<div className="flex flex-1 flex-col items-center justify-center gap-4 text-center">
					<p className="text-sm text-content-secondary">
						{summaryQuery.error instanceof Error
							? summaryQuery.error.message
							: "Failed to load analytics."}
					</p>
					<Button variant="outline" onClick={() => void summaryQuery.refetch()}>
						Retry
					</Button>
				</div>
			</div>
		);
	}

	const summary = summaryQuery.data;
	if (!summary) {
		return null;
	}

	return (
		<div className="flex flex-1 flex-col overflow-y-auto gap-6 p-6">
			{header}
			<div className="grid grid-cols-2 gap-4 md:grid-cols-4">
				<div className="rounded-lg border border-border-default bg-surface-secondary p-4">
					<p className="text-xs font-medium uppercase tracking-wide text-content-secondary">
						Total Cost
					</p>
					<p className="mt-1 text-2xl font-semibold text-content-primary">
						{formatCostMicros(summary.total_cost_micros)}
					</p>
				</div>
				<div className="rounded-lg border border-border-default bg-surface-secondary p-4">
					<p className="text-xs font-medium uppercase tracking-wide text-content-secondary">
						Input Tokens
					</p>
					<p className="mt-1 text-2xl font-semibold text-content-primary">
						{formatTokenCount(summary.total_input_tokens)}
					</p>
				</div>
				<div className="rounded-lg border border-border-default bg-surface-secondary p-4">
					<p className="text-xs font-medium uppercase tracking-wide text-content-secondary">
						Output Tokens
					</p>
					<p className="mt-1 text-2xl font-semibold text-content-primary">
						{formatTokenCount(summary.total_output_tokens)}
					</p>
				</div>
				<div className="rounded-lg border border-border-default bg-surface-secondary p-4">
					<p className="text-xs font-medium uppercase tracking-wide text-content-secondary">
						Messages
					</p>
					<p className="mt-1 text-2xl font-semibold text-content-primary">
						{(
							summary.priced_message_count + summary.unpriced_message_count
						).toLocaleString()}
					</p>
				</div>
			</div>

			{summary.unpriced_message_count > 0 && (
				<div
					data-testid="unpriced-banner"
					className="flex items-start gap-3 rounded-lg border border-border-warning bg-surface-warning p-4 text-sm text-content-primary"
				>
					<TriangleAlertIcon className="h-5 w-5 shrink-0 text-content-warning" />
					<span>
						{summary.unpriced_message_count} message
						{summary.unpriced_message_count === 1 ? "" : "s"} could not be
						priced because model pricing data was unavailable.
					</span>
				</div>
			)}

			{summary.by_model.length === 0 && summary.by_chat.length === 0 ? (
				<p className="py-12 text-center text-content-secondary">
					No usage data for this period.
				</p>
			) : (
				<>
					<div className="overflow-x-auto" data-testid="model-breakdown">
						<table className="w-full text-sm">
							<thead>
								<tr className="text-left text-xs font-medium uppercase tracking-wide text-content-secondary">
									<th className="pb-2">Model</th>
									<th className="pb-2">Provider</th>
									<th className="pb-2 text-right">Cost</th>
									<th className="pb-2 text-right">Messages</th>
									<th className="pb-2 text-right">Input</th>
									<th className="pb-2 text-right">Output</th>
								</tr>
							</thead>
							<tbody>
								{summary.by_model.map((model) => (
									<tr
										key={model.model_config_id}
										className="border-t border-border-default"
									>
										<td className="py-2">{model.display_name}</td>
										<td className="py-2 text-content-secondary">
											{model.provider}
										</td>
										<td className="py-2 text-right">
											{formatCostMicros(model.total_cost_micros)}
										</td>
										<td className="py-2 text-right">
											{model.message_count.toLocaleString()}
										</td>
										<td className="py-2 text-right">
											{formatTokenCount(model.total_input_tokens)}
										</td>
										<td className="py-2 text-right">
											{formatTokenCount(model.total_output_tokens)}
										</td>
									</tr>
								))}
							</tbody>
						</table>
					</div>

					<div className="overflow-x-auto" data-testid="chat-breakdown">
						<table className="w-full text-sm">
							<thead>
								<tr className="text-left text-xs font-medium uppercase tracking-wide text-content-secondary">
									<th className="pb-2">Chat</th>
									<th className="pb-2 text-right">Cost</th>
									<th className="pb-2 text-right">Messages</th>
									<th className="pb-2 text-right">Input</th>
									<th className="pb-2 text-right">Output</th>
								</tr>
							</thead>
							<tbody>
								{summary.by_chat.map((chat) => (
									<tr
										key={chat.root_chat_id}
										className="border-t border-border-default"
									>
										<td className="py-2">{chat.chat_title}</td>
										<td className="py-2 text-right">
											{formatCostMicros(chat.total_cost_micros)}
										</td>
										<td className="py-2 text-right">
											{chat.message_count.toLocaleString()}
										</td>
										<td className="py-2 text-right">
											{formatTokenCount(chat.total_input_tokens)}
										</td>
										<td className="py-2 text-right">
											{formatTokenCount(chat.total_output_tokens)}
										</td>
									</tr>
								))}
							</tbody>
						</table>
					</div>
				</>
			)}
		</div>
	);
};

export default AgentsAnalyticsPage;

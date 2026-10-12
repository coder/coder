import { useQuery } from "react-query";
import { webhookPublishEndpoint } from "#/api/queries/chatAutomations";
import { chatModels } from "#/api/queries/chats";
import type { ChatAutomation } from "#/api/typesGenerated";
import { CodeExample } from "#/components/CodeExample/CodeExample";
import { formatReasoningEffort } from "#/modules/aiModels/helpers";
import { formatDate } from "#/utils/time";
import { describeSchedule } from "./AutomationScheduleFields";
import { ChatTitleLink } from "./AutomationStatus";

type AutomationSummaryProps = {
	automation: ChatAutomation;
	origin: string;
};

type SummaryItemProps = {
	label: string;
	children: React.ReactNode;
};

const SummaryItem: React.FC<SummaryItemProps> = ({ label, children }) => (
	<div className="flex flex-col gap-1 sm:grid sm:grid-cols-[10rem_1fr] sm:gap-4">
		<dt className="text-sm font-medium text-content-secondary">{label}</dt>
		<dd className="m-0 min-w-0 text-sm text-content-primary wrap-anywhere">
			{children}
		</dd>
	</div>
);

/**
 * Shows an automation to a viewer who cannot edit it. Text instead of
 * disabled inputs keeps help, links and Copy buttons usable.
 */
export const AutomationSummary: React.FC<AutomationSummaryProps> = ({
	automation,
	origin,
}) => {
	const modelsQuery = useQuery({
		...chatModels(automation.organization_id),
		enabled: automation.target_mode === "new_chat",
	});
	const model = modelsQuery.data?.models.find(
		({ id }) => id === automation.new_chat_model_config_id,
	);
	return (
		<dl className="m-0 flex flex-col gap-4 rounded-lg border border-solid border-border p-4 sm:p-6">
			<SummaryItem label="Prompt">
				<span className="whitespace-pre-wrap">{automation.prompt}</span>
			</SummaryItem>
			{automation.kind === "schedule" ? (
				<SummaryItem label="Schedule">
					{describeSchedule(automation.schedule_cron ?? "")}
					<span className="text-content-secondary">
						{" "}
						· {automation.schedule_time_zone || "UTC"} ·{" "}
						<code>{automation.schedule_cron}</code>
					</span>
				</SummaryItem>
			) : (
				<>
					<SummaryItem label="Webhook">
						{automation.webhook_use === "single" ? "Single-use" : "Multi-use"}
						{automation.webhook_consumed_at &&
							`, used on ${formatDate(
								new Date(automation.webhook_consumed_at),
								{
									locale: "en-US",
									timeZoneName: "short",
								},
							)}`}
					</SummaryItem>
					<SummaryItem label="Publish endpoint">
						<CodeExample
							secret={false}
							code={webhookPublishEndpoint(origin, automation.id)}
							copyLabel="Copy endpoint"
						/>
					</SummaryItem>
				</>
			)}
			{automation.target_mode === "existing_chat" ? (
				<>
					<SummaryItem label="Runs in">
						{automation.target_chat ? (
							<ChatTitleLink
								chatId={automation.target_chat.id}
								title={automation.target_chat.title}
								size="sm"
							/>
						) : (
							"A chat of the owner that you cannot open"
						)}
					</SummaryItem>
					<SummaryItem label="When busy">
						{automation.when_busy === "queue"
							? "Queue the prompt"
							: "Skip the run"}
					</SummaryItem>
				</>
			) : (
				<>
					<SummaryItem label="Runs in">A new chat each run</SummaryItem>
					<SummaryItem label="Model">
						{model?.display_name || model?.model || "Set by the owner"}
					</SummaryItem>
					<SummaryItem label="Reasoning effort">
						{automation.reasoning_effort
							? formatReasoningEffort(automation.reasoning_effort)
							: "Model default"}
					</SummaryItem>
				</>
			)}
			{automation.created_by_chat && (
				<SummaryItem label="Created by">
					The agent in{" "}
					<ChatTitleLink
						chatId={automation.created_by_chat.id}
						title={automation.created_by_chat.title}
						size="sm"
						className="inline"
					/>
				</SummaryItem>
			)}
		</dl>
	);
};

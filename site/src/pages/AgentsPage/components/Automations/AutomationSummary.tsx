import { cn } from "cn";
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
	/** Spans every column of the card's grid. */
	wide?: boolean;
};

const SummaryItem: React.FC<SummaryItemProps> = ({ label, children, wide }) => (
	<div className={cn("flex min-w-0 flex-col gap-1", wide && "sm:col-span-3")}>
		<dt className="text-xs font-medium text-content-secondary">{label}</dt>
		<dd className="m-0 min-w-0 text-sm text-content-primary wrap-anywhere">
			{children}
		</dd>
	</div>
);

type SummarySectionProps = {
	title: string;
	children: React.ReactNode;
};

/** A section like the editor's: a heading over one bordered card. */
const SummarySection: React.FC<SummarySectionProps> = ({ title, children }) => (
	<section className="flex flex-col gap-3">
		<h3 className="m-0 text-sm font-medium text-content-primary">{title}</h3>
		<dl className="m-0 grid grid-cols-1 gap-4 rounded-lg border border-solid border-border p-4 sm:grid-cols-3">
			{children}
		</dl>
	</section>
);

/**
 * Shows an automation to a viewer who cannot edit it, in the editor's
 * sections. Text instead of disabled inputs keeps help, links and Copy
 * buttons usable.
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
		<div className="flex flex-col gap-6">
			<section className="flex flex-col gap-3">
				<h3 className="m-0 text-sm font-medium text-content-primary">Prompt</h3>
				<p className="m-0 whitespace-pre-wrap rounded-lg border border-solid border-border p-4 text-sm text-content-primary wrap-anywhere">
					{automation.prompt}
				</p>
			</section>
			<SummarySection title="Trigger">
				{automation.kind === "schedule" ? (
					<>
						<SummaryItem label="Schedule">
							{describeSchedule(automation.schedule_cron ?? "")}
						</SummaryItem>
						<SummaryItem label="Time zone">
							{automation.schedule_time_zone || "UTC"}
						</SummaryItem>
						<SummaryItem label="Cron expression">
							<code className="font-mono text-xs">
								{automation.schedule_cron}
							</code>
						</SummaryItem>
					</>
				) : (
					<>
						<SummaryItem label="Webhook" wide>
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
						<SummaryItem label="Publish endpoint" wide>
							<CodeExample
								secret={false}
								code={webhookPublishEndpoint(origin, automation.id)}
								copyLabel="Copy endpoint"
							/>
						</SummaryItem>
					</>
				)}
			</SummarySection>
			<SummarySection title="Runs in">
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
					<SummaryItem label="Created by" wide>
						The agent in{" "}
						<ChatTitleLink
							chatId={automation.created_by_chat.id}
							title={automation.created_by_chat.title}
							size="sm"
							className="inline"
						/>
					</SummaryItem>
				)}
			</SummarySection>
		</div>
	);
};

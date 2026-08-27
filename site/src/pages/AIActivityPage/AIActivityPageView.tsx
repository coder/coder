import type { FC } from "react";
import type {
	AIAuditAgent,
	AIAuditEvent,
	AIAuditEventType,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Input } from "#/components/Input/Input";
import { Margins } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import { timeFrom } from "#/utils/time";

const ALL_ITEMS = "all";

const eventTypeLabels: Record<AIAuditEventType, string> = {
	sandbox_session_started: "Sandbox started",
	sandbox_session_ended: "Sandbox ended",
	egress: "Egress",
	bridge_session_started: "Bridge session",
	tool_call: "Tool call",
	escalation_created: "Escalation created",
	escalation_resolved: "Escalation resolved",
};

const isAIAuditEventType = (value: string): value is AIAuditEventType =>
	value in eventTypeLabels;

const eventBadgeVariant = (
	event: AIAuditEvent,
): "default" | "warning" | "destructive" | "green" | "info" => {
	switch (event.type) {
		case "egress":
			return event.detail.action === "denied" ? "destructive" : "default";
		case "tool_call":
			return event.detail.disposition === "blocked" ||
				event.detail.disposition === "escalated_denied" ||
				event.detail.disposition === "escalated_expired"
				? "destructive"
				: "default";
		case "escalation_created":
			return "warning";
		case "escalation_resolved":
			return event.detail.status === "approved" ? "green" : "destructive";
		default:
			return "info";
	}
};

interface AIActivityPageViewProps {
	events: readonly AIAuditEvent[];
	agents: readonly AIAuditAgent[];
	isLoading: boolean;
	error: unknown;
	referenceDate: Date;
	sponsor: string;
	onSponsorChange: (sponsor: string) => void;
	aiAgentId: string;
	onAIAgentChange: (aiAgentId: string) => void;
	eventType: AIAuditEventType | "";
	onEventTypeChange: (eventType: AIAuditEventType | "") => void;
}

export const AIActivityPageView: FC<AIActivityPageViewProps> = ({
	events,
	agents,
	isLoading,
	error,
	referenceDate,
	sponsor,
	onSponsorChange,
	aiAgentId,
	onAIAgentChange,
	eventType,
	onEventTypeChange,
}) => {
	const agentNames = new Map(
		agents.map((agent) => [agent.user_id, agent.username]),
	);

	return (
		<Margins className="pb-12">
			<PageHeader>
				<PageHeaderTitle>AI Activity</PageHeaderTitle>
				<PageHeaderSubtitle>
					Everything your agentic identities did: egress decisions, AI sessions,
					tool calls, and approvals.
				</PageHeaderSubtitle>
			</PageHeader>

			<div className="flex flex-col gap-4">
				<form
					className="flex flex-wrap items-center gap-2"
					aria-label="Timeline filters"
					onSubmit={(event) => {
						event.preventDefault();
						const data = new FormData(event.currentTarget);
						onSponsorChange(String(data.get("sponsor") ?? "").trim());
					}}
				>
					<Input
						name="sponsor"
						className="w-56"
						defaultValue={sponsor}
						placeholder="Sponsor (requires audit access)"
						aria-label="Sponsor"
					/>
					<Select
						value={aiAgentId || ALL_ITEMS}
						onValueChange={(value) =>
							onAIAgentChange(value === ALL_ITEMS ? "" : value)
						}
					>
						<SelectTrigger className="w-56" aria-label="AI agent">
							<SelectValue placeholder="All agents" />
						</SelectTrigger>
						<SelectContent>
							<SelectItem value={ALL_ITEMS}>All agents</SelectItem>
							{agents.map((agent) => (
								<SelectItem key={agent.user_id} value={agent.user_id}>
									{agent.username}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
					<Select
						value={eventType || ALL_ITEMS}
						onValueChange={(value) =>
							onEventTypeChange(isAIAuditEventType(value) ? value : "")
						}
					>
						<SelectTrigger className="w-56" aria-label="Event type">
							<SelectValue placeholder="All events" />
						</SelectTrigger>
						<SelectContent>
							<SelectItem value={ALL_ITEMS}>All events</SelectItem>
							{Object.entries(eventTypeLabels).map(([value, label]) => (
								<SelectItem key={value} value={value}>
									{label}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
					<button type="submit" className="sr-only">
						Apply filters
					</button>
				</form>

				{Boolean(error) && <ErrorAlert error={error} />}

				{isLoading ? (
					<div
						className="flex min-h-64 items-center justify-center"
						role="status"
						aria-label="Loading AI activity"
					>
						<Spinner loading />
					</div>
				) : error ? null : events.length === 0 ? (
					<EmptyState message="No AI activity recorded" />
				) : (
					<div
						className="flex flex-col gap-2"
						role="list"
						aria-label="AI activity timeline"
					>
						{events.map((event) => (
							<TimelineEvent
								key={`${event.type}-${event.id}-${event.occurred_at}`}
								event={event}
								agentName={agentNames.get(event.ai_agent_id)}
								referenceDate={referenceDate}
							/>
						))}
					</div>
				)}
			</div>
		</Margins>
	);
};

interface TimelineEventProps {
	event: AIAuditEvent;
	agentName?: string;
	referenceDate: Date;
}

const TimelineEvent: FC<TimelineEventProps> = ({
	event,
	agentName,
	referenceDate,
}) => {
	return (
		<article
			className="rounded-lg border border-border-default border-solid bg-surface-primary px-4 py-3"
			role="listitem"
		>
			<div className="flex flex-wrap items-center gap-3">
				<Badge variant={eventBadgeVariant(event)} size="sm">
					{eventTypeLabels[event.type]}
				</Badge>
				<span className="min-w-0 flex-1 break-words font-mono text-sm text-content-primary">
					{event.summary}
				</span>
				<span
					className="shrink-0 text-xs text-content-secondary"
					title={new Date(event.occurred_at).toISOString()}
				>
					{timeFrom(event.occurred_at, referenceDate)}
				</span>
			</div>
			<div className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-content-secondary">
				<span>Agent: {agentName ?? event.ai_agent_id}</span>
				{event.workspace_name ? (
					<span>Workspace: {event.workspace_name}</span>
				) : null}
			</div>
			<details className="mt-2">
				<summary className="cursor-pointer text-xs font-medium text-content-secondary">
					Details
				</summary>
				<pre className="m-0 mt-2 max-w-full overflow-x-auto whitespace-pre-wrap break-words rounded-md bg-surface-secondary p-3 font-mono text-xs text-content-secondary">
					{JSON.stringify(event.detail, null, 2)}
				</pre>
			</details>
		</article>
	);
};

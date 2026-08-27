import type { FC } from "react";
import { useState } from "react";
import { useQuery } from "react-query";
import { aiAuditTrailTimeline } from "#/api/queries/aiAuditTrail";
import type {
	AIAuditTrailEvent,
	AIAuditTrailEventType,
} from "#/api/typesGenerated";
import { AIAuditTrailEventTypes } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
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
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { pageTitle } from "#/utils/page";
import { formatDate } from "#/utils/time";

const eventTypeLabels: Record<AIAuditTrailEventType, string> = {
	ai_agent_lifecycle: "Agent lifecycle",
	authorization_lifecycle: "Authorization",
	credential_lifecycle: "Credential",
	credential_use: "Credential use",
	sandbox_session: "Egress session",
	egress: "Egress",
};

// Journal events carry both dates; show the recording date only when it
// meaningfully trails the effective date (an observed transition recorded
// after the fact).
const recordedLagThresholdMs = 60_000;

const AIActivityPage: FC = () => {
	const [owner, setOwner] = useState("me");
	const [ownerDraft, setOwnerDraft] = useState("me");
	const [typeFilter, setTypeFilter] = useState<AIAuditTrailEventType | "all">(
		"all",
	);

	const timelineQuery = useQuery({
		...aiAuditTrailTimeline(
			owner,
			typeFilter === "all" ? undefined : [typeFilter],
		),
		refetchInterval: 10_000,
	});
	const events = timelineQuery.data?.events;

	return (
		<Margins>
			<title>{pageTitle("AI Activity")}</title>
			<PageHeader>
				<PageHeaderTitle>AI Activity</PageHeaderTitle>
				<PageHeaderSubtitle>
					End-to-end audit trail of what your AI agents are, what authority and
					credentials they hold, and what they did.
				</PageHeaderSubtitle>
			</PageHeader>

			<div className="flex flex-wrap items-end gap-4 pb-4">
				<form
					className="flex items-end gap-2"
					onSubmit={(event) => {
						event.preventDefault();
						setOwner(ownerDraft.trim() || "me");
					}}
				>
					<div className="flex flex-col gap-1">
						<label className="text-xs font-medium" htmlFor="trail-owner">
							Owner
						</label>
						<Input
							id="trail-owner"
							value={ownerDraft}
							onChange={(event) => setOwnerDraft(event.target.value)}
							placeholder="me"
						/>
					</div>
					<Button type="submit" variant="outline">
						Apply
					</Button>
				</form>

				<div className="flex flex-col gap-1">
					<label className="text-xs font-medium" htmlFor="trail-type">
						Event type
					</label>
					<Select
						value={typeFilter}
						onValueChange={(value) =>
							setTypeFilter(value as AIAuditTrailEventType | "all")
						}
					>
						<SelectTrigger id="trail-type" className="w-48">
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							<SelectItem value="all">All types</SelectItem>
							{AIAuditTrailEventTypes.map((eventType) => (
								<SelectItem key={eventType} value={eventType}>
									{eventTypeLabels[eventType]}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				</div>
			</div>

			{timelineQuery.isError ? (
				<ErrorAlert error={timelineQuery.error} />
			) : timelineQuery.isLoading ? (
				<div className="flex justify-center p-8">
					<Spinner loading />
				</div>
			) : !events || events.length === 0 ? (
				<EmptyState
					message="No AI agent activity"
					description="Lifecycle, authorization, credential, and egress records for this owner's AI agents will appear here."
				/>
			) : (
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Time</TableHead>
							<TableHead>Type</TableHead>
							<TableHead>Event</TableHead>
							<TableHead>Agent</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{events.map((event) => (
							<TimelineRow key={event.id} event={event} />
						))}
					</TableBody>
				</Table>
			)}
		</Margins>
	);
};

interface TimelineRowProps {
	event: AIAuditTrailEvent;
}

const TimelineRow: FC<TimelineRowProps> = ({ event }) => {
	const occurred = new Date(event.occurred_at);
	const recorded = new Date(event.recorded_at);
	const recordedLate =
		recorded.getTime() - occurred.getTime() > recordedLagThresholdMs;
	const denied =
		event.type === "egress" && String(event.detail.event) === "denied";

	return (
		<TableRow>
			<TableCell className="whitespace-nowrap">
				<div>{formatDate(occurred)}</div>
				{recordedLate && (
					<div className="text-xs text-content-secondary">
						recorded {formatDate(recorded)}
					</div>
				)}
			</TableCell>
			<TableCell>
				<Badge variant={denied ? "destructive" : "default"}>
					{eventTypeLabels[event.type]}
				</Badge>
			</TableCell>
			<TableCell>{event.summary}</TableCell>
			<TableCell>
				<code className="text-xs">{event.ai_agent_id.slice(0, 8)}</code>
			</TableCell>
		</TableRow>
	);
};

export default AIActivityPage;

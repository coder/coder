import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { aiAuditAgents, aiAuditTimeline } from "#/api/queries/aiAudit";
import type { AIAuditEventType } from "#/api/typesGenerated";
import { pageTitle } from "#/utils/page";
import { AIActivityPageView } from "./AIActivityPageView";

interface AIActivityPageProps {
	referenceDate?: Date;
}

const AIActivityPage: FC<AIActivityPageProps> = ({
	referenceDate = new Date(),
}) => {
	// Empty string means "me" / "all". The sponsor filter requires audit log
	// read permission for other users; the API enforces it, and the error
	// surfaces in the alert below.
	const [sponsor, setSponsor] = useState("");
	const [aiAgentId, setAIAgentId] = useState("");
	const [eventType, setEventType] = useState<AIAuditEventType | "">("");

	const agentsQuery = useQuery(aiAuditAgents(sponsor || undefined));
	const timelineQuery = useQuery({
		...aiAuditTimeline({
			sponsor: sponsor || undefined,
			aiAgentId: aiAgentId || undefined,
			types: eventType ? [eventType] : undefined,
		}),
		refetchInterval: 10_000,
	});

	return (
		<>
			<title>{pageTitle("AI Activity")}</title>
			<AIActivityPageView
				events={timelineQuery.data?.events ?? []}
				agents={agentsQuery.data ?? []}
				isLoading={timelineQuery.isLoading}
				error={timelineQuery.error ?? agentsQuery.error}
				referenceDate={referenceDate}
				sponsor={sponsor}
				onSponsorChange={(value) => {
					setSponsor(value);
					// Agent identities belong to the sponsor being viewed.
					setAIAgentId("");
				}}
				aiAgentId={aiAgentId}
				onAIAgentChange={setAIAgentId}
				eventType={eventType}
				onEventTypeChange={setEventType}
			/>
		</>
	);
};

export default AIActivityPage;

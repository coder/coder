import { API } from "#/api/api";
import type { AIAuditEventType } from "#/api/typesGenerated";

interface AIAuditTimelineParams {
	sponsor?: string;
	aiAgentId?: string;
	types?: readonly AIAuditEventType[];
}

export const aiAuditTimeline = (params: AIAuditTimelineParams) => ({
	queryKey: [
		"aiAudit",
		"timeline",
		params.sponsor ?? "me",
		params.aiAgentId ?? "all",
		params.types?.join(",") ?? "all",
	] as const,
	queryFn: () => API.getAIAuditTimeline(params),
});

export const aiAuditAgents = (sponsor?: string) => ({
	queryKey: ["aiAudit", "agents", sponsor ?? "me"] as const,
	queryFn: () => API.getAIAuditAgents(sponsor),
});

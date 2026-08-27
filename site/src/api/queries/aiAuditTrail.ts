import { API } from "#/api/api";
import type { AIAuditTrailEventType } from "#/api/typesGenerated";

const aiAuditTrailTimelineKey = (
	owner: string,
	types: readonly AIAuditTrailEventType[] | undefined,
) => ["ai-audit-trail", owner, types ?? "all"] as const;

export const aiAuditTrailTimeline = (
	owner: string,
	types: readonly AIAuditTrailEventType[] | undefined,
) => {
	return {
		queryKey: aiAuditTrailTimelineKey(owner, types),
		queryFn: () => API.getAIAuditTrailTimeline({ owner, types }),
	};
};

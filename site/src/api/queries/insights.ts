import { API, type InsightsParams, type InsightsTemplateParams } from "api/api";
import type { GetUserStatusCountsResponse } from "api/typesGenerated";
import type { UseQueryOptions } from "react-query";

export const insightsTemplate = (params: InsightsTemplateParams) => {
	return {
		queryKey: ["insights", "templates", params.template_ids, params],
		queryFn: () => API.getInsightsTemplate(params),
	};
};

export const insightsUserLatency = (params: InsightsParams) => {
	return {
		queryKey: ["insights", "userLatency", params.template_ids, params],
		queryFn: () => API.getInsightsUserLatency(params),
	};
};

export const insightsUserActivity = (params: InsightsParams) => {
	return {
		queryKey: ["insights", "userActivity", params.template_ids, params],
		queryFn: () => API.getInsightsUserActivity(params),
	};
};

export const insightsUserStatusCounts = () => {
	return {
		queryKey: ["insights", "userStatusCounts"],
		queryFn: () => API.getInsightsUserStatusCounts(),
		select: (data) => data.status_counts,
	} satisfies UseQueryOptions<
		GetUserStatusCountsResponse,
		unknown,
		GetUserStatusCountsResponse["status_counts"]
	>;
};

import { type InsightsParams, type InsightsTemplateParams, API } from "api/api";

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

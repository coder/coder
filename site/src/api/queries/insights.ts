import {
  type InsightsParams,
  type InsightsTemplateParams,
  client,
} from "api/api";

export const insightsTemplate = (params: InsightsTemplateParams) => {
  return {
    queryKey: ["insights", "templates", params.template_ids, params],
    queryFn: () => client.api.getInsightsTemplate(params),
  };
};

export const insightsUserLatency = (params: InsightsParams) => {
  return {
    queryKey: ["insights", "userLatency", params.template_ids, params],
    queryFn: () => client.api.getInsightsUserLatency(params),
  };
};

export const insightsUserActivity = (params: InsightsParams) => {
  return {
    queryKey: ["insights", "userActivity", params.template_ids, params],
    queryFn: () => client.api.getInsightsUserActivity(params),
  };
};

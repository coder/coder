import * as API from "api/api";

export const insightsTemplate = (params: API.InsightsTemplateParams) => {
  return {
    queryKey: ["insights", "templates", params.template_ids, params],
    queryFn: () => API.getInsightsTemplate(params),
  };
};

export const insightsUserLatency = (params: API.InsightsParams) => {
  return {
    queryKey: ["insights", "userLatency", params.template_ids, params],
    queryFn: () => API.getInsightsUserLatency(params),
  };
};

export const insightsUserActivity = (params: API.InsightsParams) => {
  return {
    queryKey: ["insights", "userActivity", params.template_ids, params],
    queryFn: () => API.getInsightsUserActivity(params),
  };
};

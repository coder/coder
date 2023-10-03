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

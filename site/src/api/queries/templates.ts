import * as API from "api/api";

const getTemplatesQueryKey = (orgId: string) => [orgId, "templates"];

export const templates = (orgId: string) => {
  return {
    queryKey: getTemplatesQueryKey(orgId),
    queryFn: () => API.getTemplates(orgId),
  };
};

export const templateExamples = (orgId: string) => {
  return {
    queryKey: [...getTemplatesQueryKey(orgId), "examples"],
    queryFn: () => API.getTemplateExamples(orgId),
  };
};

import { useQuery } from "@tanstack/react-query";
import * as API from "api/api";

const getTemplatesQueryKey = (orgId: string) => [orgId, "templates"];

export const useTemplates = (orgId: string) => {
  return useQuery({
    queryKey: getTemplatesQueryKey(orgId),
    queryFn: () => API.getTemplates(orgId),
  });
};

export const useTemplateExamples = (
  orgId: string,
  { enabled }: { enabled: boolean },
) => {
  return useQuery({
    queryKey: [...getTemplatesQueryKey(orgId), "examples"],
    queryFn: () => API.getTemplateExamples(orgId),
    enabled,
  });
};

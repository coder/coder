import * as API from "api/api";
import { type Template, type AuthorizationResponse } from "api/typesGenerated";
import { type QueryOptions } from "@tanstack/react-query";

export const templateByNameKey = (orgId: string, name: string) => [
  orgId,
  "template",
  name,
  "settings",
];

export const templateByName = (
  orgId: string,
  name: string,
): QueryOptions<{ template: Template; permissions: AuthorizationResponse }> => {
  return {
    queryKey: templateByNameKey(orgId, name),
    queryFn: async () => {
      const template = await API.getTemplateByName(orgId, name);
      const permissions = await API.checkAuthorization({
        checks: {
          canUpdateTemplate: {
            object: {
              resource_type: "template",
              resource_id: template.id,
            },
            action: "update",
          },
        },
      });

      return { template, permissions };
    },
  };
};

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

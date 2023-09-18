import * as API from "api/api";
import {
  type Template,
  type AuthorizationResponse,
  type CreateTemplateVersionRequest,
} from "api/typesGenerated";
import { type QueryClient, type QueryOptions } from "@tanstack/react-query";

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

export const templateVersion = (versionId: string) => {
  return {
    queryKey: ["templateVersion", versionId],
    queryFn: () => API.getTemplateVersion(versionId),
  };
};

export const templateVersions = (templateId: string) => {
  return {
    queryKey: ["templateVersions", templateId],
    queryFn: () => API.getTemplateVersions(templateId),
  };
};

export const templateVersionVariables = (versionId: string) => {
  return {
    queryKey: ["templateVersionVariables", versionId],
    queryFn: () => API.getTemplateVersionVariables(versionId),
  };
};

export const createTemplateVersion = (
  orgId: string,
  queryClient: QueryClient,
) => {
  return {
    mutationFn: (request: CreateTemplateVersionRequest) =>
      API.createTemplateVersion(orgId, request),
  };
};

export const updateActiveTemplateVersion = (
  template: Template,
  queryClient: QueryClient,
) => {
  return {
    mutationFn: (versionId: string) =>
      API.updateActiveTemplateVersion(template.id, {
        id: versionId,
      }),
    onSuccess: async () => {
      // invalidated because of `active_version_id`
      await queryClient.invalidateQueries(
        templateByNameKey(template.organization_id, template.name),
      );
    },
  };
};

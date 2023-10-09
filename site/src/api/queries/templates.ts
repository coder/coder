import * as API from "api/api";
import {
  type Template,
  type AuthorizationResponse,
  type CreateTemplateVersionRequest,
  type ProvisionerJobStatus,
  type TemplateVersion,
} from "api/typesGenerated";
import { type QueryClient, type QueryOptions } from "react-query";
import { delay } from "utils/delay";

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
    queryKey: ["templateVersion", versionId, "variables"],
    queryFn: () => API.getTemplateVersionVariables(versionId),
  };
};

export const createAndBuildTemplateVersion = (orgId: string) => {
  return {
    mutationFn: async (
      request: CreateTemplateVersionRequest,
    ): Promise<string> => {
      const newVersion = await API.createTemplateVersion(orgId, request);

      let data: TemplateVersion;
      let jobStatus: ProvisionerJobStatus;
      do {
        await delay(1000);
        data = await API.getTemplateVersion(newVersion.id);
        jobStatus = data.job.status;

        if (jobStatus === "succeeded") {
          return newVersion.id;
        }
      } while (jobStatus === "pending" || jobStatus === "running");

      // No longer pending/running, but didn't succeed
      throw data.job.error;
    },
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

export const templateVersionExternalAuthKey = (versionId: string) => [
  "templateVersion",
  versionId,
  "externalAuth",
];

export const templateVersionExternalAuth = (versionId: string) => {
  return {
    queryKey: templateVersionExternalAuthKey(versionId),
    queryFn: () => API.getTemplateVersionExternalAuth(versionId),
  };
};

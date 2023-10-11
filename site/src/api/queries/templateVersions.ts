import * as API from "api/api";

export const templateVersionLogs = (versionId: string) => {
  return {
    queryKey: ["templateVersion", versionId, "logs"],
    queryFn: () => API.getTemplateVersionLogs(versionId),
  };
};


export const promoteTemplateVersion = (versionId: string, templateID: string) => {
  return {
    mutationFn: () => API.updateActiveTemplateVersion(templateID, {
      id: versionId
    }),
  };
}

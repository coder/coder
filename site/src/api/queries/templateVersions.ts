import * as API from "api/api";

export const templateVersionLogs = (versionId: string) => {
  return {
    queryKey: ["templateVersion", versionId, "logs"],
    queryFn: () => API.getTemplateVersionLogs(versionId),
  };
};

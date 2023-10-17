import { usePermissions } from "hooks/usePermissions";
import { useOrganizationId } from "hooks/useOrganizationId";
import { useTab } from "hooks/useTab";
import { type FC, useMemo } from "react";
import { Helmet } from "react-helmet-async";
import { useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import TemplateVersionPageView from "./TemplateVersionPageView";
import { useQuery } from "react-query";
import { templateVersionByName } from "api/queries/templates";
import { getTemplateFilesWithDiff } from "utils/templateVersion";

type Params = {
  version: string;
  template: string;
};

export const TemplateVersionPage: FC = () => {
  const { version: versionName, template: templateName } =
    useParams() as Params;
  const orgId = useOrganizationId();
  const templateVersionQuery = useQuery(
    templateVersionByName(orgId, templateName, versionName),
  );
  const filesQuery = useQuery({
    queryKey: ["templateFiles", templateName],
    queryFn: () => {
      if (!templateVersionQuery.data) {
        return;
      }
      return getTemplateFilesWithDiff(templateName, templateVersionQuery.data);
    },
    enabled: templateVersionQuery.isSuccess,
  });
  const tab = useTab("file", "0");
  const permissions = usePermissions();

  const versionId = templateVersionQuery.data?.id;
  const createWorkspaceUrl = useMemo(() => {
    const params = new URLSearchParams();
    if (versionId) {
      params.set("version", versionId);
      return `/templates/${templateName}/workspace?${params.toString()}`;
    }
    return undefined;
  }, [templateName, versionId]);

  return (
    <>
      <Helmet>
        <title>{pageTitle(`Version ${versionName} · ${templateName}`)}</title>
      </Helmet>

      <TemplateVersionPageView
        error={templateVersionQuery.error || filesQuery.error}
        currentVersion={templateVersionQuery.data}
        currentFiles={filesQuery.data?.currentFiles}
        previousFiles={filesQuery.data?.previousFiles}
        versionName={versionName}
        templateName={templateName}
        tab={tab}
        createWorkspaceUrl={
          permissions.updateTemplates ? createWorkspaceUrl : undefined
        }
      />
    </>
  );
};

export default TemplateVersionPage;

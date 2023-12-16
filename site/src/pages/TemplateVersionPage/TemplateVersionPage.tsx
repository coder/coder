import { usePermissions } from "hooks/usePermissions";
import { useOrganizationId } from "hooks/useOrganizationId";
import { type FC, useMemo } from "react";
import { Helmet } from "react-helmet-async";
import { useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import TemplateVersionPageView from "./TemplateVersionPageView";
import { useQuery } from "react-query";
import { templateVersionByName } from "api/queries/templates";
import { useFileTab, useTemplateFiles } from "components/TemplateFiles/hooks";

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
  const { data: templateFiles, error: templateFilesError } = useTemplateFiles(
    templateName,
    templateVersionQuery.data,
  );
  const tab = useFileTab(templateFiles?.currentFiles);
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
        error={templateVersionQuery.error || templateFilesError}
        currentVersion={templateVersionQuery.data}
        currentFiles={templateFiles?.currentFiles}
        previousFiles={templateFiles?.previousFiles}
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

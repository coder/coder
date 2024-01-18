import { usePermissions } from "hooks/usePermissions";
import { useOrganizationId } from "hooks/useOrganizationId";
import { type FC, useMemo } from "react";
import { Helmet } from "react-helmet-async";
import { useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import TemplateVersionPageView from "./TemplateVersionPageView";
import { useQuery } from "react-query";
import {
  templateByName,
  templateFiles,
  templateVersion,
  templateVersionByName,
} from "api/queries/templates";
import { useFileTab } from "components/TemplateFiles/TemplateFiles";

type Params = {
  version: string;
  template: string;
};

export const TemplateVersionPage: FC = () => {
  const { version: versionName, template: templateName } =
    useParams() as Params;
  const orgId = useOrganizationId();

  /**
   * Template version files
   */
  const templateQuery = useQuery(templateByName(orgId, templateName));
  const selectedVersionQuery = useQuery(
    templateVersionByName(orgId, templateName, versionName),
  );
  const selectedVersionFilesQuery = useQuery({
    ...templateFiles(selectedVersionQuery.data?.job.file_id ?? ""),
    enabled: Boolean(selectedVersionQuery.data),
  });
  const activeVersionQuery = useQuery({
    ...templateVersion(templateQuery.data?.active_version_id ?? ""),
    enabled: Boolean(templateQuery.data),
  });
  const activeVersionFilesQuery = useQuery({
    ...templateFiles(activeVersionQuery.data?.job.file_id ?? ""),
    enabled: Boolean(activeVersionQuery.data),
  });
  const tab = useFileTab(selectedVersionFilesQuery.data);

  const permissions = usePermissions();
  const versionId = selectedVersionQuery.data?.id;
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
        error={
          templateQuery.error ||
          selectedVersionQuery.error ||
          selectedVersionFilesQuery.error ||
          activeVersionQuery.error ||
          activeVersionFilesQuery.error
        }
        currentVersion={selectedVersionQuery.data}
        currentFiles={selectedVersionFilesQuery.data}
        baseFiles={activeVersionFilesQuery.data}
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

import { type FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { previousTemplateVersion, templateFiles } from "api/queries/templates";
import { Loader } from "components/Loader/Loader";
import {
  TemplateFiles,
  useFileTab,
} from "modules/templates/TemplateFiles/TemplateFiles";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import { useOrganizationId } from "contexts/auth/useOrganizationId";
import { getTemplatePageTitle } from "../utils";

const TemplateFilesPage: FC = () => {
  const orgId = useOrganizationId();
  const { template, activeVersion } = useTemplateLayoutContext();
  const { data: currentFiles } = useQuery(
    templateFiles(activeVersion.job.file_id),
  );
  const { data: previousTemplate } = useQuery(
    previousTemplateVersion(orgId, template.name, activeVersion.name),
  );
  const { data: previousFiles } = useQuery({
    ...templateFiles(previousTemplate?.job.file_id ?? ""),
    enabled: Boolean(previousTemplate),
  });
  const tab = useFileTab(currentFiles);

  return (
    <>
      <Helmet>
        <title>{getTemplatePageTitle("Source Code", template)}</title>
      </Helmet>

      {previousFiles && currentFiles && tab.isLoaded ? (
        <TemplateFiles
          currentFiles={currentFiles}
          baseFiles={previousFiles}
          tab={tab}
        />
      ) : (
        <Loader />
      )}
    </>
  );
};

export default TemplateFilesPage;

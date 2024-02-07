import { type FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { previousTemplateVersion, templateFiles } from "api/queries/templates";
import { Loader } from "components/Loader/Loader";
import { TemplateFiles } from "modules/templates/TemplateFiles/TemplateFiles";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import { useOrganizationId } from "contexts/auth/useOrganizationId";
import { getTemplatePageTitle } from "../utils";

const TemplateFilesPage: FC = () => {
  const orgId = useOrganizationId();
  const { template, activeVersion } = useTemplateLayoutContext();
  const { data: currentFiles } = useQuery(
    templateFiles(activeVersion.job.file_id),
  );
  const previousVersionQuery = useQuery(
    previousTemplateVersion(orgId, template.name, activeVersion.name),
  );
  const previousVersion = previousVersionQuery.data;
  const hasPreviousVersion =
    previousVersionQuery.isSuccess && previousVersion !== null;
  const { data: previousFiles } = useQuery({
    ...templateFiles(previousVersion?.job.file_id ?? ""),
    enabled: hasPreviousVersion,
  });
  const shouldDisplayFiles =
    currentFiles && (!hasPreviousVersion || previousFiles);

  return (
    <>
      <Helmet>
        <title>{getTemplatePageTitle("Source Code", template)}</title>
      </Helmet>

      {shouldDisplayFiles ? (
        <TemplateFiles
          currentFiles={currentFiles}
          baseFiles={previousFiles}
          versionName={activeVersion.name}
          templateName={template.name}
        />
      ) : (
        <Loader />
      )}
    </>
  );
};

export default TemplateFilesPage;

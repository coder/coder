import { useQuery } from "@tanstack/react-query";
import { getPreviousTemplateVersionByName } from "api/api";
import { TemplateVersion } from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { TemplateFiles } from "components/TemplateFiles/TemplateFiles";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import { useOrganizationId } from "hooks/useOrganizationId";
import { useTab } from "hooks/useTab";
import { FC, useEffect } from "react";
import { Helmet } from "react-helmet-async";
import {
  getTemplateVersionFiles,
  TemplateVersionFiles,
} from "utils/templateVersion";
import { getTemplatePageTitle } from "../utils";

const fetchTemplateFiles = async (
  organizationId: string,
  templateName: string,
  activeVersion: TemplateVersion,
) => {
  const previousVersion = await getPreviousTemplateVersionByName(
    organizationId,
    templateName,
    activeVersion.name,
  );
  const loadFilesPromises: ReturnType<typeof getTemplateVersionFiles>[] = [];
  loadFilesPromises.push(getTemplateVersionFiles(activeVersion));
  if (previousVersion) {
    loadFilesPromises.push(getTemplateVersionFiles(previousVersion));
  }
  const [currentFiles, previousFiles] = await Promise.all(loadFilesPromises);
  return {
    currentFiles,
    previousFiles,
  };
};

const useTemplateFiles = (
  organizationId: string,
  templateName: string,
  activeVersion: TemplateVersion,
) =>
  useQuery({
    queryKey: ["templateFiles", templateName],
    queryFn: () =>
      fetchTemplateFiles(organizationId, templateName, activeVersion),
  });

const useFileTab = (templateFiles: TemplateVersionFiles | undefined) => {
  // Tabs The default tab is the tab that has main.tf but until we loads the
  // files and check if main.tf exists we don't know which tab is the default
  // one so we just use empty string
  const tab = useTab("file", "");
  const isLoaded = tab.value !== "";
  useEffect(() => {
    if (templateFiles && !isLoaded) {
      const terraformFileIndex = Object.keys(templateFiles).indexOf("main.tf");
      // If main.tf exists use the index if not just use the first tab
      tab.set(terraformFileIndex !== -1 ? terraformFileIndex.toString() : "0");
    }
  }, [isLoaded, tab, templateFiles]);

  return {
    ...tab,
    isLoaded,
  };
};

const TemplateFilesPage: FC = () => {
  const { template, activeVersion } = useTemplateLayoutContext();
  const orgId = useOrganizationId();
  const { data: templateFiles } = useTemplateFiles(
    orgId,
    template.name,
    activeVersion,
  );
  const tab = useFileTab(templateFiles?.currentFiles);

  return (
    <>
      <Helmet>
        <title>{getTemplatePageTitle("Source Code", template)}</title>
      </Helmet>

      {templateFiles && tab.isLoaded ? (
        <TemplateFiles
          currentFiles={templateFiles.currentFiles}
          previousFiles={templateFiles.previousFiles}
          tab={tab}
        />
      ) : (
        <Loader />
      )}
    </>
  );
};

export default TemplateFilesPage;

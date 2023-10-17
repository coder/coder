import { useQuery } from "react-query";
import { TemplateVersion } from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { TemplateFiles } from "components/TemplateFiles/TemplateFiles";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import { useTab } from "hooks/useTab";
import { FC, useEffect } from "react";
import { Helmet } from "react-helmet-async";
import {
  getTemplateFilesWithDiff,
  TemplateVersionFiles,
} from "utils/templateVersion";
import { getTemplatePageTitle } from "../utils";

const useTemplateFiles = (
  templateName: string,
  activeVersion: TemplateVersion,
) =>
  useQuery({
    queryKey: ["templateFiles", templateName],
    queryFn: () => getTemplateFilesWithDiff(templateName, activeVersion),
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
  const { data: templateFiles } = useTemplateFiles(
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

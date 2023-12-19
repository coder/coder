import { type FC } from "react";
import { Helmet } from "react-helmet-async";
import { Loader } from "components/Loader/Loader";
import { TemplateFiles } from "components/TemplateFiles/TemplateFiles";
import { useFileTab, useTemplateFiles } from "components/TemplateFiles/hooks";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import { getTemplatePageTitle } from "../utils";

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

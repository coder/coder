import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { templateExamples } from "api/queries/templates";
import type { TemplateExample } from "api/typesGenerated";
import { useDashboard } from "modules/dashboard/useDashboard";
import { pageTitle } from "utils/page";
import { getTemplatesByTag } from "utils/starterTemplates";
import { CreateTemplatesPageView } from "./CreateTemplatesPageView";
import { StarterTemplatesPageView } from "./StarterTemplatesPageView";

const CreateTemplatesGalleryPage: FC = () => {
  const { experiments } = useDashboard();
  const templateExamplesQuery = useQuery(templateExamples("default"));
  const starterTemplatesByTag = templateExamplesQuery.data
    ? // Currently, the scratch template should not be displayed on the starter templates page.
      getTemplatesByTag(removeScratchExample(templateExamplesQuery.data))
    : undefined;
  const multiOrgExperimentEnabled = experiments.includes("multi-organization");

  return (
    <>
      <Helmet>
        <title>{pageTitle("Create a Template")}</title>
      </Helmet>
      {multiOrgExperimentEnabled ? (
        <CreateTemplatesPageView
          error={templateExamplesQuery.error}
          starterTemplatesByTag={starterTemplatesByTag}
        />
      ) : (
        <StarterTemplatesPageView
          error={templateExamplesQuery.error}
          starterTemplatesByTag={starterTemplatesByTag}
        />
      )}
    </>
  );
};

const removeScratchExample = (data: TemplateExample[]) => {
  return data.filter((example) => example.id !== "scratch");
};

export default CreateTemplatesGalleryPage;

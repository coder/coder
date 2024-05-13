import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import { templateExamples } from "api/queries/templates";
import { useDashboard } from "modules/dashboard/useDashboard";
import { pageTitle } from "utils/page";
import { StarterTemplatePageView } from "./StarterTemplatePageView";

const StarterTemplatePage: FC = () => {
  const { exampleId } = useParams() as { exampleId: string };
  const { organizationId } = useDashboard();
  const templateExamplesQuery = useQuery(templateExamples(organizationId));
  const starterTemplate = templateExamplesQuery.data?.find(
    (example) => example.id === exampleId,
  );

  return (
    <>
      <Helmet>
        <title>{pageTitle(starterTemplate?.name ?? exampleId)}</title>
      </Helmet>

      <StarterTemplatePageView
        starterTemplate={starterTemplate}
        error={templateExamplesQuery.error}
      />
    </>
  );
};

export default StarterTemplatePage;

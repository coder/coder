import { useOrganizationId } from "hooks/useOrganizationId";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { StarterTemplatePageView } from "./StarterTemplatePageView";
import { useQuery } from "@tanstack/react-query";
import { templateExamples } from "api/queries/templates";

const StarterTemplatePage: FC = () => {
  const { exampleId } = useParams() as { exampleId: string };
  const organizationId = useOrganizationId();
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

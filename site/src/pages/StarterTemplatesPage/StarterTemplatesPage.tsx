import { useOrganizationId } from "hooks/useOrganizationId";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { StarterTemplatesPageView } from "./StarterTemplatesPageView";
import { useQuery } from "@tanstack/react-query";
import { templateExamples } from "api/queries/templates";
import { getTemplatesByTag } from "utils/starterTemplates";

const StarterTemplatesPage: FC = () => {
  const organizationId = useOrganizationId();
  const templateExamplesQuery = useQuery(templateExamples(organizationId));
  const starterTemplatesByTag = templateExamplesQuery.data
    ? getTemplatesByTag(templateExamplesQuery.data)
    : undefined;

  return (
    <>
      <Helmet>
        <title>{pageTitle("Starter Templates")}</title>
      </Helmet>

      <StarterTemplatesPageView
        error={templateExamplesQuery.error}
        starterTemplatesByTag={starterTemplatesByTag}
      />
    </>
  );
};

export default StarterTemplatesPage;

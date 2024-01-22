import { type FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { templateExamples } from "api/queries/templates";
import { useOrganizationId } from "contexts/auth/useOrganizationId";
import { pageTitle } from "utils/page";
import { getTemplatesByTag } from "utils/starterTemplates";
import { StarterTemplatesPageView } from "./StarterTemplatesPageView";

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

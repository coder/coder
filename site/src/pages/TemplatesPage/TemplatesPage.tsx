import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import {
  templateExamples,
  templatesByOrganizationId,
  templates,
} from "api/queries/templates";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useDashboard } from "modules/dashboard/useDashboard";
import { pageTitle } from "utils/page";
import { getTemplatesByOrg } from "utils/templateAggregators";
import { TemplatesPageView as MultiOrgTemplatesPageView } from "./MultiOrgTemplatePage/TemplatesPageView";
import { TemplatesPageView } from "./TemplatePage/TemplatesPageView";

export const TemplatesPage: FC = () => {
  const { permissions } = useAuthenticated();
  const { organizationId, experiments } = useDashboard();

  const templatesByOrganizationIdQuery = useQuery(
    templatesByOrganizationId(organizationId),
  );
  const templatesQuery = useQuery(templates());
  const templatesByOrg = templatesQuery.data
    ? getTemplatesByOrg(templatesQuery.data)
    : undefined;
  const examplesQuery = useQuery({
    ...templateExamples(organizationId),
    enabled: permissions.createTemplates,
  });
  const error =
    templatesByOrganizationIdQuery.error ||
    examplesQuery.error ||
    templatesQuery.error;

  // template gallery requires both experiments to be enabled.
  const templateGalleryExperimentEnabled =
    experiments.includes("multi-organization") &&
    experiments.includes("template-gallery");

  return (
    <>
      <Helmet>
        <title>{pageTitle("Templates")}</title>
      </Helmet>
      {templateGalleryExperimentEnabled ? (
        <MultiOrgTemplatesPageView
          templatesByOrg={templatesByOrg}
          examples={examplesQuery.data}
          canCreateTemplates={permissions.createTemplates}
          error={error}
        />
      ) : (
        <TemplatesPageView
          error={error}
          canCreateTemplates={permissions.createTemplates}
          examples={examplesQuery.data}
          templates={templatesByOrganizationIdQuery.data}
        />
      )}
    </>
  );
};

export default TemplatesPage;

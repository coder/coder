import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { templateExamples, templatesByOrganizationId, templates, } from "api/queries/templates";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useDashboard } from "modules/dashboard/useDashboard";
import { pageTitle } from "utils/page";
import { TemplatesPageView as MultiOrgTemplatesPageView } from "./MultiOrgTemplatePage/TemplatesPageView";
import { TemplatesPageView } from "./TemplatePage/TemplatesPageView";

export const TemplatesPage: FC = () => {
  const { permissions } = useAuthenticated();
  const { organizationId, experiments } = useDashboard();

  const templatesByOrganizationIdQuery = useQuery(templatesByOrganizationId(organizationId));
  const templatesQuery = useQuery(templates());
  const examplesQuery = useQuery({
    ...templateExamples(organizationId),
    enabled: permissions.createTemplates,
  });
  const error = templatesByOrganizationIdQuery.error || examplesQuery.error || templatesQuery.error;
  const multiOrgExperimentEnabled = experiments.includes("multi-organization");

  return (
    <>
      <Helmet>
        <title>{pageTitle("Templates")}</title>
      </Helmet>
      {multiOrgExperimentEnabled ? (
        <MultiOrgTemplatesPageView
          error={error}
          canCreateTemplates={permissions.createTemplates}
          examples={examplesQuery.data}
          templates={templatesQuery.data}
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

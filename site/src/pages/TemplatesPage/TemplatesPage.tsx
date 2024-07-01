import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { templateExamples, templates } from "api/queries/templates";
import { myOrganizations } from "api/queries/users";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useDashboard } from "modules/dashboard/useDashboard";
import { pageTitle } from "utils/page";
import { TemplatesPageView } from "./TemplatesPageView";
import { TemplatesPageViewV2 } from "./TemplatesPageViewV2";

export const TemplatesPage: FC = () => {
  const { permissions } = useAuthenticated();
  const { organizationId, experiments } = useDashboard();

  const organizationsQuery = useQuery(myOrganizations());
  const templatesQuery = useQuery(templates(organizationId));
  const examplesQuery = useQuery({
    ...templateExamples(organizationId),
    enabled: permissions.createTemplates,
  });
  const error = templatesQuery.error || examplesQuery.error || organizationsQuery.error;
  const multiOrgExperimentEnabled = experiments.includes("multi-organization");

  console.log({ multiOrgExperimentEnabled })
  return (
    <>
      <Helmet>
        <title>{pageTitle("Templates")}</title>
      </Helmet>
      {multiOrgExperimentEnabled ? (
        <TemplatesPageViewV2
          error={error}
          canCreateTemplates={permissions.createTemplates}
          examples={examplesQuery.data}
          templates={templatesQuery.data}
          organizations={organizationsQuery.data}
        />
    ) : (
        <TemplatesPageView
          error={error}
          canCreateTemplates={permissions.createTemplates}
          examples={examplesQuery.data}
          templates={templatesQuery.data}
        />
    )}
    </>
  );
};

export default TemplatesPage;

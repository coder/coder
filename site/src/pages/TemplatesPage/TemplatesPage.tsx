import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { templateExamples, templates } from "api/queries/templates";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { pageTitle } from "utils/page";
import { TemplatesPageView } from "./TemplatesPageView";
import { useDashboard } from "modules/dashboard/useDashboard";

export const TemplatesPage: FC = () => {
  const { permissions } = useAuthenticated();
  const { showOrganizations } = useDashboard();

  const templatesQuery = useQuery(templates());
  const examplesQuery = useQuery({
    ...templateExamples(),
    enabled: permissions.createTemplates,
  });
  const error = templatesQuery.error || examplesQuery.error;

  return (
    <>
      <Helmet>
        <title>{pageTitle("Templates")}</title>
      </Helmet>
      <TemplatesPageView
        error={error}
        showOrganizations={showOrganizations}
        canCreateTemplates={permissions.createTemplates}
        examples={examplesQuery.data}
        templates={templatesQuery.data}
      />
    </>
  );
};

export default TemplatesPage;

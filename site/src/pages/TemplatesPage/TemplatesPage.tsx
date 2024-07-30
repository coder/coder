import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { templateExamples, templates } from "api/queries/templates";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useDashboard } from "modules/dashboard/useDashboard";
import { pageTitle } from "utils/page";
import { TemplatesPageView } from "./TemplatesPageView";

export const TemplatesPage: FC = () => {
  const { permissions } = useAuthenticated();
  const { organizationId } = useDashboard();

  const templatesQuery = useQuery(templates(organizationId));
  const examplesQuery = useQuery({
    ...templateExamples(organizationId),
    enabled: permissions.createAnyTemplate,
  });
  const error = templatesQuery.error || examplesQuery.error;

  return (
    <>
      <Helmet>
        <title>{pageTitle("Templates")}</title>
      </Helmet>
      <TemplatesPageView
        error={error}
        canCreateTemplates={permissions.createAnyTemplate}
        examples={examplesQuery.data}
        templates={templatesQuery.data}
      />
    </>
  );
};

export default TemplatesPage;

import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { templateExamples, templates } from "api/queries/templates";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { pageTitle } from "utils/page";
import { TemplatesPageView } from "./TemplatesPageView";

export const TemplatesPage: FC = () => {
  const { organizationId, permissions } = useAuthenticated();
  const templatesQuery = useQuery(templates(organizationId));
  const examplesQuery = useQuery({
    ...templateExamples(organizationId),
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
        canCreateTemplates={permissions.createTemplates}
        examples={examplesQuery.data}
        templates={templatesQuery.data}
      />
    </>
  );
};

export default TemplatesPage;

import { useOrganizationId } from "hooks/useOrganizationId";
import { usePermissions } from "hooks/usePermissions";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { TemplatesPageView } from "./TemplatesPageView";
import { templateExamples, templates } from "api/queries/templates";
import { useQuery } from "@tanstack/react-query";

export const TemplatesPage: FC = () => {
  const organizationId = useOrganizationId();
  const permissions = usePermissions();
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

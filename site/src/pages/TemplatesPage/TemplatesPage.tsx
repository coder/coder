import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router-dom";
import {
  templateExamples,
  templatesByOrganizationId,
  templates,
} from "api/queries/templates";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useDashboard } from "modules/dashboard/useDashboard";
import { filterParamsKey } from "utils/filters";
import { pageTitle } from "utils/page";
import { TemplatesPageView as MultiOrgTemplatesPageView } from "./MultiOrgTemplatePage/TemplatesPageView";
import { TemplatesPageView } from "./TemplatePage/TemplatesPageView";

export const TemplatesPage: FC = () => {
  const { permissions } = useAuthenticated();
  const { organizationId, experiments } = useDashboard();
  const [searchParams] = useSearchParams();
  const query = searchParams.get(filterParamsKey) || undefined;

  const templatesByOrganizationIdQuery = useQuery(
    templatesByOrganizationId(organizationId),
  );
  const templatesQuery = useQuery(templates({ q: query }));
  const examplesQuery = useQuery({
    ...templateExamples(organizationId),
    enabled: permissions.createTemplates,
  });
  const error =
    templatesByOrganizationIdQuery.error ||
    examplesQuery.error ||
    templatesQuery.error;
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
          query={query}
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

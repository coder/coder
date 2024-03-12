import { createContext, type FC, Suspense, useContext } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { Outlet, useParams } from "react-router-dom";
import { checkAuthorization } from "api/queries/authCheck";
import { templateByName } from "api/queries/templates";
import type { AuthorizationResponse, Template } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { Stack } from "components/Stack/Stack";
import { useOrganizationId } from "contexts/auth/useOrganizationId";
import { pageTitle } from "utils/page";
import { Sidebar } from "./Sidebar";

const TemplateSettings = createContext<
  { template: Template; permissions: AuthorizationResponse } | undefined
>(undefined);

export function useTemplateSettings() {
  const value = useContext(TemplateSettings);
  if (!value) {
    throw new Error("This hook can only be used from a template settings page");
  }

  return value;
}

export const TemplateSettingsLayout: FC = () => {
  const orgId = useOrganizationId();
  const { template: templateName } = useParams() as { template: string };
  const templateQuery = useQuery(templateByName(orgId, templateName));
  const permissionsQuery = useQuery({
    ...checkAuthorization({
      checks: {
        canUpdateTemplate: {
          object: {
            resource_type: "template",
            resource_id: templateQuery.data?.id ?? "",
          },
          action: "update",
        },
      },
    }),
    enabled: templateQuery.isSuccess,
  });

  if (templateQuery.isLoading || permissionsQuery.isLoading) {
    return <Loader />;
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle([templateName, "Settings"])}</title>
      </Helmet>

      <Margins>
        <Stack css={{ padding: "48px 0" }} direction="row" spacing={10}>
          {templateQuery.isError || permissionsQuery.isError ? (
            <ErrorAlert error={templateQuery.error} />
          ) : (
            <TemplateSettings.Provider
              value={{
                template: templateQuery.data,
                permissions: permissionsQuery.data,
              }}
            >
              <Sidebar template={templateQuery.data} />
              <Suspense fallback={<Loader />}>
                <main css={{ width: "100%" }}>
                  <Outlet />
                </main>
              </Suspense>
            </TemplateSettings.Provider>
          )}
        </Stack>
      </Margins>
    </>
  );
};

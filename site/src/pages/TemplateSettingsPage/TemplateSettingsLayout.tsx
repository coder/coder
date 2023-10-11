import { makeStyles } from "@mui/styles";
import { Sidebar } from "./Sidebar";
import { Stack } from "components/Stack/Stack";
import { createContext, FC, Suspense, useContext } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { Loader } from "components/Loader/Loader";
import { Outlet, useParams } from "react-router-dom";
import { Margins } from "components/Margins/Margins";
import { useQuery } from "react-query";
import { useOrganizationId } from "hooks/useOrganizationId";
import { templateByName } from "api/queries/templates";
import { type AuthorizationResponse, type Template } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { checkAuthorization } from "api/queries/authCheck";

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
  const styles = useStyles();
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
        <Stack className={styles.wrapper} direction="row" spacing={10}>
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
                <main className={styles.content}>
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

const useStyles = makeStyles((theme) => ({
  wrapper: {
    padding: theme.spacing(6, 0),
  },

  content: {
    width: "100%",
  },
}));

import { makeStyles } from "@mui/styles";
import { Sidebar } from "./Sidebar";
import { Stack } from "components/Stack/Stack";
import { createContext, FC, Suspense, useContext } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "../../utils/page";
import { Loader } from "components/Loader/Loader";
import { Outlet, useParams } from "react-router-dom";
import { Margins } from "components/Margins/Margins";
import { checkAuthorization, getTemplateByName } from "api/api";
import { useQuery } from "@tanstack/react-query";
import { useOrganizationId } from "hooks/useOrganizationId";

const templatePermissions = (templateId: string) =>
  ({
    canUpdateTemplate: {
      object: {
        resource_type: "template",
        resource_id: templateId,
      },
      action: "update",
    },
  }) as const;

const fetchTemplateSettings = async (orgId: string, name: string) => {
  const template = await getTemplateByName(orgId, name);
  const permissions = await checkAuthorization({
    checks: templatePermissions(template.id),
  });

  return {
    template,
    permissions,
  };
};

export const getTemplateQuery = (name: string) => [
  "template",
  name,
  "settings",
];

const useTemplate = (orgId: string, name: string) => {
  return useQuery({
    queryKey: getTemplateQuery(name),
    queryFn: () => fetchTemplateSettings(orgId, name),
    keepPreviousData: true,
  });
};

const TemplateSettingsContext = createContext<
  Awaited<ReturnType<typeof fetchTemplateSettings>> | undefined
>(undefined);

export const useTemplateSettingsContext = () => {
  const context = useContext(TemplateSettingsContext);

  if (!context) {
    throw new Error(
      "useTemplateSettingsContext must be used within a TemplateSettingsContext.Provider",
    );
  }

  return context;
};

export const TemplateSettingsLayout: FC = () => {
  const styles = useStyles();
  const orgId = useOrganizationId();
  const { template: templateName } = useParams() as { template: string };
  const { data: settings } = useTemplate(orgId, templateName);

  return (
    <>
      <Helmet>
        <title>{pageTitle([templateName, "Settings"])}</title>
      </Helmet>

      {settings ? (
        <TemplateSettingsContext.Provider value={settings}>
          <Margins>
            <Stack className={styles.wrapper} direction="row" spacing={10}>
              <Sidebar template={settings.template} />
              <Suspense fallback={<Loader />}>
                <main className={styles.content}>
                  <Outlet />
                </main>
              </Suspense>
            </Stack>
          </Margins>
        </TemplateSettingsContext.Provider>
      ) : (
        <Loader />
      )}
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

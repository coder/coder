import { makeStyles } from "@mui/styles";
import { useOrganizationId } from "hooks/useOrganizationId";
import { createContext, FC, Suspense, useContext } from "react";
import { NavLink, Outlet, useNavigate, useParams } from "react-router-dom";
import { combineClasses } from "utils/combineClasses";
import { Margins } from "components/Margins/Margins";
import { Stack } from "components/Stack/Stack";
import { Loader } from "components/Loader/Loader";
import { TemplatePageHeader } from "./TemplatePageHeader";
import {
  checkAuthorization,
  getTemplateByName,
  getTemplateVersion,
} from "api/api";
import { useQuery } from "@tanstack/react-query";
import { AuthorizationRequest } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";

const templatePermissions = (
  templateId: string,
): AuthorizationRequest["checks"] => ({
  canUpdateTemplate: {
    object: {
      resource_type: "template",
      resource_id: templateId,
    },
    action: "update",
  },
});

const fetchTemplate = async (orgId: string, templateName: string) => {
  const template = await getTemplateByName(orgId, templateName);
  const [activeVersion, permissions] = await Promise.all([
    getTemplateVersion(template.active_version_id),
    checkAuthorization({
      checks: templatePermissions(template.id),
    }),
  ]);

  return {
    template,
    activeVersion,
    permissions,
  };
};

type TemplateLayoutContextValue = Awaited<ReturnType<typeof fetchTemplate>>;

const TemplateLayoutContext = createContext<
  TemplateLayoutContextValue | undefined
>(undefined);

export const useTemplateLayoutContext = (): TemplateLayoutContextValue => {
  const context = useContext(TemplateLayoutContext);
  if (!context) {
    throw new Error(
      "useTemplateLayoutContext only can be used inside of TemplateLayout",
    );
  }
  return context;
};

export const TemplateLayout: FC<{ children?: JSX.Element }> = ({
  children = <Outlet />,
}) => {
  const navigate = useNavigate();
  const styles = useStyles();
  const orgId = useOrganizationId();
  const { template: templateName } = useParams() as { template: string };
  const { data, error, isLoading } = useQuery({
    queryKey: ["template", templateName],
    queryFn: () => fetchTemplate(orgId, templateName),
  });
  const shouldShowInsights = data?.permissions?.canUpdateTemplate;

  if (error) {
    return (
      <div className={styles.error}>
        <ErrorAlert error={error} />
      </div>
    );
  }

  if (isLoading || !data) {
    return <Loader />;
  }

  return (
    <>
      <TemplatePageHeader
        template={data.template}
        activeVersion={data.activeVersion}
        permissions={data.permissions}
        onDeleteTemplate={() => {
          navigate("/templates");
        }}
      />

      <div className={styles.tabs}>
        <Margins>
          <Stack direction="row" spacing={0.25}>
            <NavLink
              end
              to={`/templates/${templateName}`}
              className={({ isActive }) =>
                combineClasses([
                  styles.tabItem,
                  isActive ? styles.tabItemActive : undefined,
                ])
              }
            >
              Summary
            </NavLink>
            <NavLink
              end
              to={`/templates/${templateName}/docs`}
              className={({ isActive }) =>
                combineClasses([
                  styles.tabItem,
                  isActive ? styles.tabItemActive : undefined,
                ])
              }
            >
              Docs
            </NavLink>
            {data.permissions.canUpdateTemplate && (
              <NavLink
                to={`/templates/${templateName}/files`}
                className={({ isActive }) =>
                  combineClasses([
                    styles.tabItem,
                    isActive ? styles.tabItemActive : undefined,
                  ])
                }
              >
                Source Code
              </NavLink>
            )}
            <NavLink
              to={`/templates/${templateName}/versions`}
              className={({ isActive }) =>
                combineClasses([
                  styles.tabItem,
                  isActive ? styles.tabItemActive : undefined,
                ])
              }
            >
              Versions
            </NavLink>
            <NavLink
              to={`/templates/${templateName}/embed`}
              className={({ isActive }) =>
                combineClasses([
                  styles.tabItem,
                  isActive ? styles.tabItemActive : undefined,
                ])
              }
            >
              Embed
            </NavLink>
            {shouldShowInsights && (
              <NavLink
                to={`/templates/${templateName}/insights`}
                className={({ isActive }) =>
                  combineClasses([
                    styles.tabItem,
                    isActive ? styles.tabItemActive : undefined,
                  ])
                }
              >
                Insights
              </NavLink>
            )}
          </Stack>
        </Margins>
      </div>

      <Margins>
        <TemplateLayoutContext.Provider value={data}>
          <Suspense fallback={<Loader />}>{children}</Suspense>
        </TemplateLayoutContext.Provider>
      </Margins>
    </>
  );
};

export const useStyles = makeStyles((theme) => {
  return {
    error: {
      margin: theme.spacing(2),
    },
    tabs: {
      borderBottom: `1px solid ${theme.palette.divider}`,
      marginBottom: theme.spacing(5),
    },

    tabItem: {
      textDecoration: "none",
      color: theme.palette.text.secondary,
      fontSize: 14,
      display: "block",
      padding: theme.spacing(0, 2, 2),

      "&:hover": {
        color: theme.palette.text.primary,
      },
    },

    tabItemActive: {
      color: theme.palette.text.primary,
      position: "relative",

      "&:before": {
        content: `""`,
        left: 0,
        bottom: 0,
        height: 2,
        width: "100%",
        background: theme.palette.secondary.dark,
        position: "absolute",
      },
    },
  };
});

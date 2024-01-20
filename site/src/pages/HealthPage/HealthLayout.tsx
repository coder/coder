import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import CircularProgress from "@mui/material/CircularProgress";
import ReplayIcon from "@mui/icons-material/Replay";
import NotificationsOffOutlined from "@mui/icons-material/NotificationsOffOutlined";
import { cx } from "@emotion/css";
import { useTheme } from "@emotion/react";
import { type FC, Suspense } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { NavLink, Outlet } from "react-router-dom";
import kebabCase from "lodash/fp/kebabCase";
import { health, refreshHealth } from "api/queries/debug";
import type { HealthSeverity } from "api/typesGenerated";
import { type ClassName, useClassName } from "hooks/useClassName";
import { pageTitle } from "utils/page";
import { createDayString } from "utils/createDayString";
import { DashboardFullPage } from "modules/dashboard/DashboardLayout";
import { Loader } from "components/Loader/Loader";
import { HealthIcon } from "./Content";

export const HealthLayout: FC = () => {
  const theme = useTheme();
  const queryClient = useQueryClient();
  const { data: healthStatus } = useQuery({
    ...health(),
    refetchInterval: 30_000,
  });
  const { mutate: forceRefresh, isLoading: isRefreshing } = useMutation(
    refreshHealth(queryClient),
  );
  const sections = {
    derp: "DERP",
    access_url: "Access URL",
    websocket: "Websocket",
    database: "Database",
    workspace_proxy: "Workspace Proxy",
    provisioner_daemons: "Provisioner Daemons",
  } as const;
  const visibleSections = filterVisibleSections(sections);

  const link = useClassName(classNames.link, []);
  const activeLink = useClassName(classNames.activeLink, []);

  return (
    <>
      <Helmet>
        <title>{pageTitle("Health")}</title>
      </Helmet>

      {healthStatus ? (
        <DashboardFullPage>
          <div
            css={{
              display: "flex",
              flexBasis: 0,
              flex: 1,
              overflow: "hidden",
            }}
          >
            <div
              css={{
                width: 256,
                flexShrink: 0,
                borderRight: `1px solid ${theme.palette.divider}`,
                fontSize: 14,
              }}
            >
              <div
                css={{
                  padding: 24,
                  display: "flex",
                  flexDirection: "column",
                  gap: 16,
                }}
              >
                <div>
                  <div
                    css={{
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "space-between",
                    }}
                  >
                    <HealthIcon size={32} severity={healthStatus.severity} />

                    <Tooltip title="Refresh health checks">
                      <IconButton
                        size="small"
                        disabled={isRefreshing}
                        data-testid="healthcheck-refresh-button"
                        onClick={() => {
                          forceRefresh();
                        }}
                      >
                        {isRefreshing ? (
                          <CircularProgress size={16} />
                        ) : (
                          <ReplayIcon css={{ width: 20, height: 20 }} />
                        )}
                      </IconButton>
                    </Tooltip>
                  </div>
                  <div css={{ fontWeight: 500, marginTop: 16 }}>
                    {healthStatus.healthy ? "Healthy" : "Unhealthy"}
                  </div>
                  <div
                    css={{
                      color: theme.palette.text.secondary,
                      lineHeight: "150%",
                    }}
                  >
                    {healthStatus.healthy
                      ? Object.keys(visibleSections).some((key) => {
                          const section =
                            healthStatus[key as keyof typeof visibleSections];
                          return (
                            section.warnings && section.warnings.length > 0
                          );
                        })
                        ? "All systems operational, but performance might be degraded"
                        : "All systems operational"
                      : "Some issues have been detected"}
                  </div>
                </div>

                <div css={{ display: "flex", flexDirection: "column" }}>
                  <span css={{ fontWeight: 500 }}>Last check</span>
                  <span
                    data-chromatic="ignore"
                    css={{
                      color: theme.palette.text.secondary,
                      lineHeight: "150%",
                    }}
                  >
                    {createDayString(healthStatus.time)}
                  </span>
                </div>

                <div css={{ display: "flex", flexDirection: "column" }}>
                  <span css={{ fontWeight: 500 }}>Version</span>
                  <span
                    data-chromatic="ignore"
                    css={{
                      color: theme.palette.text.secondary,
                      lineHeight: "150%",
                    }}
                  >
                    {healthStatus.coder_version}
                  </span>
                </div>
              </div>

              <nav css={{ display: "flex", flexDirection: "column", gap: 1 }}>
                {Object.keys(visibleSections)
                  .sort()
                  .map((key) => {
                    const label =
                      visibleSections[key as keyof typeof visibleSections];
                    const healthSection =
                      healthStatus[key as keyof typeof visibleSections];

                    return (
                      <NavLink
                        end
                        key={key}
                        to={`/health/${kebabCase(key)}`}
                        className={({ isActive }) =>
                          cx([link, isActive && activeLink])
                        }
                      >
                        <HealthIcon
                          size={16}
                          severity={healthSection.severity as HealthSeverity}
                        />
                        {label}
                        {healthSection.dismissed && (
                          <NotificationsOffOutlined
                            css={{
                              fontSize: 14,
                              marginLeft: "auto",
                              color: theme.palette.text.disabled,
                            }}
                          />
                        )}
                      </NavLink>
                    );
                  })}
              </nav>
            </div>

            <div css={{ overflowY: "auto", width: "100%" }}>
              <Suspense fallback={<Loader />}>
                <Outlet context={healthStatus} />
              </Suspense>
            </div>
          </div>
        </DashboardFullPage>
      ) : (
        <Loader />
      )}
    </>
  );
};

const filterVisibleSections = <T extends object>(sections: T) => {
  return Object.keys(sections).reduce(
    (visible, sectionName) => {
      const sectionValue = sections[sectionName as keyof typeof sections];

      if (!sectionValue) {
        return visible;
      }

      return {
        ...visible,
        [sectionName]: sectionValue,
      };
    },
    {} as Partial<typeof sections>,
  );
};

const classNames = {
  link: (css, theme) =>
    css({
      background: "none",
      pointerEvents: "auto",
      color: theme.palette.text.secondary,
      border: "none",
      fontSize: 14,
      width: "100%",
      display: "flex",
      alignItems: "center",
      gap: 12,
      textAlign: "left",
      height: 36,
      padding: "0 24px",
      cursor: "pointer",
      textDecoration: "none",

      "&:hover": {
        background: theme.palette.action.hover,
        color: theme.palette.text.primary,
      },
    }),

  activeLink: (css, theme) =>
    css({
      background: theme.palette.action.hover,
      pointerEvents: "none",
      color: theme.palette.text.primary,
    }),
} satisfies Record<string, ClassName>;

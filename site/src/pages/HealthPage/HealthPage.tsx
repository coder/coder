import CheckCircleOutlined from "@mui/icons-material/CheckCircleOutlined";
import ErrorOutline from "@mui/icons-material/ErrorOutline";
import ReplayIcon from "@mui/icons-material/Replay";
import { useTheme } from "@mui/material/styles";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import CircularProgress from "@mui/material/CircularProgress";
import { type Interpolation, type Theme } from "@emotion/react";
import { type FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { getHealth } from "api/api";
import { health, refreshHealth } from "api/queries/debug";
import { useTab } from "hooks";
import { createDayString } from "utils/createDayString";
import { pageTitle } from "utils/page";
import { DashboardFullPage } from "components/Dashboard/DashboardLayout";
import { Loader } from "components/Loader/Loader";
import { SyntaxHighlighter } from "components/SyntaxHighlighter/SyntaxHighlighter";

const sections = {
  derp: "DERP",
  access_url: "Access URL",
  websocket: "Websocket",
  database: "Database",
  workspace_proxy: "Workspace Proxy",
} as const;

export default function HealthPage() {
  const tab = useTab("tab", "derp");
  const queryClient = useQueryClient();
  const { data: healthStatus } = useQuery({
    ...health(),
    refetchInterval: 30_000,
  });
  const { mutate: forceRefresh, isLoading: isRefreshing } = useMutation(
    refreshHealth(queryClient),
  );

  return (
    <>
      <Helmet>
        <title>{pageTitle("Health")}</title>
      </Helmet>

      {healthStatus ? (
        <HealthPageView
          tab={tab}
          healthStatus={healthStatus}
          forceRefresh={forceRefresh}
          isRefreshing={isRefreshing}
        />
      ) : (
        <Loader />
      )}
    </>
  );
}

interface HealthPageViewProps {
  healthStatus: Awaited<ReturnType<typeof getHealth>>;
  tab: ReturnType<typeof useTab>;
  forceRefresh: () => void;
  isRefreshing: boolean;
}

export const HealthPageView: FC<HealthPageViewProps> = ({
  healthStatus,
  tab,
  forceRefresh,
  isRefreshing,
}) => {
  const theme = useTheme();

  return (
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
                {healthStatus.healthy ? (
                  <CheckCircleOutlined
                    css={{
                      width: 32,
                      height: 32,
                      color: theme.palette.success.light,
                    }}
                  />
                ) : (
                  <ErrorOutline
                    css={{
                      width: 32,
                      height: 32,
                      color: theme.palette.error.light,
                    }}
                  />
                )}

                <Tooltip title="Refresh health checks">
                  <IconButton
                    size="small"
                    disabled={isRefreshing}
                    data-testid="healthcheck-refresh-button"
                    onClick={forceRefresh}
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
                  ? Object.keys(sections).some(
                      (key) =>
                        healthStatus[key as keyof typeof sections].warnings !==
                          null &&
                        healthStatus[key as keyof typeof sections].warnings
                          .length > 0,
                    )
                    ? "All systems operational, but performance might be degraded"
                    : "All systems operational"
                  : "Some issues have been detected"}
              </div>
            </div>

            <div css={{ display: "flex", flexDirection: "column" }}>
              <span css={{ fontWeight: 500 }}>Last check</span>
              <span
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
            {Object.keys(sections)
              .sort()
              .map((key) => {
                const label = sections[key as keyof typeof sections];
                const isActive = tab.value === key;
                const healthSection =
                  healthStatus[key as keyof typeof sections];
                const isHealthy = healthSection.healthy;
                const isWarning = healthSection.warnings?.length > 0;
                return (
                  <button
                    key={key}
                    onClick={() => {
                      tab.set(key);
                    }}
                    css={[
                      styles.sectionLink,
                      isActive && styles.activeSectionLink,
                    ]}
                  >
                    {isHealthy ? (
                      isWarning ? (
                        <CheckCircleOutlined
                          css={{
                            width: 16,
                            height: 16,
                            color: theme.palette.warning.light,
                          }}
                        />
                      ) : (
                        <CheckCircleOutlined
                          css={{
                            width: 16,
                            height: 16,
                            color: theme.palette.success.light,
                          }}
                        />
                      )
                    ) : (
                      <ErrorOutline
                        css={{
                          width: 16,
                          height: 16,
                          color: theme.palette.error.main,
                        }}
                      />
                    )}
                    {label}
                  </button>
                );
              })}
          </nav>
        </div>
        <div css={{ overflowY: "auto", width: "100%" }} data-chromatic="ignore">
          <SyntaxHighlighter
            language="json"
            editorProps={{ height: "100%" }}
            value={JSON.stringify(
              healthStatus[tab.value as keyof typeof healthStatus],
              null,
              2,
            )}
          />
        </div>
      </div>
    </DashboardFullPage>
  );
};

const styles = {
  sectionLink: (theme) => ({
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
    background: "none",
    color: theme.palette.text.secondary,

    "&:hover": {
      background: theme.palette.action.hover,
      color: theme.palette.text.primary,
    },
  }),

  activeSectionLink: (theme) => ({
    background: theme.palette.action.hover,
    pointerEvents: "none",
    color: theme.palette.text.primary,
  }),
} satisfies Record<string, Interpolation<Theme>>;

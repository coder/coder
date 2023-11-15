import { type Interpolation, type Theme } from "@emotion/react";
import Box from "@mui/material/Box";
import { useQuery, useQueryClient } from "react-query";
import { getHealth } from "api/api";
import { Loader } from "components/Loader/Loader";
import { useTab } from "hooks";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { colors } from "theme/colors";
import CheckCircleOutlined from "@mui/icons-material/CheckCircleOutlined";
import ErrorOutline from "@mui/icons-material/ErrorOutline";
import { SyntaxHighlighter } from "components/SyntaxHighlighter/SyntaxHighlighter";
import { Stack } from "components/Stack/Stack";
import {
  FullWidthPageHeader,
  PageHeaderTitle,
  PageHeaderSubtitle,
} from "components/PageHeader/FullWidthPageHeader";
import { Stats, StatsItem } from "components/Stats/Stats";
import { createDayString } from "utils/createDayString";
import { DashboardFullPage } from "components/Dashboard/DashboardLayout";
import { LoadingButton } from "@mui/lab";
import ReplayIcon from "@mui/icons-material/Replay";
import { FC } from "react";

const sections = {
  derp: "DERP",
  access_url: "Access URL",
  websocket: "Websocket",
  database: "Database",
} as const;

export default function HealthPage() {
  const tab = useTab("tab", "derp");
  const queryClient = useQueryClient();
  const forceRefresh = async () => {
    await queryClient.invalidateQueries(["health"]);
  }
  const { data: healthStatus } = useQuery({
    queryKey: ["health"],
    // TODO: We don't want to set force=true each time.
    // Only if the "refresh" button is clicked.
    queryFn: async () => getHealth(true),
    refetchInterval: 30_000,
  });

  return (
    <>
      <Helmet>
        <title>{pageTitle("Health")}</title>
      </Helmet>

      {healthStatus ? (
        <HealthPageView healthStatus={healthStatus} tab={tab} forceRefresh={forceRefresh} />
      ) : (
        <Loader />
      )}
    </>
  );
}

export function HealthPageView({
  healthStatus,
  tab,
  forceRefresh,
}: {
  healthStatus: Awaited<ReturnType<typeof getHealth>>;
  tab: ReturnType<typeof useTab>;
  forceRefresh: () => Promise<void>;
}) {
  return (
    <DashboardFullPage>
      <FullWidthPageHeader sticky={false}>
        <Stack direction="row" spacing={2} alignItems="center">
          {healthStatus.healthy ? (
            <CheckCircleOutlined
              sx={{
                width: 32,
                height: 32,
                color: (theme) => theme.palette.success.light,
              }}
            />
          ) : (
            <ErrorOutline
              sx={{
                width: 32,
                height: 32,
                color: (theme) => theme.palette.error.main,
              }}
            />
          )}

          <div>
            <PageHeaderTitle>
              {healthStatus.healthy ? "Healthy" : "Unhealthy"}
            </PageHeaderTitle>
            <PageHeaderSubtitle>
              {healthStatus.healthy
                ? "All systems operational"
                : "Some issues have been detected"}
            </PageHeaderSubtitle>
          </div>
        </Stack>

        <Stats aria-label="Deployment details" css={styles.stats}>
          <StatsItem
            css={styles.statsItem}
            label="Last check"
            value={createDayString(healthStatus.time)}
          />
          <StatsItem
            css={styles.statsItem}
            label="Coder version"
            value={healthStatus.coder_version}
          />
        </Stats>
        <RefreshButton
          loading={false}
          handleAction={async () => {
            await forceRefresh().catch((e) => {
              // handle error
              console.log("error forcing refresh: "+ e)
            })
          }}
        />
      </FullWidthPageHeader>
      <Box
        sx={{
          display: "flex",
          flexBasis: 0,
          flex: 1,
          overflow: "hidden",
        }}
      >
        <Box
          sx={{
            width: 256,
            flexShrink: 0,
            borderRight: (theme) => `1px solid ${theme.palette.divider}`,
          }}
        >
          <Box
            sx={{
              fontSize: 10,
              textTransform: "uppercase",
              fontWeight: 500,
              color: (theme) => theme.palette.text.secondary,
              padding: "12px 24px",
              letterSpacing: "0.5px",
            }}
          >
            Health
          </Box>
          <Box component="nav">
            {Object.keys(sections)
              .sort()
              .map((key) => {
                const label = sections[key as keyof typeof sections];
                const isActive = tab.value === key;
                const isHealthy =
                  healthStatus[key as keyof typeof sections].healthy;

                return (
                  <Box
                    component="button"
                    key={key}
                    onClick={() => {
                      tab.set(key);
                    }}
                    sx={{
                      background: isActive ? colors.gray[13] : "none",
                      border: "none",
                      fontSize: 14,
                      width: "100%",
                      display: "flex",
                      alignItems: "center",
                      gap: 1,
                      textAlign: "left",
                      height: 36,
                      padding: "0 24px",
                      cursor: "pointer",
                      pointerEvents: isActive ? "none" : "auto",
                      color: (theme) =>
                        isActive
                          ? theme.palette.text.primary
                          : theme.palette.text.secondary,
                      "&:hover": {
                        background: (theme) => theme.palette.action.hover,
                        color: (theme) => theme.palette.text.primary,
                      },
                    }}
                  >
                    {isHealthy ? (
                      <CheckCircleOutlined
                        sx={{
                          width: 16,
                          height: 16,
                          color: (theme) => theme.palette.success.light,
                        }}
                      />
                    ) : (
                      <ErrorOutline
                        sx={{
                          width: 16,
                          height: 16,
                          color: (theme) => theme.palette.error.main,
                        }}
                      />
                    )}
                    {label}
                  </Box>
                );
              })}
          </Box>
        </Box>
        {/* 62px - navbar and 36px - the bottom bar */}
        <Box sx={{ overflowY: "auto", width: "100%" }} data-chromatic="ignore">
          <SyntaxHighlighter
            language="json"
            editorProps={{ height: "100%" }}
            value={JSON.stringify(
              healthStatus[tab.value as keyof typeof healthStatus],
              null,
              2,
            )}
          />
        </Box>
      </Box>
    </DashboardFullPage>
  );
}

const styles = {
  stats: (theme) => ({
    padding: 0,
    border: 0,
    gap: 48,
    rowGap: 24,
    flex: 1,

    [theme.breakpoints.down("md")]: {
      display: "flex",
      flexDirection: "column",
      alignItems: "flex-start",
      gap: 8,
    },
  }),

  statsItem: {
    flexDirection: "column",
    gap: 0,
    padding: 0,

    "& > span:first-of-type": {
      fontSize: 12,
      fontWeight: 500,
    },
  },
} satisfies Record<string, Interpolation<Theme>>;

interface HealthcheckAction {
  handleAction: () => void;
  loading: boolean;
}

export const RefreshButton: FC<HealthcheckAction> = ({
  handleAction,
  loading,
}) => {
  return (
    <LoadingButton
      loading={loading}
      loadingPosition="start"
      data-testid="healthcheck-refresh-button"
      startIcon={<ReplayIcon />}
      onClick={handleAction}
    >
      {loading ? <>Refreshing&hellip;</> : <>Refresh</>}
    </LoadingButton>
  );
};

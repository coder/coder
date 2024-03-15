import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import type { FC } from "react";
import { Link } from "react-router-dom";
import type {
  ProvisionerJobLog,
  WorkspaceAgent,
  WorkspaceBuild,
} from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { BuildAvatar } from "components/BuildAvatar/BuildAvatar";
import { Loader } from "components/Loader/Loader";
import {
  FullWidthPageHeader,
  PageHeaderTitle,
  PageHeaderSubtitle,
} from "components/PageHeader/FullWidthPageHeader";
import { Stack } from "components/Stack/Stack";
import { Stats, StatsItem } from "components/Stats/Stats";
import { TAB_PADDING_X, TabLink, Tabs, TabsList } from "components/Tabs/Tabs";
import { useSearchParamsKey } from "hooks/useSearchParamsKey";
import { DashboardFullPage } from "modules/dashboard/DashboardLayout";
import { AgentLogs, useAgentLogs } from "modules/resources/AgentLogs";
import {
  WorkspaceBuildData,
  WorkspaceBuildDataSkeleton,
} from "modules/workspaces/WorkspaceBuildData/WorkspaceBuildData";
import { WorkspaceBuildLogs } from "modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import { displayWorkspaceBuildDuration } from "utils/workspace";
import { Sidebar, SidebarCaption, SidebarItem } from "./Sidebar";

export const LOGS_TAB_KEY = "logs";

const sortLogsByCreatedAt = (logs: ProvisionerJobLog[]) => {
  return [...logs].sort(
    (a, b) =>
      new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
  );
};

export interface WorkspaceBuildPageViewProps {
  logs: ProvisionerJobLog[] | undefined;
  build: WorkspaceBuild | undefined;
  builds: WorkspaceBuild[] | undefined;
  activeBuildNumber: number;
}

export const WorkspaceBuildPageView: FC<WorkspaceBuildPageViewProps> = ({
  logs,
  build,
  builds,
  activeBuildNumber,
}) => {
  const theme = useTheme();
  const tabState = useSearchParamsKey({
    key: LOGS_TAB_KEY,
    defaultValue: "build",
  });

  if (!build) {
    return <Loader />;
  }

  const agents = build.resources.flatMap((r) => r.agents ?? []);
  const selectedAgent = agents.find((a) => a.id === tabState.value);

  return (
    <DashboardFullPage>
      <FullWidthPageHeader sticky={false}>
        <Stack direction="row" alignItems="center" spacing={3}>
          <BuildAvatar build={build} />
          <div>
            <PageHeaderTitle>Build #{build.build_number}</PageHeaderTitle>
            <PageHeaderSubtitle>{build.initiator_name}</PageHeaderSubtitle>
          </div>
        </Stack>

        <Stats aria-label="Build details" css={styles.stats}>
          <StatsItem
            css={styles.statsItem}
            label="Workspace"
            value={
              <Link
                to={`/@${build.workspace_owner_name}/${build.workspace_name}`}
              >
                {build.workspace_name}
              </Link>
            }
          />
          <StatsItem
            css={styles.statsItem}
            label="Template version"
            value={build.template_version_name}
          />
          <StatsItem
            css={styles.statsItem}
            label="Duration"
            value={displayWorkspaceBuildDuration(build)}
          />
          <StatsItem
            css={styles.statsItem}
            label="Started at"
            value={new Date(build.created_at).toLocaleString()}
          />
          <StatsItem
            css={styles.statsItem}
            label="Action"
            value={
              <span css={{ textTransform: "capitalize" }}>
                {build.transition}
              </span>
            }
          />
        </Stats>
      </FullWidthPageHeader>

      <div
        css={{
          display: "flex",
          alignItems: "start",
          overflow: "hidden",
          flex: 1,
          flexBasis: 0,
        }}
      >
        <Sidebar>
          <SidebarCaption>Builds</SidebarCaption>
          {!builds &&
            Array.from({ length: 15 }, (_, i) => (
              <SidebarItem key={i}>
                <WorkspaceBuildDataSkeleton />
              </SidebarItem>
            ))}

          {builds?.map((build) => (
            <Link
              key={build.id}
              to={`/@${build.workspace_owner_name}/${build.workspace_name}/builds/${build.build_number}`}
            >
              <SidebarItem active={build.build_number === activeBuildNumber}>
                <WorkspaceBuildData build={build} />
              </SidebarItem>
            </Link>
          ))}
        </Sidebar>

        <div css={{ height: "100%", overflowY: "auto", width: "100%" }}>
          <Tabs active={tabState.value}>
            <TabsList>
              <TabLink to={`?${LOGS_TAB_KEY}=build`} value="build">
                Build
              </TabLink>

              {agents.map((a) => (
                <TabLink
                  to={`?${LOGS_TAB_KEY}=${a.id}`}
                  value={a.id}
                  key={a.id}
                >
                  coder_agent.{a.name}
                </TabLink>
              ))}
            </TabsList>
          </Tabs>
          {build.transition === "delete" && build.job.status === "failed" && (
            <Alert
              severity="error"
              css={{
                borderRadius: 0,
                border: 0,
                background: theme.palette.error.dark,
                borderBottom: `1px solid ${theme.palette.divider}`,
              }}
            >
              <div>
                The workspace may have failed to delete due to a Terraform state
                mismatch. A template admin may run{" "}
                <code
                  css={{
                    display: "inline-block",
                    width: "fit-content",
                    fontWeight: 600,
                  }}
                >
                  {`coder rm ${
                    build.workspace_owner_name + "/" + build.workspace_name
                  } --orphan`}
                </code>{" "}
                to delete the workspace skipping resource destruction.
              </div>
            </Alert>
          )}

          {tabState.value === "build" ? (
            <BuildLogsContent logs={logs} />
          ) : (
            <AgentLogsContent agent={selectedAgent!} />
          )}
        </div>
      </div>
    </DashboardFullPage>
  );
};

const BuildLogsContent: FC<{ logs?: ProvisionerJobLog[] }> = ({ logs }) => {
  if (!logs) {
    return <Loader />;
  }

  return (
    <WorkspaceBuildLogs
      css={{
        border: 0,
        "--log-line-side-padding": `${TAB_PADDING_X}px`,
        // Add extra spacing to the first log header to prevent it from being
        // too close to the tabs
        "& .logs-header:first-of-type": {
          paddingTop: 16,
        },
      }}
      logs={sortLogsByCreatedAt(logs)}
    />
  );
};

const AgentLogsContent: FC<{ agent: WorkspaceAgent }> = ({ agent }) => {
  const logs = useAgentLogs(agent.id);

  if (!logs) {
    return <Loader />;
  }

  return (
    <AgentLogs
      sources={agent.log_sources}
      logs={logs}
      height={560}
      width="100%"
    />
  );
};

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

import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import { type FC } from "react";
import type { ProvisionerJobLog, WorkspaceBuild } from "api/typesGenerated";
import { Link } from "react-router-dom";
import { displayWorkspaceBuildDuration } from "utils/workspace";
import { DashboardFullPage } from "modules/dashboard/DashboardLayout";
import { BuildAvatar } from "components/BuildAvatar/BuildAvatar";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import { WorkspaceBuildLogs } from "modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import {
  FullWidthPageHeader,
  PageHeaderTitle,
  PageHeaderSubtitle,
} from "components/PageHeader/FullWidthPageHeader";
import { Stats, StatsItem } from "components/Stats/Stats";
import { Alert } from "components/Alert/Alert";
import {
  WorkspaceBuildData,
  WorkspaceBuildDataSkeleton,
} from "modules/workspaces/WorkspaceBuild/WorkspaceBuildData";
import { Sidebar, SidebarCaption, SidebarItem } from "./Sidebar";

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

  if (!build) {
    return <Loader />;
  }

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
          {logs ? (
            <WorkspaceBuildLogs
              css={{ border: 0 }}
              logs={sortLogsByCreatedAt(logs)}
            />
          ) : (
            <Loader />
          )}
        </div>
      </div>
    </DashboardFullPage>
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

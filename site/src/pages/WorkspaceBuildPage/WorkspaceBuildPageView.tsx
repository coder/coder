import { BuildAvatar } from "components/BuildsTable/BuildAvatar"
import { FC } from "react"
import { ProvisionerJobLog, WorkspaceBuild } from "../../api/typesGenerated"
import { Loader } from "../../components/Loader/Loader"
import { Stack } from "../../components/Stack/Stack"
import { WorkspaceBuildLogs } from "../../components/WorkspaceBuildLogs/WorkspaceBuildLogs"
import { WorkspaceBuildStateError } from "./WorkspaceBuildStateError"
import { makeStyles, useTheme } from "@mui/styles"
import {
  FullWidthPageHeader,
  PageHeaderTitle,
  PageHeaderSubtitle,
} from "components/PageHeader/FullWidthPageHeader"
import { Link } from "react-router-dom"
import { Stats, StatsItem } from "components/Stats/Stats"
import {
  displayWorkspaceBuildDuration,
  getDisplayWorkspaceBuildInitiatedBy,
  getDisplayWorkspaceBuildStatus,
} from "utils/workspace"
import Box from "@mui/material/Box"
import {
  Sidebar,
  SidebarCaption,
  SidebarItem,
} from "components/Sidebar/Sidebar"
import { BuildIcon } from "components/BuildIcon/BuildIcon"

const sortLogsByCreatedAt = (logs: ProvisionerJobLog[]) => {
  return [...logs].sort(
    (a, b) =>
      new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
  )
}

export interface WorkspaceBuildPageViewProps {
  logs: ProvisionerJobLog[] | undefined
  build: WorkspaceBuild | undefined
  builds: WorkspaceBuild[] | undefined
  activeBuildNumber: number
}

export const WorkspaceBuildPageView: FC<WorkspaceBuildPageViewProps> = ({
  logs,
  build,
  builds,
  activeBuildNumber,
}) => {
  const styles = useStyles()
  const theme = useTheme()

  if (!build) {
    return <Loader />
  }

  return (
    <Box
      sx={{
        height: "calc(100vh - 62px - 36px)",
        overflow: "hidden",
        // Remove padding added from dashboard layout (.siteContent)
        marginBottom: "-48px",
        display: "flex",
        flexDirection: "column",
      }}
    >
      <FullWidthPageHeader sticky={false}>
        <Stack direction="row" alignItems="center" spacing={3}>
          <BuildAvatar build={build} />
          <div>
            <PageHeaderTitle>Build #{build.build_number}</PageHeaderTitle>
            <PageHeaderSubtitle>{build.initiator_name}</PageHeaderSubtitle>
          </div>
        </Stack>

        <Stats aria-label="Build details" className={styles.stats}>
          <StatsItem
            className={styles.statsItem}
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
            className={styles.statsItem}
            label="Duration"
            value={displayWorkspaceBuildDuration(build)}
          />
          <StatsItem
            className={styles.statsItem}
            label="Started at"
            value={new Date(build.created_at).toLocaleString()}
          />
          <StatsItem
            className={styles.statsItem}
            label="Action"
            value={build.transition}
          />
        </Stats>
      </FullWidthPageHeader>

      <Box
        sx={{
          display: "flex",
          alignItems: "start",
          overflow: "hidden",
          flex: 1,
        }}
      >
        <Sidebar>
          <SidebarCaption>Builds</SidebarCaption>
          {builds?.map((b) => (
            <Link
              key={b.id}
              to={`/@${b.workspace_owner_name}/${b.workspace_name}/builds/${b.build_number}`}
            >
              <SidebarItem
                active={b.build_number === activeBuildNumber}
                sx={{ color: "red" }}
              >
                <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                  <BuildIcon
                    transition={b.transition}
                    sx={{
                      width: 16,
                      height: 16,
                      color: getDisplayWorkspaceBuildStatus(theme, b).color,
                    }}
                  />
                  <Box sx={{ overflow: "hidden" }}>
                    <Box
                      sx={{
                        textTransform: "capitalize",
                        color: (theme) => theme.palette.text.primary,
                        textOverflow: "ellipsis",
                        overflow: "hidden",
                        whiteSpace: "nowrap",
                      }}
                    >
                      {b.transition} by{" "}
                      <strong>{getDisplayWorkspaceBuildInitiatedBy(b)}</strong>
                    </Box>
                    <Box
                      sx={{
                        fontSize: 12,
                        color: (theme) => theme.palette.text.secondary,
                        mt: 0.25,
                      }}
                    >
                      {displayWorkspaceBuildDuration(b)}
                    </Box>
                  </Box>
                </Box>
              </SidebarItem>
            </Link>
          ))}
        </Sidebar>

        <Box sx={{ height: "100%", overflowY: "auto", width: "100%" }}>
          {build.transition === "delete" && build.job.status === "failed" && (
            <WorkspaceBuildStateError build={build} />
          )}
          {!logs && <Loader />}
          {logs && (
            <WorkspaceBuildLogs
              sx={{ border: 0 }}
              logs={sortLogsByCreatedAt(logs)}
            />
          )}
        </Box>
      </Box>
    </Box>
  )
}

const useStyles = makeStyles((theme) => ({
  stats: {
    padding: 0,
    border: 0,
    gap: theme.spacing(6),
    rowGap: theme.spacing(3),
    flex: 1,

    [theme.breakpoints.down("md")]: {
      display: "flex",
      flexDirection: "column",
      alignItems: "flex-start",
      gap: theme.spacing(1),
    },
  },

  statsItem: {
    flexDirection: "column",
    gap: 0,
    padding: 0,

    "& > span:first-of-type": {
      fontSize: 12,
      fontWeight: 500,
    },
  },
}))

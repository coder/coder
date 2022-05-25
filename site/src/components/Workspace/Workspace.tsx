import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import React from "react"
import * as TypesGen from "../../api/typesGenerated"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import { BuildsTable } from "../BuildsTable/BuildsTable"
import { Resources } from "../Resources/Resources"
import { Stack } from "../Stack/Stack"
import { WorkspaceActions } from "../WorkspaceActions/WorkspaceActions"
import { WorkspaceSchedule } from "../WorkspaceSchedule/WorkspaceSchedule"
import { WorkspaceSection } from "../WorkspaceSection/WorkspaceSection"
import { WorkspaceStats } from "../WorkspaceStats/WorkspaceStats"

export interface WorkspaceProps {
  handleStart: () => void
  handleStop: () => void
  handleUpdate: () => void
  handleCancel: () => void
  workspace: TypesGen.Workspace
  resources?: TypesGen.WorkspaceResource[]
  getResourcesError?: Error
  builds?: TypesGen.WorkspaceBuild[]
}

/**
 * Workspace is the top-level component for viewing an individual workspace
 */
export const Workspace: React.FC<WorkspaceProps> = ({
  handleStart,
  handleStop,
  handleUpdate,
  handleCancel,
  workspace,
  resources,
  getResourcesError,
  builds,
}) => {
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <div className={styles.header}>
        <div>
          <Typography variant="h4" className={styles.title}>
            {workspace.name}
          </Typography>

          <Typography color="textSecondary" className={styles.subtitle}>
            {workspace.owner_name}
          </Typography>
        </div>

        <div className={styles.headerActions}>
          <WorkspaceActions
            workspace={workspace}
            handleStart={handleStart}
            handleStop={handleStop}
            handleUpdate={handleUpdate}
            handleCancel={handleCancel}
          />
        </div>
      </div>

      <Stack direction="row" spacing={3} className={styles.layout}>
        <Stack spacing={3} className={styles.main}>
          <WorkspaceStats workspace={workspace} />
          <Resources resources={resources} getResourcesError={getResourcesError} workspace={workspace} />
          <WorkspaceSection title="Timeline" contentsProps={{ className: styles.timelineContents }}>
            <BuildsTable builds={builds} className={styles.timelineTable} />
          </WorkspaceSection>
        </Stack>

        <Stack spacing={3} className={styles.sidebar}>
          <WorkspaceSchedule workspace={workspace} />
        </Stack>
      </Stack>
    </div>
  )
}

export const useStyles = makeStyles((theme) => {
  return {
    root: {
      display: "flex",
      flexDirection: "column",
    },
    header: {
      paddingTop: theme.spacing(5),
      paddingBottom: theme.spacing(5),
      fontFamily: MONOSPACE_FONT_FAMILY,
      display: "flex",
      alignItems: "center",
    },
    headerActions: {
      marginLeft: "auto",
    },
    title: {
      fontWeight: 600,
      fontFamily: "inherit",
    },
    subtitle: {
      fontFamily: "inherit",
      marginTop: theme.spacing(0.5),
    },
    layout: {
      alignItems: "flex-start",
    },
    main: {
      width: "100%",
    },
    sidebar: {
      width: theme.spacing(32),
      flexShrink: 0,
    },
    timelineContents: {
      margin: 0,
    },
    timelineTable: {
      border: 0,
    },
  }
})

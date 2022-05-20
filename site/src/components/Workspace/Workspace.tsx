import { makeStyles } from "@material-ui/core/styles"
import React from "react"
import * as TypesGen from "../../api/typesGenerated"
import { WorkspaceStatus } from "../../util/workspace"
import { BuildsTable } from "../BuildsTable/BuildsTable"
import { Resources } from "../Resources/Resources"
import { WorkspaceSchedule } from "../WorkspaceSchedule/WorkspaceSchedule"
import { WorkspaceSection } from "../WorkspaceSection/WorkspaceSection"
import { WorkspaceStatusBar } from "../WorkspaceStatusBar/WorkspaceStatusBar"

export interface WorkspaceProps {
  handleStart: () => void
  handleStop: () => void
  handleRetry: () => void
  handleUpdate: () => void
  workspace: TypesGen.Workspace
  workspaceStatus: WorkspaceStatus
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
  handleRetry,
  handleUpdate,
  workspace,
  workspaceStatus,
  resources,
  getResourcesError,
  builds,
}) => {
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <div className={styles.vertical}>
        <WorkspaceStatusBar
          workspace={workspace}
          handleStart={handleStart}
          handleStop={handleStop}
          handleRetry={handleRetry}
          handleUpdate={handleUpdate}
          workspaceStatus={workspaceStatus}
        />
        <Resources resources={resources} getResourcesError={getResourcesError} />
        <div className={styles.horizontal}>
          <div className={styles.sidebarContainer}>
            <WorkspaceSchedule workspace={workspace} />

          </div>

          <div className={styles.timelineContainer}>
            <WorkspaceSection title="Timeline" contentsProps={{ className: styles.timelineContents }}>
              <BuildsTable builds={builds} className={styles.timelineTable} />
            </WorkspaceSection>
          </div>
        </div>
      </div>
    </div>
  )
}

export const useStyles = makeStyles(() => {
  return {
    root: {
      display: "flex",
      flexDirection: "column",
    },
    horizontal: {
      display: "flex",
      flexDirection: "row",
    },
    vertical: {
      display: "flex",
      flexDirection: "column",
    },
    sidebarContainer: {
      display: "flex",
      flexDirection: "column",
      flex: "0 0 350px",
    },
    timelineContainer: {
      flex: 1,
    },
    timelineContents: {
      margin: 0,
    },
    timelineTable: {
      border: 0,
    },
  }
})

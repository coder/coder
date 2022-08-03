import { makeStyles } from "@material-ui/core/styles"
import { WorkspaceStatusBadge } from "components/WorkspaceStatusBadge/WorkspaceStatusBadge"
import { FC } from "react"
import { useNavigate } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { BuildsTable } from "../BuildsTable/BuildsTable"
import { Margins } from "../Margins/Margins"
import { PageHeader, PageHeaderSubtitle, PageHeaderTitle } from "../PageHeader/PageHeader"
import { Resources } from "../Resources/Resources"
import { Stack } from "../Stack/Stack"
import { WorkspaceActions } from "../WorkspaceActions/WorkspaceActions"
import { WorkspaceDeletedBanner } from "../WorkspaceDeletedBanner/WorkspaceDeletedBanner"
import { WorkspaceScheduleBanner } from "../WorkspaceScheduleBanner/WorkspaceScheduleBanner"
import { WorkspaceScheduleButton } from "../WorkspaceScheduleButton/WorkspaceScheduleButton"
import { WorkspaceSection } from "../WorkspaceSection/WorkspaceSection"
import { WorkspaceStats } from "../WorkspaceStats/WorkspaceStats"

export interface WorkspaceProps {
  bannerProps: {
    isLoading?: boolean
    onExtend: () => void
  }
  scheduleProps: {
    onDeadlinePlus: () => void
    onDeadlineMinus: () => void
  }
  handleStart: () => void
  handleStop: () => void
  handleDelete: () => void
  handleUpdate: () => void
  handleCancel: () => void
  workspace: TypesGen.Workspace
  resources?: TypesGen.WorkspaceResource[]
  getResourcesError?: Error
  builds?: TypesGen.WorkspaceBuild[]
  canUpdateWorkspace: boolean
}

/**
 * Workspace is the top-level component for viewing an individual workspace
 */
export const Workspace: FC<React.PropsWithChildren<WorkspaceProps>> = ({
  bannerProps,
  scheduleProps,
  handleStart,
  handleStop,
  handleDelete,
  handleUpdate,
  handleCancel,
  workspace,
  resources,
  getResourcesError,
  builds,
  canUpdateWorkspace,
}) => {
  const styles = useStyles()
  const navigate = useNavigate()

  return (
    <Margins>
      <PageHeader
        actions={
          <Stack direction="row" spacing={1} className={styles.actions}>
            <WorkspaceScheduleButton
              workspace={workspace}
              onDeadlineMinus={scheduleProps.onDeadlineMinus}
              onDeadlinePlus={scheduleProps.onDeadlinePlus}
              canUpdateWorkspace={canUpdateWorkspace}
            />
            <WorkspaceActions
              workspace={workspace}
              handleStart={handleStart}
              handleStop={handleStop}
              handleDelete={handleDelete}
              handleUpdate={handleUpdate}
              handleCancel={handleCancel}
            />
          </Stack>
        }
      >
        <WorkspaceStatusBadge build={workspace.latest_build} className={styles.statusBadge} />
        <PageHeaderTitle>{workspace.name}</PageHeaderTitle>
        <PageHeaderSubtitle>{workspace.owner_name}</PageHeaderSubtitle>
      </PageHeader>

      <Stack direction="row" spacing={3}>
        <Stack direction="column" className={styles.firstColumnSpacer} spacing={3}>
          <WorkspaceScheduleBanner
            isLoading={bannerProps.isLoading}
            onExtend={bannerProps.onExtend}
            workspace={workspace}
          />

          <WorkspaceDeletedBanner
            workspace={workspace}
            handleClick={() => navigate(`/templates`)}
          />

          <WorkspaceStats workspace={workspace} handleUpdate={handleUpdate} />

          {!!resources && !!resources.length && (
            <Resources
              resources={resources}
              getResourcesError={getResourcesError}
              workspace={workspace}
              canUpdateWorkspace={canUpdateWorkspace}
            />
          )}

          <WorkspaceSection title="Logs" contentsProps={{ className: styles.timelineContents }}>
            <BuildsTable builds={builds} className={styles.timelineTable} />
          </WorkspaceSection>
        </Stack>
      </Stack>
    </Margins>
  )
}

const spacerWidth = 300

export const useStyles = makeStyles((theme) => {
  return {
    statusBadge: {
      marginBottom: theme.spacing(3),
    },

    actions: {
      [theme.breakpoints.down("sm")]: {
        flexDirection: "column",
      },
    },

    firstColumnSpacer: {
      flex: 2,
    },

    secondColumnSpacer: {
      flex: `0 0 ${spacerWidth}px`,
    },

    layout: {
      alignItems: "flex-start",
    },

    main: {
      width: "100%",
    },

    timelineContents: {
      margin: 0,
    },

    timelineTable: {
      border: 0,
    },
  }
})

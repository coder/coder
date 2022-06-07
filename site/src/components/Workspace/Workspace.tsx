import { makeStyles } from "@material-ui/core/styles"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import { BuildsTable } from "../BuildsTable/BuildsTable"
import { Margins } from "../Margins/Margins"
import { PageHeader, PageHeaderSubtitle, PageHeaderTitle } from "../PageHeader/PageHeader"
import { Resources } from "../Resources/Resources"
import { Stack } from "../Stack/Stack"
import { WorkspaceActions } from "../WorkspaceActions/WorkspaceActions"
import { WorkspaceSchedule } from "../WorkspaceSchedule/WorkspaceSchedule"
import { WorkspaceScheduleBanner } from "../WorkspaceScheduleBanner/WorkspaceScheduleBanner"
import { WorkspaceSection } from "../WorkspaceSection/WorkspaceSection"
import { WorkspaceStats } from "../WorkspaceStats/WorkspaceStats"

export interface WorkspaceProps {
  bannerProps: {
    isLoading?: boolean
    onExtend: () => void
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
}

/**
 * Workspace is the top-level component for viewing an individual workspace
 */
export const Workspace: FC<WorkspaceProps> = ({
  bannerProps,
  handleStart,
  handleStop,
  handleDelete,
  handleUpdate,
  handleCancel,
  workspace,
  resources,
  getResourcesError,
  builds,
}) => {
  const styles = useStyles()

  return (
    <Margins>
      <PageHeader
        actions={
          <WorkspaceActions
            workspace={workspace}
            handleStart={handleStart}
            handleStop={handleStop}
            handleDelete={handleDelete}
            handleUpdate={handleUpdate}
            handleCancel={handleCancel}
          />
        }
      >
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

          <WorkspaceStats workspace={workspace} />

          <Resources resources={resources} getResourcesError={getResourcesError} workspace={workspace} />

          <WorkspaceSection title="Timeline" contentsProps={{ className: styles.timelineContents }}>
            <BuildsTable builds={builds} className={styles.timelineTable} />
          </WorkspaceSection>
        </Stack>

        <Stack direction="column" className={styles.secondColumnSpacer} spacing={3}>
          <WorkspaceSchedule workspace={workspace} />
        </Stack>
      </Stack>
    </Margins>
  )
}

export const useStyles = makeStyles((theme) => {
  return {
    firstColumnSpacer: {
      flex: 2,
    },
    secondColumnSpacer: {
      flex: "0 0 300px",
    },
    header: {
      paddingTop: theme.spacing(5),
      paddingBottom: theme.spacing(5),
      fontFamily: MONOSPACE_FONT_FAMILY,
      display: "flex",
      alignItems: "center",
      justifyContent: "space-between",
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

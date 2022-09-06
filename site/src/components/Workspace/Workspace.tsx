import { makeStyles } from "@material-ui/core/styles"
import { ErrorSummary } from "components/ErrorSummary/ErrorSummary"
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

export enum WorkspaceErrors {
  GET_RESOURCES_ERROR = "getResourcesError",
  GET_BUILDS_ERROR = "getBuildsError",
  BUILD_ERROR = "buildError",
  CANCELLATION_ERROR = "cancellationError",
}

export interface WorkspaceProps {
  bannerProps: {
    isLoading?: boolean
    onExtend: () => void
  }
  scheduleProps: {
    onDeadlinePlus: () => void
    onDeadlineMinus: () => void
    deadlinePlusEnabled: () => boolean
    deadlineMinusEnabled: () => boolean
  }
  handleStart: () => void
  handleStop: () => void
  handleDelete: () => void
  handleUpdate: () => void
  handleCancel: () => void
  workspace: TypesGen.Workspace
  resources?: TypesGen.WorkspaceResource[]
  builds?: TypesGen.WorkspaceBuild[]
  canUpdateWorkspace: boolean
  workspaceErrors: Partial<Record<WorkspaceErrors, Error | unknown>>
  buildInfo?: TypesGen.BuildInfoResponse
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
  builds,
  canUpdateWorkspace,
  workspaceErrors,
  buildInfo,
}) => {
  const styles = useStyles()
  const navigate = useNavigate()

  const buildError = workspaceErrors[WorkspaceErrors.BUILD_ERROR] ? (
    <ErrorSummary error={workspaceErrors[WorkspaceErrors.BUILD_ERROR]} dismissible />
  ) : (
    <></>
  )
  const cancellationError = workspaceErrors[WorkspaceErrors.CANCELLATION_ERROR] ? (
    <ErrorSummary error={workspaceErrors[WorkspaceErrors.CANCELLATION_ERROR]} dismissible />
  ) : (
    <></>
  )

  return (
    <Margins>
      <PageHeader
        actions={
          <Stack direction="row" spacing={1} className={styles.actions}>
            <WorkspaceScheduleButton
              workspace={workspace}
              onDeadlineMinus={scheduleProps.onDeadlineMinus}
              onDeadlinePlus={scheduleProps.onDeadlinePlus}
              deadlineMinusEnabled={scheduleProps.deadlineMinusEnabled}
              deadlinePlusEnabled={scheduleProps.deadlinePlusEnabled}
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

      <Stack direction="column" className={styles.firstColumnSpacer} spacing={2.5}>
        {buildError}
        {cancellationError}

        <WorkspaceScheduleBanner
          isLoading={bannerProps.isLoading}
          onExtend={bannerProps.onExtend}
          workspace={workspace}
        />

        <WorkspaceDeletedBanner workspace={workspace} handleClick={() => navigate(`/templates`)} />

        <WorkspaceStats workspace={workspace} handleUpdate={handleUpdate} />

        {!!resources && !!resources.length && (
          <Resources
            resources={resources}
            getResourcesError={workspaceErrors[WorkspaceErrors.GET_RESOURCES_ERROR]}
            workspace={workspace}
            canUpdateWorkspace={canUpdateWorkspace}
            buildInfo={buildInfo}
          />
        )}

        <WorkspaceSection title="Logs" contentsProps={{ className: styles.timelineContents }}>
          {workspaceErrors[WorkspaceErrors.GET_BUILDS_ERROR] ? (
            <ErrorSummary error={workspaceErrors[WorkspaceErrors.GET_BUILDS_ERROR]} />
          ) : (
            <BuildsTable builds={builds} className={styles.timelineTable} />
          )}
        </WorkspaceSection>
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

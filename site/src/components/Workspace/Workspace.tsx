import { makeStyles } from "@material-ui/core/styles"
import { WorkspaceStatusBadge } from "components/WorkspaceStatusBadge/WorkspaceStatusBadge"
import { FC } from "react"
import { useNavigate } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { BuildsTable } from "../BuildsTable/BuildsTable"
import { Margins } from "../Margins/Margins"
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "../PageHeader/PageHeader"
import { Resources } from "../Resources/Resources"
import { Stack } from "../Stack/Stack"
import { WorkspaceActions } from "../WorkspaceActions/WorkspaceActions"
import { WorkspaceDeletedBanner } from "../WorkspaceDeletedBanner/WorkspaceDeletedBanner"
import { WorkspaceScheduleBanner } from "../WorkspaceScheduleBanner/WorkspaceScheduleBanner"
import { WorkspaceScheduleButton } from "../WorkspaceScheduleButton/WorkspaceScheduleButton"
import { WorkspaceSection } from "../WorkspaceSection/WorkspaceSection"
import { WorkspaceStats } from "../WorkspaceStats/WorkspaceStats"
import { AlertBanner } from "../AlertBanner/AlertBanner"
import { useTranslation } from "react-i18next"
import {
  EstimateTransitionTime,
  WorkspaceBuildProgress,
} from "components/WorkspaceBuildProgress/WorkspaceBuildProgress"

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
    onDeadlinePlus: (hours: number) => void
    onDeadlineMinus: (hours: number) => void
    deadlinePlusEnabled: () => boolean
    deadlineMinusEnabled: () => boolean
    maxDeadlineIncrease: number
    maxDeadlineDecrease: number
  }
  handleStart: () => void
  handleStop: () => void
  handleDelete: () => void
  handleUpdate: () => void
  handleCancel: () => void
  isUpdating: boolean
  workspace: TypesGen.Workspace
  resources?: TypesGen.WorkspaceResource[]
  builds?: TypesGen.WorkspaceBuild[]
  canUpdateWorkspace: boolean
  hideSSHButton?: boolean
  workspaceErrors: Partial<Record<WorkspaceErrors, Error | unknown>>
  buildInfo?: TypesGen.BuildInfoResponse
  applicationsHost?: string
  template?: TypesGen.Template
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
  isUpdating,
  resources,
  builds,
  canUpdateWorkspace,
  workspaceErrors,
  hideSSHButton,
  buildInfo,
  applicationsHost,
  template,
}) => {
  const { t } = useTranslation("workspacePage")
  const styles = useStyles()
  const navigate = useNavigate()
  const hasTemplateIcon =
    workspace.template_icon && workspace.template_icon !== ""

  const buildError = Boolean(workspaceErrors[WorkspaceErrors.BUILD_ERROR]) && (
    <AlertBanner
      severity="error"
      error={workspaceErrors[WorkspaceErrors.BUILD_ERROR]}
      dismissible
    />
  )

  const cancellationError = Boolean(
    workspaceErrors[WorkspaceErrors.CANCELLATION_ERROR],
  ) && (
    <AlertBanner
      severity="error"
      error={workspaceErrors[WorkspaceErrors.CANCELLATION_ERROR]}
      dismissible
    />
  )

  const workspaceRefreshWarning = Boolean(
    workspaceErrors[WorkspaceErrors.GET_RESOURCES_ERROR],
  ) && (
    <AlertBanner
      severity="warning"
      text={t("warningsAndErrors.workspaceRefreshWarning")}
      dismissible
    />
  )

  let buildTimeEstimate: number | undefined = undefined
  let isTransitioning: boolean | undefined = undefined
  if (template !== undefined) {
    ;[buildTimeEstimate, isTransitioning] = EstimateTransitionTime(
      template,
      workspace,
    )
  }

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
              maxDeadlineDecrease={scheduleProps.maxDeadlineDecrease}
              maxDeadlineIncrease={scheduleProps.maxDeadlineIncrease}
              canUpdateWorkspace={canUpdateWorkspace}
            />
            <WorkspaceActions
              workspaceStatus={workspace.latest_build.status}
              isOutdated={workspace.outdated}
              handleStart={handleStart}
              handleStop={handleStop}
              handleDelete={handleDelete}
              handleUpdate={handleUpdate}
              handleCancel={handleCancel}
              isUpdating={isUpdating}
            />
          </Stack>
        }
      >
        <Stack direction="row" spacing={3} alignItems="center">
          {hasTemplateIcon && (
            <img
              alt=""
              src={workspace.template_icon}
              className={styles.templateIcon}
            />
          )}
          <div>
            <PageHeaderTitle>
              {workspace.name}
              <WorkspaceStatusBadge
                build={workspace.latest_build}
                className={styles.statusBadge}
              />
            </PageHeaderTitle>
            <PageHeaderSubtitle condensed>
              {workspace.owner_name}
            </PageHeaderSubtitle>
          </div>
        </Stack>
      </PageHeader>

      <Stack
        direction="column"
        className={styles.firstColumnSpacer}
        spacing={4}
      >
        {buildError}
        {cancellationError}
        {workspaceRefreshWarning}

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

        {isTransitioning !== undefined && isTransitioning && (
          <WorkspaceBuildProgress
            workspace={workspace}
            buildEstimate={buildTimeEstimate}
          />
        )}

        {typeof resources !== "undefined" && resources.length > 0 && (
          <Resources
            resources={resources}
            getResourcesError={
              workspaceErrors[WorkspaceErrors.GET_RESOURCES_ERROR]
            }
            workspace={workspace}
            canUpdateWorkspace={canUpdateWorkspace}
            buildInfo={buildInfo}
            hideSSHButton={hideSSHButton}
            applicationsHost={applicationsHost}
          />
        )}

        <WorkspaceSection
          contentsProps={{ className: styles.timelineContents }}
        >
          {workspaceErrors[WorkspaceErrors.GET_BUILDS_ERROR] ? (
            <AlertBanner
              severity="error"
              error={workspaceErrors[WorkspaceErrors.GET_BUILDS_ERROR]}
            />
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
      marginLeft: theme.spacing(2),
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

    templateIcon: {
      width: theme.spacing(6),
      height: theme.spacing(6),
    },

    timelineContents: {
      margin: 0,
    },

    timelineTable: {
      border: 0,
    },
  }
})

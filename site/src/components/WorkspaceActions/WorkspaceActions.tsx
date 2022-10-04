import { DropdownButton } from "components/DropdownButton/DropdownButton"
import { FC, ReactNode, useMemo } from "react"
import { useTranslation } from "react-i18next"
import { Workspace, WorkspaceStatus } from "../../api/typesGenerated"
import {
  ActionLoadingButton,
  DeleteButton,
  DisabledButton,
  StartButton,
  StopButton,
  UpdateButton,
} from "../DropdownButton/ActionCtas"
import { ButtonMapping, ButtonTypesEnum, WorkspaceStateActions } from "./constants"

/**
 * Jobs submitted while another job is in progress will be discarded,
 * so check whether workspace job status has reached completion (whether successful or not).
 */
const canAcceptJobs = (workspaceStatus: WorkspaceStatus) =>
  ["running", "stopped", "deleted", "failed", "canceled"].includes(workspaceStatus)

export interface WorkspaceActionsProps {
  workspace: Workspace
  handleStart: () => void
  handleStop: () => void
  handleDelete: () => void
  handleUpdate: () => void
  handleCancel: () => void
  isUpdating: boolean
  children?: ReactNode
}

export const WorkspaceActions: FC<WorkspaceActionsProps> = ({
  workspace,
  handleStart,
  handleStop,
  handleDelete,
  handleUpdate,
  handleCancel,
  isUpdating,
}) => {
  const { t } = useTranslation("workspacePage")
  const workspaceStatus = workspace.latest_build.status

  const canBeUpdated = workspace.outdated && canAcceptJobs(workspaceStatus)

  // actions are the primary and secondary CTAs that appear in the workspace actions dropdown
  const actions = useMemo(() => {
    if (!canBeUpdated) {
      return WorkspaceStateActions[workspaceStatus]
    }

    // if an update is available, we make the update button the primary CTA
    // and move the former primary CTA to the secondary actions list
    const updatedActions = { ...WorkspaceStateActions[workspaceStatus] }
    updatedActions.secondary = [updatedActions.primary, ...updatedActions.secondary]
    updatedActions.primary = ButtonTypesEnum.update

    return updatedActions
  }, [canBeUpdated, workspaceStatus])

  // A mapping of button type to the corresponding React component
  const buttonMapping: ButtonMapping = {
    [ButtonTypesEnum.update]: <UpdateButton handleAction={handleUpdate} />,
    [ButtonTypesEnum.updating]: <ActionLoadingButton label={t("actionButton.updating")} />,
    [ButtonTypesEnum.start]: <StartButton handleAction={handleStart} />,
    [ButtonTypesEnum.starting]: <ActionLoadingButton label={t("actionButton.starting")} />,
    [ButtonTypesEnum.stop]: <StopButton handleAction={handleStop} />,
    [ButtonTypesEnum.stopping]: <ActionLoadingButton label={t("actionButton.stopping")} />,
    [ButtonTypesEnum.delete]: <DeleteButton handleAction={handleDelete} />,
    [ButtonTypesEnum.deleting]: <ActionLoadingButton label={t("actionButton.deleting")} />,
    [ButtonTypesEnum.canceling]: <DisabledButton workspaceStatus={workspaceStatus} />,
    [ButtonTypesEnum.disabled]: <DisabledButton workspaceStatus={workspaceStatus} />,
    [ButtonTypesEnum.pending]: <DisabledButton workspaceStatus={workspaceStatus} />,
  }

  return (
    <DropdownButton
      primaryAction={
        isUpdating ? buttonMapping[ButtonTypesEnum.updating] : buttonMapping[actions.primary]
      }
      canCancel={actions.canCancel}
      handleCancel={handleCancel}
      secondaryActions={actions.secondary.map((action) => ({
        action,
        button: buttonMapping[action],
      }))}
    />
  )
}

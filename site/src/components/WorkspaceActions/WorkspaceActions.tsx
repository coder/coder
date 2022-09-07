import { DropdownButton } from "components/DropdownButton/DropdownButton"
import { FC, ReactNode, useMemo } from "react"
import { getWorkspaceStatus, WorkspaceStateEnum, WorkspaceStatus } from "util/workspace"
import { Workspace } from "../../api/typesGenerated"
import {
  ActionLoadingButton,
  DeleteButton,
  DisabledButton,
  Language,
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
  ["started", "stopped", "deleted", "error", "canceled"].includes(workspaceStatus)

export interface WorkspaceActionsProps {
  workspace: Workspace
  handleStart: () => void
  handleStop: () => void
  handleDelete: () => void
  handleUpdate: () => void
  handleCancel: () => void
  children?: ReactNode
}

export const WorkspaceActions: FC<WorkspaceActionsProps> = ({
  workspace,
  handleStart,
  handleStop,
  handleDelete,
  handleUpdate,
  handleCancel,
}) => {
  const workspaceStatus: keyof typeof WorkspaceStateEnum = getWorkspaceStatus(
    workspace.latest_build,
  )
  const workspaceState = WorkspaceStateEnum[workspaceStatus]

  const canBeUpdated = workspace.outdated && canAcceptJobs(workspaceStatus)

  // actions are the primary and secondary CTAs that appear in the workspace actions dropdown
  const actions = useMemo(() => {
    if (!canBeUpdated) {
      return WorkspaceStateActions[workspaceState]
    }

    // if an update is available, we make the update button the primary CTA
    // and move the former primary CTA to the secondary actions list
    const updatedActions = { ...WorkspaceStateActions[workspaceState] }
    updatedActions.secondary = [updatedActions.primary, ...updatedActions.secondary]
    updatedActions.primary = ButtonTypesEnum.update

    return updatedActions
  }, [canBeUpdated, workspaceState])

  // A mapping of button type to the corresponding React component
  const buttonMapping: ButtonMapping = {
    [ButtonTypesEnum.update]: <UpdateButton handleAction={handleUpdate} />,
    [ButtonTypesEnum.start]: <StartButton handleAction={handleStart} />,
    [ButtonTypesEnum.starting]: <ActionLoadingButton label={Language.starting} />,
    [ButtonTypesEnum.stop]: <StopButton handleAction={handleStop} />,
    [ButtonTypesEnum.stopping]: <ActionLoadingButton label={Language.stopping} />,
    [ButtonTypesEnum.delete]: <DeleteButton handleAction={handleDelete} />,
    [ButtonTypesEnum.deleting]: <ActionLoadingButton label={Language.deleting} />,
    [ButtonTypesEnum.canceling]: <DisabledButton workspaceState={workspaceState} />,
    [ButtonTypesEnum.disabled]: <DisabledButton workspaceState={workspaceState} />,
    [ButtonTypesEnum.queued]: <DisabledButton workspaceState={workspaceState} />,
    [ButtonTypesEnum.loading]: <DisabledButton workspaceState={workspaceState} />,
  }

  return (
    <DropdownButton
      primaryAction={buttonMapping[actions.primary]}
      canCancel={actions.canCancel}
      handleCancel={handleCancel}
      secondaryActions={actions.secondary.map((action) => ({
        action,
        button: buttonMapping[action],
      }))}
    />
  )
}

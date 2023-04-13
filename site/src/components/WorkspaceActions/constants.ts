import { WorkspaceStatus } from "api/typesGenerated"
import { ReactNode } from "react"

// the button types we have
export enum ButtonTypesEnum {
  start = "start",
  starting = "starting",
  stop = "stop",
  stopping = "stopping",
  delete = "delete",
  deleting = "deleting",
  update = "update",
  updating = "updating",
  settings = "settings",
  changeVersion = "changeVersion",
  // disabled buttons
  canceling = "canceling",
  deleted = "deleted",
  pending = "pending",
}

export type ButtonMapping = {
  [key in ButtonTypesEnum]: ReactNode
}

interface WorkspaceAbilities {
  primaryActions: ButtonTypesEnum[]
  secondaryActions?: ButtonTypesEnum[]
  canCancel: boolean
  canAcceptJobs: boolean
}

export const buttonAbilities = (
  status: WorkspaceStatus,
): WorkspaceAbilities => {
  return statusToAbilities[status]
}

const defaultSecondaryActions = [
  ButtonTypesEnum.settings,
  ButtonTypesEnum.changeVersion,
  ButtonTypesEnum.delete,
]

const statusToAbilities: Record<WorkspaceStatus, WorkspaceAbilities> = {
  starting: {
    primaryActions: [ButtonTypesEnum.starting],
    canCancel: true,
    canAcceptJobs: false,
  },
  running: {
    primaryActions: [ButtonTypesEnum.stop],
    secondaryActions: defaultSecondaryActions,
    canCancel: false,
    canAcceptJobs: true,
  },
  stopping: {
    primaryActions: [ButtonTypesEnum.stopping],
    canCancel: true,
    canAcceptJobs: false,
  },
  stopped: {
    primaryActions: [ButtonTypesEnum.start],
    secondaryActions: defaultSecondaryActions,
    canCancel: false,
    canAcceptJobs: true,
  },
  canceled: {
    primaryActions: [ButtonTypesEnum.start, ButtonTypesEnum.stop],
    secondaryActions: defaultSecondaryActions,
    canCancel: false,
    canAcceptJobs: true,
  },
  // in the case of an error
  failed: {
    primaryActions: [ButtonTypesEnum.start, ButtonTypesEnum.stop],
    secondaryActions: defaultSecondaryActions,
    canCancel: false,
    canAcceptJobs: true,
  },
  /**
   * disabled states
   */
  canceling: {
    primaryActions: [ButtonTypesEnum.canceling],
    canCancel: false,
    canAcceptJobs: false,
  },
  deleting: {
    primaryActions: [ButtonTypesEnum.deleting],
    canCancel: true,
    canAcceptJobs: false,
  },
  deleted: {
    primaryActions: [ButtonTypesEnum.deleted],
    canCancel: false,
    canAcceptJobs: true,
  },
  pending: {
    primaryActions: [ButtonTypesEnum.pending],
    canCancel: false,
    canAcceptJobs: false,
  },
}

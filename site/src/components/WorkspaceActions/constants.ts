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
  // disabled buttons
  canceling = "canceling",
  deleted = "deleted",
  pending = "pending",
}

export type ButtonMapping = {
  [key in ButtonTypesEnum]: ReactNode
}

interface WorkspaceAbilities {
  actions: ButtonTypesEnum[]
  canCancel: boolean
  canAcceptJobs: boolean
}

export const buttonAbilities = (
  status: WorkspaceStatus,
): WorkspaceAbilities => {
  return statusToAbilities[status]
}

const statusToAbilities: Record<WorkspaceStatus, WorkspaceAbilities> = {
  starting: {
    actions: [ButtonTypesEnum.starting],
    canCancel: true,
    canAcceptJobs: false,
  },
  running: {
    actions: [
      ButtonTypesEnum.stop,
      ButtonTypesEnum.settings,
      ButtonTypesEnum.delete,
    ],
    canCancel: false,
    canAcceptJobs: true,
  },
  stopping: {
    actions: [ButtonTypesEnum.stopping],
    canCancel: true,
    canAcceptJobs: false,
  },
  stopped: {
    actions: [
      ButtonTypesEnum.start,
      ButtonTypesEnum.settings,
      ButtonTypesEnum.delete,
    ],
    canCancel: false,
    canAcceptJobs: true,
  },
  canceled: {
    actions: [
      ButtonTypesEnum.start,
      ButtonTypesEnum.stop,
      ButtonTypesEnum.settings,
      ButtonTypesEnum.delete,
    ],
    canCancel: false,
    canAcceptJobs: true,
  },
  // in the case of an error
  failed: {
    actions: [
      ButtonTypesEnum.start,
      ButtonTypesEnum.stop,
      ButtonTypesEnum.settings,
      ButtonTypesEnum.delete,
    ],
    canCancel: false,
    canAcceptJobs: true,
  },
  /**
   * disabled states
   */
  canceling: {
    actions: [ButtonTypesEnum.canceling],
    canCancel: false,
    canAcceptJobs: false,
  },
  deleting: {
    actions: [ButtonTypesEnum.deleting],
    canCancel: true,
    canAcceptJobs: false,
  },
  deleted: {
    actions: [ButtonTypesEnum.deleted],
    canCancel: false,
    canAcceptJobs: true,
  },
  pending: {
    actions: [ButtonTypesEnum.pending],
    canCancel: false,
    canAcceptJobs: false,
  },
}

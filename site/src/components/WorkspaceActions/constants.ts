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
  // disabled buttons
  canceling = "canceling",
  deleted = "deleted",
  pending = "pending",
}

export type ButtonMapping = {
  [key in ButtonTypesEnum]: ReactNode
}

type StateActionsType = Record<
  WorkspaceStatus,
  {
    primary: ButtonTypesEnum
    secondary: ButtonTypesEnum[]
    canCancel: boolean
  }
>

// A mapping of workspace state to button type
// 'Primary' actions are the main ctas
// 'Secondary' actions are ctas housed within the popover
export const WorkspaceStateActions: StateActionsType = {
  starting: {
    primary: ButtonTypesEnum.starting,
    secondary: [],
    canCancel: true,
  },
  running: {
    primary: ButtonTypesEnum.stop,
    secondary: [ButtonTypesEnum.delete],
    canCancel: false,
  },
  stopping: {
    primary: ButtonTypesEnum.stopping,
    secondary: [],
    canCancel: true,
  },
  stopped: {
    primary: ButtonTypesEnum.start,
    secondary: [ButtonTypesEnum.delete],
    canCancel: false,
  },
  canceled: {
    primary: ButtonTypesEnum.start,
    secondary: [ButtonTypesEnum.stop, ButtonTypesEnum.delete],
    canCancel: false,
  },
  // in the case of an error
  failed: {
    primary: ButtonTypesEnum.start, // give the user the ability to start a workspace again
    secondary: [ButtonTypesEnum.delete], // allows the user to delete
    canCancel: false,
  },
  /**
   * disabled states
   */
  canceling: {
    primary: ButtonTypesEnum.canceling,
    secondary: [],
    canCancel: false,
  },
  deleting: {
    primary: ButtonTypesEnum.deleting,
    secondary: [],
    canCancel: true,
  },
  deleted: {
    primary: ButtonTypesEnum.deleted,
    secondary: [],
    canCancel: false,
  },
  pending: {
    primary: ButtonTypesEnum.pending,
    secondary: [],
    canCancel: false,
  },
}

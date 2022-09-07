import { ReactNode } from "react"
import { WorkspaceStateEnum } from "util/workspace"

// the button types we have
export enum ButtonTypesEnum {
  start = "start",
  starting = "starting",
  stop = "stop",
  stopping = "stopping",
  delete = "delete",
  deleting = "deleting",
  update = "update",
  // disabled buttons
  canceling = "canceling",
  disabled = "disabled",
  queued = "queued",
  loading = "loading",
}

export type ButtonMapping = {
  [key in ButtonTypesEnum]: ReactNode
}

type StateActionsType = {
  [key in WorkspaceStateEnum]: {
    primary: ButtonTypesEnum
    secondary: ButtonTypesEnum[]
    canCancel: boolean
  }
}

// A mapping of workspace state to button type
// 'Primary' actions are the main ctas
// 'Secondary' actions are ctas housed within the popover
export const WorkspaceStateActions: StateActionsType = {
  [WorkspaceStateEnum.starting]: {
    primary: ButtonTypesEnum.starting,
    secondary: [],
    canCancel: true,
  },
  [WorkspaceStateEnum.started]: {
    primary: ButtonTypesEnum.stop,
    secondary: [ButtonTypesEnum.delete],
    canCancel: false,
  },
  [WorkspaceStateEnum.stopping]: {
    primary: ButtonTypesEnum.stopping,
    secondary: [],
    canCancel: true,
  },
  [WorkspaceStateEnum.stopped]: {
    primary: ButtonTypesEnum.start,
    secondary: [ButtonTypesEnum.delete],
    canCancel: false,
  },
  [WorkspaceStateEnum.canceled]: {
    primary: ButtonTypesEnum.start,
    secondary: [ButtonTypesEnum.stop, ButtonTypesEnum.delete],
    canCancel: false,
  },
  // in the case of an error
  [WorkspaceStateEnum.error]: {
    primary: ButtonTypesEnum.start, // give the user the ability to start a workspace again
    secondary: [ButtonTypesEnum.delete], // allows the user to delete
    canCancel: false,
  },
  /**
   * disabled states
   */
  [WorkspaceStateEnum.canceling]: {
    primary: ButtonTypesEnum.canceling,
    secondary: [],
    canCancel: false,
  },
  [WorkspaceStateEnum.deleting]: {
    primary: ButtonTypesEnum.deleting,
    secondary: [],
    canCancel: true,
  },
  [WorkspaceStateEnum.deleted]: {
    primary: ButtonTypesEnum.disabled,
    secondary: [],
    canCancel: false,
  },
  [WorkspaceStateEnum.queued]: {
    primary: ButtonTypesEnum.queued,
    secondary: [],
    canCancel: false,
  },
  [WorkspaceStateEnum.loading]: {
    primary: ButtonTypesEnum.loading,
    secondary: [],
    canCancel: false,
  },
}

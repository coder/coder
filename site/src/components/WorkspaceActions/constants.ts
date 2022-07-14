// all the possible states returned by the API
export enum WorkspaceStateEnum {
  starting = "Starting",
  started = "Started",
  stopping = "Stopping",
  stopped = "Stopped",
  canceling = "Canceling",
  canceled = "Canceled",
  deleting = "Deleting",
  deleted = "Deleted",
  queued = "Queued",
  error = "Error",
  loading = "Loading",
}

// the button types we have
export enum ButtonTypesEnum {
  start,
  stop,
  delete,
  update,
  cancel,
  error,
  // disabled buttons
  canceling,
  disabled,
  queued,
  loading,
}

type StateActionsType = {
  [key in WorkspaceStateEnum]: {
    primary: ButtonTypesEnum
    secondary: ButtonTypesEnum[]
  }
}

// A mapping of workspace state to button type
// 'Primary' actions are the main ctas
// 'Secondary' actions are ctas housed within the popover
export const WorkspaceStateActions: StateActionsType = {
  [WorkspaceStateEnum.starting]: {
    primary: ButtonTypesEnum.cancel,
    secondary: [],
  },
  [WorkspaceStateEnum.started]: {
    primary: ButtonTypesEnum.stop,
    secondary: [ButtonTypesEnum.delete, ButtonTypesEnum.update],
  },
  [WorkspaceStateEnum.stopping]: {
    primary: ButtonTypesEnum.cancel,
    secondary: [],
  },
  [WorkspaceStateEnum.stopped]: {
    primary: ButtonTypesEnum.start,
    secondary: [ButtonTypesEnum.delete, ButtonTypesEnum.update],
  },
  [WorkspaceStateEnum.canceled]: {
    primary: ButtonTypesEnum.start,
    secondary: [ButtonTypesEnum.stop, ButtonTypesEnum.delete, ButtonTypesEnum.update],
  },
  // in the case of an error
  [WorkspaceStateEnum.error]: {
    primary: ButtonTypesEnum.start, // give the user the ability to start a workspace again
    secondary: [ButtonTypesEnum.delete, ButtonTypesEnum.update], // allows the user to delete or update
  },
  /**
   * disabled states
   */
  [WorkspaceStateEnum.canceling]: {
    primary: ButtonTypesEnum.canceling,
    secondary: [],
  },
  [WorkspaceStateEnum.deleting]: {
    primary: ButtonTypesEnum.cancel,
    secondary: [],
  },
  [WorkspaceStateEnum.deleted]: {
    primary: ButtonTypesEnum.disabled,
    secondary: [],
  },
  [WorkspaceStateEnum.queued]: {
    primary: ButtonTypesEnum.queued,
    secondary: [],
  },
  [WorkspaceStateEnum.loading]: {
    primary: ButtonTypesEnum.loading,
    secondary: [],
  },
}

import { State } from "xstate"
import { WorkspaceBuildTransition } from "../../api/types"
import { WorkspaceStatus } from "../../pages/WorkspacePage/WorkspacePage"
import { WorkspaceContext, WorkspaceEvent } from "./workspaceXService"

const inProgressToStatus: Record<WorkspaceBuildTransition, WorkspaceStatus> = {
  start: "starting",
  stop: "stopping",
  delete: "deleting",
}

const succeededToStatus: Record<WorkspaceBuildTransition, WorkspaceStatus> = {
  start: "started",
  stop: "stopped",
  delete: "deleted",
}

export const selectWorkspaceStatus = (state: State<WorkspaceContext, WorkspaceEvent>): WorkspaceStatus => {
  const transition = state.context.workspace?.latest_build.transition as WorkspaceBuildTransition
  const jobStatus = state.context.workspace?.latest_build.job.status
  switch (jobStatus) {
    case undefined:
      return "loading"
    case "succeeded":
      return succeededToStatus[transition]
    case "pending":
      return inProgressToStatus[transition]
    case "running":
      return inProgressToStatus[transition]
    case "canceling":
      return "canceling"
    case "canceled":
      return "error"
    case "failed":
      return "error"
  }
}

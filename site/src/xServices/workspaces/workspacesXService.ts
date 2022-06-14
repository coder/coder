import { ActorRefFrom, assign, createMachine, spawn } from "xstate"
import * as API from "../../api/api"
import { getErrorMessage } from "../../api/errors"
import * as TypesGen from "../../api/typesGenerated"
import { displayError, displayMsg, displaySuccess } from "../../components/GlobalSnackbar/utils"
import { workspaceQueryToFilter } from "../../util/workspace"

/**
 * Workspace item machine
 *
 * It is used to control the state and actions of each workspace in the
 * workspaces page view
 **/
interface WorkspaceItemContext {
  data: TypesGen.Workspace
  updatedTemplate?: TypesGen.Template
}

type WorkspaceItemEvent = {
  type: "UPDATE_VERSION"
}

export const workspaceItemMachine = createMachine(
  {
    id: "workspaceItemMachine",
    schema: {
      context: {} as WorkspaceItemContext,
      events: {} as WorkspaceItemEvent,
      services: {} as {
        getTemplate: {
          data: TypesGen.Template
        }
        startWorkspace: {
          data: TypesGen.WorkspaceBuild
        }
        getWorkspace: {
          data: TypesGen.Workspace
        }
      },
    },
    tsTypes: {} as import("./workspacesXService.typegen").Typegen0,
    type: "parallel",
    states: {
      updateVersion: {
        initial: "idle",
        states: {
          idle: {
            on: {
              UPDATE_VERSION: {
                target: "gettingUpdatedTemplate",
                // We can improve the UI by optimistically updating the workspace status
                // to "Queued" so the UI can display the updated state right away and we
                // don't need to display an extra spinner.
                actions: ["assignQueuedStatus", "displayUpdatingVersionMessage"],
              },
            },
          },
          gettingUpdatedTemplate: {
            invoke: {
              id: "getTemplate",
              src: "getTemplate",
              onDone: {
                actions: "assignUpdatedTemplate",
                target: "restartingWorkspace",
              },
              onError: {
                target: "error",
                actions: "displayUpdateVersionError",
              },
            },
          },
          restartingWorkspace: {
            invoke: {
              id: "startWorkspace",
              src: "startWorkspace",
              onDone: {
                actions: "assignLatestBuild",
                target: "waitingToBeUpdated",
              },
              onError: {
                target: "error",
                actions: "displayUpdateVersionError",
              },
            },
          },
          waitingToBeUpdated: {
            after: {
              5000: "gettingUpdatedWorkspaceData",
            },
          },
          gettingUpdatedWorkspaceData: {
            invoke: {
              id: "getWorkspace",
              src: "getWorkspace",
              onDone: [
                {
                  target: "waitingToBeUpdated",
                  cond: "isOutdated",
                  actions: ["assignUpdatedData"],
                },
                {
                  target: "success",
                  actions: ["assignUpdatedData", "displayUpdatedSuccessMessage"],
                },
              ],
            },
          },
          error: {
            type: "final",
          },
          success: {
            type: "final",
          },
        },
      },
    },
  },
  {
    guards: {
      isOutdated: (_, event) => event.data.outdated,
    },
    services: {
      getTemplate: (context) => API.getTemplate(context.data.template_id),
      startWorkspace: (context) => {
        if (!context.updatedTemplate) {
          throw new Error("Updated template is not loaded.")
        }

        return API.startWorkspace(context.data.id, context.updatedTemplate.active_version_id)
      },
      getWorkspace: (context) => API.getWorkspace(context.data.id),
    },
    actions: {
      assignUpdatedTemplate: assign({
        updatedTemplate: (_, event) => event.data,
      }),
      assignLatestBuild: assign({
        data: (context, event) => {
          return {
            ...context.data,
            latest_build: event.data,
          }
        },
      }),
      displayUpdateVersionError: (_, event) => {
        const message = getErrorMessage(event.data, "Error on update workspace version.")
        displayError(message)
      },
      displayUpdatingVersionMessage: () => {
        displayMsg("Updating workspace...")
      },
      assignQueuedStatus: assign({
        data: (ctx) => {
          return {
            ...ctx.data,
            latest_build: {
              ...ctx.data.latest_build,
              job: {
                ...ctx.data.latest_build.job,
                status: "pending" as TypesGen.ProvisionerJobStatus,
              },
            },
          }
        },
      }),
      displayUpdatedSuccessMessage: () => {
        displaySuccess("Workspace updated successfully.")
      },
      assignUpdatedData: assign({
        data: (_, event) => event.data,
      }),
    },
  },
)

/**
 * Workspaces machine
 *
 * It is used to control the state of the workspace list
 **/

export type WorkspaceItemMachineRef = ActorRefFrom<typeof workspaceItemMachine>

interface WorkspacesContext {
  workspaceRefs?: WorkspaceItemMachineRef[]
  filter?: string
  getWorkspacesError?: Error | unknown
}

type WorkspacesEvent = { type: "SET_FILTER"; query: string } | { type: "UPDATE_VERSION"; workspaceId: string }

export const workspacesMachine = createMachine(
  {
    tsTypes: {} as import("./workspacesXService.typegen").Typegen1,
    schema: {
      context: {} as WorkspacesContext,
      events: {} as WorkspacesEvent,
      services: {} as {
        getWorkspaces: {
          data: TypesGen.Workspace[]
        }
      },
    },
    id: "workspaceState",
    initial: "ready",
    states: {
      ready: {
        on: {
          SET_FILTER: "extractingFilter",
          UPDATE_VERSION: {
            actions: "triggerUpdateVersion",
          },
        },
      },
      extractingFilter: {
        entry: "assignFilter",
        always: {
          target: "gettingWorkspaces",
        },
      },
      gettingWorkspaces: {
        entry: "clearGetWorkspacesError",
        invoke: {
          src: "getWorkspaces",
          id: "getWorkspaces",
          onDone: {
            target: "ready",
            actions: ["assignWorkspaceRefs", "clearGetWorkspacesError"],
          },
          onError: {
            target: "ready",
            actions: ["assignGetWorkspacesError", "clearWorkspaces"],
          },
        },
        tags: "loading",
      },
    },
  },
  {
    actions: {
      assignWorkspaceRefs: assign({
        workspaceRefs: (_, event) =>
          event.data.map((data) => {
            return spawn(workspaceItemMachine.withContext({ data }), data.id)
          }),
      }),
      assignFilter: assign({
        filter: (_, event) => event.query,
      }),
      assignGetWorkspacesError: assign({
        getWorkspacesError: (_, event) => event.data,
      }),
      clearGetWorkspacesError: (context) => assign({ ...context, getWorkspacesError: undefined }),
      clearWorkspaces: (context) => assign({ ...context, workspaces: undefined }),
      triggerUpdateVersion: (context, event) => {
        const workspaceRef = context.workspaceRefs?.find((ref) => ref.id === event.workspaceId)

        if (!workspaceRef) {
          throw new Error(`No workspace ref found for ${event.workspaceId}.`)
        }

        workspaceRef.send("UPDATE_VERSION")
      },
    },
    services: {
      getWorkspaces: (context) => API.getWorkspaces(workspaceQueryToFilter(context.filter)),
    },
  },
)

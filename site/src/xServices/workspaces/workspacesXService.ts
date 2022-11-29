import { getPaginationData } from "components/PaginationWidget/utils"
import {
  PaginationContext,
  paginationMachine,
  PaginationMachineRef,
} from "xServices/pagination/paginationXService"
import { ActorRefFrom, assign, createMachine, spawn, send } from "xstate"
import * as API from "../../api/api"
import { getErrorMessage } from "../../api/errors"
import * as TypesGen from "../../api/typesGenerated"
import {
  displayError,
  displayMsg,
  displaySuccess,
} from "../../components/GlobalSnackbar/utils"
import { queryToFilter } from "../../util/filters"

export const workspacePaginationId = "workspacePagination"

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

type WorkspaceItemEvent =
  | {
      type: "UPDATE_VERSION"
    }
  | {
      type: "UPDATE_DATA"
      data: TypesGen.Workspace
    }

export const workspaceItemMachine = createMachine(
  {
    id: "workspaceItemMachine",
    predictableActionArguments: true,
    tsTypes: {} as import("./workspacesXService.typegen").Typegen0,
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
                // to "Pending" so the UI can display the updated state right away and we
                // don't need to display an extra spinner.
                actions: [
                  "assignPendingStatus",
                  "displayUpdatingVersionMessage",
                ],
              },
              UPDATE_DATA: {
                actions: "assignUpdatedData",
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
                target: "idle",
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
                target: "idle",
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
                  target: "idle",
                  actions: [
                    "assignUpdatedData",
                    "displayUpdatedSuccessMessage",
                  ],
                },
              ],
            },
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

        return API.startWorkspace(
          context.data.id,
          context.updatedTemplate.active_version_id,
        )
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
        const message = getErrorMessage(
          event.data,
          "Error on update workspace version.",
        )
        displayError(message)
      },
      displayUpdatingVersionMessage: () => {
        displayMsg("Updating workspace...")
      },
      assignPendingStatus: assign({
        data: (ctx) => {
          return {
            ...ctx.data,
            latest_build: {
              ...ctx.data.latest_build,
              status: "pending" as TypesGen.WorkspaceStatus,
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
  paginationRef?: PaginationMachineRef
  filter: string
  count?: number
  getWorkspacesError?: Error | unknown
  paginationContext: PaginationContext
}

type WorkspacesEvent =
  | { type: "UPDATE_PAGE"; page: string }
  | { type: "UPDATE_VERSION"; workspaceId: string }
  | { type: "UPDATE_FILTER"; query?: string }

export const workspacesMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QHcD2AnA1rADgQwGM4BlAFz1LADpZz1SBLAOygAU8pmKHUmBiANoAGALqJQOVLAaNe4kAA9EAWgAcANnVUAzAEYATOoCcAdiFHt+obtUAaEAE9Euo6qo2LR0y-3bV+1QBfQPs0LFxCEnJKKhhSRhYAdQxsfCJYPgheamYAN1RMajjk8LS4YTEkEElpWSZ5JQRlAFYjABYdA1UjXVbmtu1teycENqN9KhNm5tVVbXU5nvHm4NCUiPSyCiKweOYoEtTIjKymHKZ8wtjdw43y3UqJKRkeeqrG5X02rT11XSEAiYBl4TMNEG1+lQhEJ1FZtF4hM0TPpdKsQGEjptojs9kl1mUMmB0OgMFQcAAbCgAMwwAFtrqRbgSKvIai85O8VM11EIdNMNEJtBCBkNHODoVQAtChCZ4eYbNo0Ri7rAtjEAK44CDcPGlSIAJTAVJO2SoeQK1E12soTINRtgLKqbLqDRU+n0RioRn6JhMqiRQjaQn96jBCBszSozX0zW0rSMxnUst0ipC6PxxzV1GQeBkABVUIaqeg4AALW3pPgKWjbKh4KmUdAACma0oAlHxlQSs1Qc-nC0aS7Byxn0o6nrVXq6mtzPULevDdEm2kHQWKEB6Jt045ZoUn-uo2krR1FtnwAKqsAAiAEE8wBRAD6ADV7-riABJADyADlx9VnhdTkmksXRJgTBY-GMYNvTDZRek9TRej9XQDFcP0VjTLtM2xC9rzvJ9WBvABxe9-2dKdgOUWUtGjL5Az8QY-TsdcEyjVCTBsIEvB49Rjz1LEz0vW8H0fAAxD8ABkH31cjAMo0APn6Dpvhlf4E3+GNmjgpdtCoMYYVUAZuQ0JMgjRJhUAgOB5GwwSYhreh9nYTgmG4DkJ3ZN5FJULwOiFMw-SsVRrEFMNvS9Hiouikx+MxU8YjiBIDhPeAnXkjzFF8gYdERfwEy8PwUTDQw9KRZoU2hcZNDafRYqw1KeytHUUoEsAizSzygJ8poAjcKxvmMNpZW5VoSosL0iu6UrLGROKVR7PtSALIshxHNrOoAydMqUmYo00ExTCM-pBhK8woRTWFWh5WYgyPBqNqzVkMu8rKmn+WqdGGmV-GDULRRGLQotUXRfVBsx2kRYJgiAA */
  createMachine(
    {
      tsTypes: {} as import("./workspacesXService.typegen").Typegen1,
      schema: {
        context: {} as WorkspacesContext,
        events: {} as WorkspacesEvent,
        services: {} as {
          getWorkspaces: {
            data: TypesGen.WorkspacesResponse
          }
          updateWorkspaceRefs: {
            data: {
              refsToKeep: WorkspaceItemMachineRef[]
              newWorkspaces: TypesGen.Workspace[]
            }
          }
        },
      },
      predictableActionArguments: true,
      id: "workspacesState",
      on: {
        UPDATE_VERSION: {
          actions: "triggerUpdateVersion",
        },
        UPDATE_PAGE: {
          target: "gettingWorkspaces",
          actions: "updateURL",
        },
        UPDATE_FILTER: {
          actions: ["assignFilter", "sendResetPage"],
        },
      },
      initial: "startingPagination",
      states: {
        startingPagination: {
          entry: "assignPaginationRef",
          always: {
            target: "gettingWorkspaces",
          },
        },
        gettingWorkspaces: {
          entry: "clearGetWorkspacesError",
          invoke: {
            src: "getWorkspaces",
            id: "getWorkspaces",
            onDone: [
              {
                target: "waitToRefreshWorkspaces",
                cond: "isEmpty",
                actions: ["assignWorkspaceRefs", "assignCount"],
              },
              {
                target: "updatingWorkspaceRefs",
                actions: "assignCount",
              },
            ],
            onError: [
              {
                target: "waitToRefreshWorkspaces",
                actions: "assignGetWorkspacesError",
              },
            ],
          },
        },
        updatingWorkspaceRefs: {
          invoke: {
            src: "updateWorkspaceRefs",
            id: "updateWorkspaceRefs",
            onDone: [
              {
                target: "waitToRefreshWorkspaces",
                actions: "assignUpdatedWorkspaceRefs",
              },
            ],
          },
        },
        waitToRefreshWorkspaces: {
          after: {
            "5000": {
              target: "gettingWorkspaces",
              actions: [],
              internal: false,
            },
          },
        },
      },
    },
    {
      guards: {
        isEmpty: (context) => !context.workspaceRefs,
      },
      actions: {
        assignWorkspaceRefs: assign({
          workspaceRefs: (_, event) =>
            event.data.workspaces.map((data) => {
              return spawn(workspaceItemMachine.withContext({ data }), data.id)
            }),
        }),
        assignCount: assign({
          count: (_, event) => event.data.count,
        }),
        assignPaginationRef: assign({
          paginationRef: (context) =>
            spawn(
              paginationMachine.withContext(context.paginationContext),
              workspacePaginationId,
            ),
        }),
        assignFilter: assign({
          filter: (context, event) => event.query ?? context.filter,
        }),
        sendResetPage: send(
          { type: "RESET_PAGE" },
          { to: workspacePaginationId },
        ),
        assignGetWorkspacesError: assign({
          getWorkspacesError: (_, event) => event.data,
        }),
        clearGetWorkspacesError: (context) =>
          assign({ ...context, getWorkspacesError: undefined }),
        triggerUpdateVersion: (context, event) => {
          const workspaceRef = context.workspaceRefs?.find(
            (ref) => ref.id === event.workspaceId,
          )

          if (!workspaceRef) {
            throw new Error(`No workspace ref found for ${event.workspaceId}.`)
          }

          workspaceRef.send("UPDATE_VERSION")
        },
        assignUpdatedWorkspaceRefs: assign({
          workspaceRefs: (_, event) => {
            const newWorkspaceRefs = event.data.newWorkspaces.map((workspace) =>
              spawn(
                workspaceItemMachine.withContext({ data: workspace }),
                workspace.id,
              ),
            )
            return event.data.refsToKeep.concat(newWorkspaceRefs)
          },
        }),
      },
      services: {
        getWorkspaces: (context) => {
          if (context.paginationRef) {
            const { offset, limit } = getPaginationData(context.paginationRef)
            return API.getWorkspaces({
              ...queryToFilter(context.filter),
              offset,
              limit,
            })
          } else {
            throw new Error("Cannot get workspaces without pagination data")
          }
        },
        updateWorkspaceRefs: (context, event) => {
          const refsToKeep: WorkspaceItemMachineRef[] = []
          context.workspaceRefs?.forEach((ref) => {
            const matchingWorkspace = event.data.workspaces.find(
              (workspace) => ref.id === workspace.id,
            )
            if (matchingWorkspace) {
              // if a workspace machine reference describes a workspace that has not been deleted,
              // update its data and mark it as a refToKeep
              ref.send({ type: "UPDATE_DATA", data: matchingWorkspace })
              refsToKeep.push(ref)
            } else {
              // if it describes a workspace that has been deleted, stop the machine
              ref.stop && ref.stop()
            }
          })

          const newWorkspaces = event.data.workspaces.filter(
            (workspace) =>
              !context.workspaceRefs?.find((ref) => ref.id === workspace.id),
          )

          return Promise.resolve({
            refsToKeep,
            newWorkspaces,
          })
        },
      },
    },
  )

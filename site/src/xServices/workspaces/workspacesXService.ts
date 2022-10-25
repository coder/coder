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
  getCountError?: Error | unknown
  paginationContext: PaginationContext
}

type WorkspacesEvent =
  | { type: "REFRESH_DATA" }
  | { type: "UPDATE_VERSION"; workspaceId: string }
  | { type: "UPDATE_FILTER"; query?: string }

export const workspacesMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QHcD2AnA1rADgQwGM4BlAFz1LADoDUBXAO1KoEsIAbMAYgFUAFACIBBACoBRAPoAxAJIAZcQCUA2gAYAuolA5UsFqRaoGWkAA9EARgCsANioBmAEw37q+1YDsLgBwAWbx4ANCAAnohOVFZWjvYeFr5WAJxeThYAvmnBaFi4hCTklDT0TFQwpAYMUADCxaRcEEbULAwAbqiY1GUA6hjY+ESwNYykappIIDp6BkYm5gi+Ho5UjraOqhaq3jYbNsFhCNa+VDYJqp72iX423jEZWb25A2QU1LTDpWDlzdW1XGDo6AwVBw7AoADMMABbD6kHo5fpwIZMUYmSb6QzGcZzGyORJUDYJOIeKzeWKJPaIFZWKjeRKqYluOJWXw2VQ2O4gbJ9PKwZ6FLmPOBUWDkdAVKB8PBQZoUDFcFHjNHTTGgOYAWm8Fjx9i1jl8Tn83gC5NCiDVvgsy1UiUcjg8HiNuKNxI5AoRvIK1DdPJh4rh3IG9UarFa7U6n39gtgCu0unRMyxZts1McG281tShosFIORqoxNJLkcpL8Fscroe7r5XsrPrKftrgYaDCaoY6MMj7uUFjGsamGNmZq2lsSvlU-isGZsiSs9hzxY8VDOtrpNg8l2tvgr8J51ao3oGvu+nZ5fwBQJB4KhHcbcBjEzjysHCDVzKW10SFnsmuLiSc3hzPMYk2VNfH8BYom3AN8hefdb1gKg6BwCBZUqE8iEUMAwVgIMWxDNp2yQlDKHQsBMOw+8lQHRMX1cOx1hZDZU1sDZ7DnU0EDYpYwM3CwthiB1vCsKCoz3A8hWQPB9BEVByPQOAAAtSJw0wRVgvAwUodAAApJ1UVQAEouHEj1YJM-cpNIGS5MU5TKMfajVSTO0l3WGJ3CSUlEhNfYLmONlNVpVMHRsZkRKrT04J3QNFDEKRYuIAAJCRhBEIR7P7BMnJfadvAcWl9Q2cdxzXHNzUtNYbTtQSnQCYTMk5eDq14QRREkAA1MRFGIGQAHkADkMvjFUzHCalokSE54k8L8LC8MrnCWG4-EWNj4gtDwMgahhUAgOATBMvc3hKNhOFRBystG+ZR0iekyQ8a1WKsHMHqXX87TXGwcXscLd0i47mHrb4kVIc7MpG7FqU-G1SVY+xWWeji+KWJJthtM4GQSLcGsO-7ajB4bnzmqhoeLHU3Hhs4ytsKgwK8MmDRtX6nki8y1LFb5JWlBhZUuqjLvVI07GsS4Vj8TxfE-Mr4lUZY-0m+wwOcM5fHLHGmtZ+CjzQ+CCafGiNRnZYhPcNj9TWPj5zsVGtW8i1xfcdl1eimD+S1ojUKgUjyPgRULohs04ktKw01V5lSScHNFbxac31yvivGsZnXZrF2EMk6TZKw+TYCU3W-fB581XcS1FnXe0blcDwxyCDi2OWcvcU-L6ziSZPTLdtO9ccq61VHexjdnWdFacdYAI4tUVzli4i0VrY2R+53oI7sBu4Fs1vIqk3h-Nseyp1PLcRn1w7QsaxvC2tIgA */
  createMachine(
    {
      tsTypes: {} as import("./workspacesXService.typegen").Typegen1,
      schema: {
        context: {} as WorkspacesContext,
        events: {} as WorkspacesEvent,
        services: {} as {
          getWorkspaces: {
            data: TypesGen.Workspace[]
          }
          getWorkspacesCount: {
            data: { count: number }
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
      },
      type: "parallel",
      states: {
        count: {
          initial: "gettingCount",
          on: {
            UPDATE_FILTER: {
              target: ".gettingCount",
              actions: ["assignFilter", "sendResetPage"],
            },
          },
          states: {
            idle: {},
            gettingCount: {
              entry: "clearGetCountError",
              invoke: {
                src: "getWorkspacesCount",
                id: "getWorkspacesCount",
                onDone: [
                  {
                    target: "idle",
                    actions: "assignCount",
                  },
                ],
                onError: [
                  {
                    target: "idle",
                    actions: "assignGetCountError",
                  },
                ],
              },
            },
          },
        },
        workspaces: {
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
                    actions: "assignWorkspaceRefs",
                  },
                  {
                    target: "updatingWorkspaceRefs",
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
                  target: "#workspacesState.workspaces.gettingWorkspaces",
                  actions: [],
                  internal: false,
                },
              },
            },
          },
          on: {
            REFRESH_DATA: {
              target: ".gettingWorkspaces",
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
            event.data.map((data) => {
              return spawn(workspaceItemMachine.withContext({ data }), data.id)
            }),
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
        assignCount: assign({
          count: (_, event) => event.data.count,
        }),
        assignGetCountError: assign({
          getCountError: (_, event) => event.data,
        }),
        clearGetCountError: assign({
          getCountError: (_) => undefined,
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
            const matchingWorkspace = event.data.find(
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

          const newWorkspaces = event.data.filter(
            (workspace) =>
              !context.workspaceRefs?.find((ref) => ref.id === workspace.id),
          )

          return Promise.resolve({
            refsToKeep,
            newWorkspaces,
          })
        },
        getWorkspacesCount: (context) =>
          API.getWorkspacesCount({ q: context.filter }),
      },
    },
  )

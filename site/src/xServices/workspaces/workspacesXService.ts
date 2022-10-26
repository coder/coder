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
  | { type: "UPDATE_PAGE"; page: string }
  | { type: "UPDATE_VERSION"; workspaceId: string }
  | { type: "UPDATE_FILTER"; query?: string }

export const workspacesMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QHcD2AnA1rADgQwGM4BlAFz1LADoDUBXAO1KplNIEsGoBhepgYgioG1TgDdUmaqwDqGbPiKxejUgG0ADAF1EoHKljsOw3SAAeiAIyWNVAMwA2AJwAOACyWnAJjsuvGty8AGhAAT0QfWxcAVm9ohwcXAHYkp0sXFwBfTJC0LFxCEnJKGj5mVg4uFQEwdHQMKhwAGwoAMwwAWxYwUjl8xThq9W1TfUNjBlMLBDsnaKovaMtfDQ00jLckhxDwhEiqGO9-OxPZk69s3PkCpTIKalpVfgBVAAUAEQBBABUAUQB9ABiAEkADJ-ABKmh0SBAYyM7BMsOmbm8VA00Q0KRcli2di86TcOwiDioDk8DjcMS8Lm8dg0LgclxAeQUhVgdxKrJucCosHI6EqUFeeCgnAoiIY-GhowMCKRoGmAFocU57J4vG58VSMqliQglR4FmsvF5sX5XMloszuQMOcVqLb2d02JwoH02UpBMJRAwJFIXR6ebAZbD4RMpoglfF5gSGWt8ZYdZZ9ekXFQktEXI58dn3B4LjkWdc7ZzHSXnRU3UG7d6RFRxJJpD0a+y1JYYXo5RHkVHGZYqE43AEYgnnNE7PqaUl0dFTU4NA5Ui41m4bRXbg6qE6lC6ha2vbV6uhGi1SO10F1ZBu4KGu+NJZGDdFAmTacscTSnLn9RkFvS-CTKlNmia0ix3Ip7m3G9YCoOgcAgCUuAPMAITAVpYDrX1-WoeDEMoFC0Iwu84W7R9ewNelSQ0JNyQ0Al4hsE59XOKg3GHId0gcHwkgyMCrn6dky2gwTd2QPAjG+VAiPQOAAAsUMwsx+SgvBWkodAAApMVWABKfgIPtKDDO3CTSCkmT5MUkjw3IxUoznGdVksHw7FA1xZicFi1QcRccVpAleIcF911EyCuRgl4Ph+AFXk+ABxX4bLIhVzAc78DjsawHDc7wdW2MIo2WNVeNNTMaTNLLF1Cz1wrAKKvj+f4ADVfghYhgQAeQAOWSh9UumWYM2cWjVixdxJ0Kg1NXmXzEhcjF3Bo7IiwYVAIDgUxDOEx4mAbCAmjAWV+smCj-HTLK50xZxFg0WZgimxwSrcSkkheuY3GiJJCwE2qjJKXbyh6IUhmO+VTvshBuIORJKQJHx4esKcfOOfwEi-OxUhq4MdrKMGe0hpMYcZQIXMTAkXP1JVoZWNyGSTQkMX44swv+8tWb5AUhRFMUGAlVLbIGvtGSoSxYj8LNPretIqaTWwvG-JwcvYrxfJfH6Wb+4STKrZCYPxuy0oNWlYxiNyTi1fx0inUlYnJJwHY8dwJ3ibHSy3Ey8KQ90byI+AwxSiGjaVJJrCoJYGUCF9s3xFjUTJD7VdcdItjFt2hI9mDTMk6T0Nk2AFP1gOTqfJU3IHb7UmxSI3qxFi7AWKuFbSBImacdPN2Mov73B0uhwbmkXYt-EaJcKn5wWRXuN4r6XNAju6oNoWDQdgdB-NuxLdHqmsvTBXZm4lyMgpJkVqAA */
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
          on: {
            UPDATE_FILTER: {
              target: ".gettingCount",
              actions: ["assignFilter", "sendResetPage"],
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
            UPDATE_PAGE: {
              target: ".gettingWorkspaces",
              actions: "updateURL",
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

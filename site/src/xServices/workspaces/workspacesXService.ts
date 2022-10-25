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
  | { type: "REFRESH_DATA"; limit: number; offset: number }
  | { type: "UPDATE_VERSION"; workspaceId: string }
  | { type: "UPDATE_FILTER"; query?: string }

export const workspacesMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QHcD2AnA1rADgQwGM4BlAFz1LACYA6A1AVwDtSaZTSBLJqAYUZYBiCKiZga3AG6pM49gHUM2fEVj9mpANoAGALqJQOVLE5dRBkAA9EAFgDstABw2AbI5cBmAKx27L7QCMXjYANCAAnogeAbTaXgCcdl4BflRUSR52AL5ZYWhYuIQk5JS09BpsYBzcfAKkgmDo6Bg0OAA2FABmGAC2laSKBSpw6iw6+kggRiZmTBbWCPY2NDGO2olB7gHajmGRi+srdom+8QGOFzaOOXlKhapkFNQ0+cpFsDSw5OhcPAAKeCg3AonFEgnGFmmplBc0mCw8LjsNAuLiCNkyNm0Vw8e0Qdm0sRcNgSVDc8WSNgCNxAr3uxSetFpww+7F+UEGb1UwlE4ikMjkVQ5dNgEMmUNm80QVHi2hoRO0STS2Ku8VxCG22hcNHi8UcyR8rkcaWyuRpd2Zj1KL3N736bKFzO5YgkTGksn6DvemgCE0Mxmh5jhUucyPcjmiVC8-g8NljaoCMWWjhS53lCS8cWpTPelue2dUdpqnq5jWa6FaHVI3XQfQUNtUor9MxhkoQXmS2tJLi8jjOMoCcYiiHiVGWHgja1J8U8VGupvz9KtC4+DBwEBBPGLYAASmBOrAnbzXfyaKv15Qt7v942pv6JUGEPEY3LoplNR4Zd4XGryY4aHZUyodZNUSI0s3rRc8wgj5kDwUwABVUCvdA4AACy3A9LC+J4aDwTpKHQAAKDNtG0ABKQRl1zRloJeODSEQ5C0Iwm9xRbB8yTlaUEwSWMCRcb8hwQI0AhoAlyQAgIYz1cChhzEooLkrltwAUQAMVU4gAAkAH0ABEAEF4IM1i73Y0AFmJDwaCoAJP38Xt-HSNV8UJYlkgA7QPGTLxZM5SCqEEABVP5DPglSdIANRU7diAASQAeQAOVM5tAws4cUjE-wfDcK4gMyNUCpWC4UkciNdSpedoOo4LQqMiK1LigAZcLt1SgNYQy9UEyRM5zm8uJIxiQT9js+IaG8U5jkRbZpTnU0mFQCA4AsKiFLKOoJAgNowEhMz0qsPEQz1OJMSk9MrjVGMkXxfEe1jAJuzyvzhWougttZGpRlIfa0q6o6EGOOVAl1Gx0lRSM9Wu2z-xOcHk3OOwLg8V6LQ2j6ND+zrW2iEG7OcCGYgeuwiqoay4kSTFI11HUBLR+SGWtJS4E+b42QBIEmBBQ62MOhZkzldEYyJDze1JoTuy8GzjnB2MRxRGwGYeDHl0LTdoOx+9uo8ICbMh3V8Skuxxxc8MaC8aUrhsGVpW8qhlYC5n-JXNcN3ZCCr3gMUDoBhZPGWWdtm8LwPHWcdQiEhN0RWIC0mCKMBx7R3YHetXYIQpC9xQ2B0M1n3-tbdxrPHM4paCbjRsQESxKt3sUm7Zw51uFnU9V-Omxxh9xyFiPRbKxI1SjLV0ifEcSPiWMleq1vqK18zAYAWieybdUCHxyccOxKVD+Nn0p7eR3sYC0hyHIgA */
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
            idle: {
              on: {
                UPDATE_FILTER: {
                  target: "gettingCount",
                  actions: ["assignFilter", "sendResetPage"],
                }
              }
            },
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
              always: "gettingWorkspaces" // TODO
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
          paginationRef: (context) => spawn(
            paginationMachine.withContext(context.paginationContext),
            workspacePaginationId
          ),
        }),
        assignFilter: assign({
          filter: (context, event) => event.query ?? context.filter,
        }),
        sendResetPage: () => send("RESET_PAGE", { to: workspacePaginationId }),
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

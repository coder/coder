import { ActorRefFrom, assign, createMachine, spawn } from "xstate"
import * as API from "../../api/api"
import { getErrorMessage } from "../../api/errors"
import * as TypesGen from "../../api/typesGenerated"
import {
  displayError,
  displayMsg,
  displaySuccess,
} from "../../components/GlobalSnackbar/utils"
import { queryToFilter } from "../../util/filters"

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
  filter: string
  getWorkspacesError?: Error | unknown
  getCountError?: Error | unknown
  page: number
  count?: number
  limit: number
}

type WorkspacesEvent =
  | { type: "UPDATE_FILTER"; query?: string }
  | { type: "UPDATE_VERSION"; workspaceId: string }
  | { type: "NEXT" }
  | { type: "PREVIOUS" }
  | { type: "GO_TO_PAGE"; page: number }

export const workspacesMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QHcD2AnA1rADgQwGM4BlAFz1LADpk8BLUgFVQCUwAzdOACwHUNs+IrADEAD1jlKVPO0roAFAFYADGoCUItFlyESU6rQbM2nHvx1C4AbRUBdRKBypYDOqgB2jkGMQBGAGYANgCqFQAmAE4-cIAOOPDwoIB2ZIAaEABPRHCVKljAv2S-EMiClSUAFliAXxqM7UE9WDIKanYwUgJuOg8oKgJUAFcPUioYUlJeqABhYdGRCE9qXoA3VExqCYsm4TmR0lsHJBBnVynPb18ESsq8uOLk2KCVSqVY4qUM7IQAyqoApElOFKskgpEUn5avUQI1dMJWtIOl0en0BvMxhMpn19gswOh0BgqDgADYUdgYAC2406O3hcFxh3s3jObkuJ2ufyUVEiAXi4QCySUAWBARF3xyaiowuqZShQSCiUidQaAnpLQMVGR3WmNDVVlgVCGOAgFGmdKsplESw8Kw8602RpNbQteitRxZLjZXg5iFitwByTKARUQVulQCiQlCBeeX9sWifihhXBKth+uaiPanR1aLhBppk3NGeEi2WVDWGy2tJLNmZJ1ZFx9oGub25f0jyQqSiUQvi0aUiqoiXCfnecT8kRUwTT+czmu1qP6c+EhexUFdpZtdod1dIm5sfmOTi9TauiFukWlCb5097-tD0cqQ57dw+8cCz9ntY1bS1OaXPVLGaNdi2A0t8UJdBiTJUgKXQalth-D0G1Pdxmx8fximHF5O1uVJKgFAJoxCZIqChPlIhBUMngib9wP0P9F2mMtbSoSQ-xXRikQA6YUJPc50PPBBAhSYcQWBJJcmBfsshyIpxNHLsCiUIFIyCejdm4sARAAcQAUUYAB9XgAHkWAAaWIAAFABBGZ9OIfjTjQ9kW0QABaQiwmKadkhDQjgmSSpow8gIx3Ivkg17KIwSUPxNPVLMRAAVWsgARWzGH0oyADEAEkABlspYZzGyE30ECePwAVicLkiiJNKjHcJQvC-5Xg+Io+XHYFEoNZK0sy7KjIANX0lhiHy0yADkytcjDrj8NQyNHBUilUYplsiNrvJFJJVEHZqew0mEuN-SgRBm-SAA1GHmwS3MwkSgm5XsFQO4E-FBQI2u+sJqgaiFAeKV7+vnNoRGslh9NG6aUqc+sBO9YTBX+cFR1HQdIgjI6-o6wGojDD5QaUcGEQMPTTKMxhqbsgyHpRyr3iCKhgtE0E7gFZ58YBj4iZBkoybTDxUAgOBvHOrMaHoJhWA4LhYD4H9PUexb-DFdHe26p44hFPxo1EnkgihfzBV5QI+rOn9peYtFBgOUCcQxVWmfc35-nCVTYh9iFwpBEFo0BAEp3iCJRzUEVKnJ7T-xRXUHdGKht1ds9KpCVnXj+V5ihBD7ozWtnEnlPluZ7AIY4u7N4-tl3ULV4SPMCNm7iTZbgg+d7QqqKgXlBYIIxUSIcd5Svbd4vMfydU11wPK1U4q926vCHkdeFKiXjHJ9qjZwcffUwEVGW4XVQYqu49zZcp6xMCtPgeu3eev5pVSSdEjFRUShKbfuSFIJ339hCL2p1T533HjXK+Z9k7LAXk9a4xRPZ3F8tzMUEJoz+TeikXkipQx7wrtbM+4DL5ATvrA9WCAm6hEInEFQqQFSCjqv6IOg5d7-zeIEP4pQx4LgnlAMhjcTYtyPkmac-8hRglCt9UIfcGrdg+LQj43C2j8Mqk3IeQi26iM7hIuSFC7j-ECD7LOdVBRDzqHUIAA */
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
        UPDATE_FILTER: {
          target: ".fetching",
          actions: ["assignFilter", "resetPage"],
        },
        UPDATE_VERSION: {
          actions: "triggerUpdateVersion",
        },
        NEXT: {
          target: ".fetching",
          actions: ["assignNextPage", "onPageChange"],
        },
        PREVIOUS: {
          target: ".fetching",
          actions: ["assignPreviousPage", "onPageChange"],
        },
        GO_TO_PAGE: {
          target: ".fetching",
          actions: ["assignPage", "onPageChange"],
        },
      },
      initial: "fetching",
      states: {
        waitToRefreshWorkspaces: {
          after: {
            "5000": {
              target: "#workspacesState.fetching",
              actions: [],
              internal: false,
            },
          },
        },
        fetching: {
          type: "parallel",
          states: {
            count: {
              initial: "gettingCount",
              states: {
                gettingCount: {
                  entry: "clearGetCountError",
                  invoke: {
                    src: "getWorkspacesCount",
                    id: "getWorkspacesCount",
                    onDone: [
                      {
                        target: "done",
                        actions: "assignCount",
                      },
                    ],
                    onError: [
                      {
                        target: "done",
                        actions: "assignGetCountError",
                      },
                    ],
                  },
                },
                done: {
                  type: "final",
                },
              },
            },
            workspaces: {
              initial: "gettingWorkspaces",
              states: {
                updatingWorkspaceRefs: {
                  invoke: {
                    src: "updateWorkspaceRefs",
                    id: "updateWorkspaceRefs",
                    onDone: [
                      {
                        target: "done",
                        actions: "assignUpdatedWorkspaceRefs",
                      },
                    ],
                  },
                },
                gettingWorkspaces: {
                  entry: "clearGetWorkspacesError",
                  invoke: {
                    src: "getWorkspaces",
                    id: "getWorkspaces",
                    onDone: [
                      {
                        target: "done",
                        cond: "isEmpty",
                        actions: "assignWorkspaceRefs",
                      },
                      {
                        target: "updatingWorkspaceRefs",
                      },
                    ],
                    onError: [
                      {
                        target: "done",
                        actions: "assignGetWorkspacesError",
                      },
                    ],
                  },
                },
                done: {
                  type: "final",
                },
              },
            },
          },
          onDone: {
            target: "waitToRefreshWorkspaces",
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
        assignFilter: assign({
          filter: (context, event) => event.query ?? context.filter,
        }),
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
        assignNextPage: assign({
          page: (context) => context.page + 1,
        }),
        assignPreviousPage: assign({
          page: (context) => context.page - 1,
        }),
        assignPage: assign({
          page: (_, event) => event.page,
        }),
        resetPage: assign({
          page: (_) => 1,
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
        getWorkspaces: (context) =>
          API.getWorkspaces({
            ...queryToFilter(context.filter),
            offset: (context.page - 1) * context.limit,
            limit: context.limit,
          }),
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

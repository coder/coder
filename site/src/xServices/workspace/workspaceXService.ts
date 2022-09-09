import { assign, createMachine, send } from "xstate"
import { pure } from "xstate/lib/actions"
import * as API from "../../api/api"
import * as Types from "../../api/types"
import * as TypesGen from "../../api/typesGenerated"
import { displayError, displaySuccess } from "../../components/GlobalSnackbar/utils"

const latestBuild = (builds: TypesGen.WorkspaceBuild[]) => {
  // Cloning builds to not change the origin object with the sort()
  return [...builds].sort((a, b) => {
    return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()
  })[0]
}

const Language = {
  refreshTemplateError: "Error updating workspace: latest template could not be fetched.",
  buildError: "Workspace action failed.",
}

type Permissions = Record<keyof ReturnType<typeof permissionsToCheck>, boolean>

export interface WorkspaceContext {
  workspace?: TypesGen.Workspace
  template?: TypesGen.Template
  build?: TypesGen.WorkspaceBuild
  resources?: TypesGen.WorkspaceResource[]
  getWorkspaceError?: Error | unknown
  // error creating a new WorkspaceBuild
  buildError?: Error | unknown
  // these are separate from getX errors because they don't make the page unusable
  refreshWorkspaceError: Error | unknown
  refreshTemplateError: Error | unknown
  getResourcesError: Error | unknown
  // Builds
  builds?: TypesGen.WorkspaceBuild[]
  getBuildsError?: Error | unknown
  loadMoreBuildsError?: Error | unknown
  cancellationMessage?: Types.Message
  cancellationError?: Error | unknown
  // permissions
  permissions?: Permissions
  checkPermissionsError?: Error | unknown
  userId?: string
}

export type WorkspaceEvent =
  | { type: "GET_WORKSPACE"; workspaceName: string; username: string }
  | { type: "START" }
  | { type: "STOP" }
  | { type: "ASK_DELETE" }
  | { type: "DELETE" }
  | { type: "CANCEL_DELETE" }
  | { type: "UPDATE" }
  | { type: "CANCEL" }
  | { type: "LOAD_MORE_BUILDS" }
  | { type: "REFRESH_TIMELINE" }

export const checks = {
  readWorkspace: "readWorkspace",
  updateWorkspace: "updateWorkspace",
} as const

const permissionsToCheck = (workspace: TypesGen.Workspace) => ({
  [checks.readWorkspace]: {
    object: {
      resource_type: "workspace",
      resource_id: workspace.id,
      owner_id: workspace.owner_id,
    },
    action: "read",
  },
  [checks.updateWorkspace]: {
    object: {
      resource_type: "workspace",
      resource_id: workspace.id,
      owner_id: workspace.owner_id,
    },
    action: "update",
  },
})

export const workspaceMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QHcD2AnA1rADgQwGMwBlAFz1LADoZTSBLAOygHUNt8iBiCVR6pgDdUmarTZZchMIlA5Usegz6yQAD0QAWAAwAmKgHYAbAFYjADgDMB80YCcu87oA0IAJ6JLARhP6Dly01dO1tzAztTAF9I1zRJThJyShowOiZWdiluMHR0DCocABsKADMMAFsU0gkOaVV5RWVGVQ0EOwMvQxNw7VsHbU1Nc1cPBEtgy0MAoIcTE20vI0to2MyEsgpqdDAS7dgAC3SAFTByos2ePgFGYVEqbd24fZOz4sp6hSV6FSR1LT1DKYLNY+o4XO5EI4DH5prpfHYAgYViA4rUiBtkg89odmC9zpQuDk8ugCm8yuhKlinni3jJfg0vj9QK0dH4gVYbPYwSNPNprFNAroDNptAYOl4kTEUWtpBixKkGMwAAo5cr0WCKPiwS78KhCETUAj7MAETAqinqzWMeD0z5NFqILx6fQWAwmOzzfzmII8hCLTSTIxB8USyxGLzLKWorKJTZVRVQc1qjXfa2E3L5fHkypGk1m1WW1M2uR21MOv3Oqiu92eyze8GjRx2OxUXSw0yaIxw8zI6PrJJbMB4CBuAqoQqFdI1GP3HbYqcy7i8XX6u5Ug7ThIfRpl36tUOTbQI8wDMxeHxGX0dcxUEUirwRJ29Nu9xexzFDkdjicL+LSWePAcv5omA6bEqSpQVAB2KbnUto7kyfx+v4h7Hqe4YXr6Sz6Heei9NYRhBJor5-uiA6zsOo7yD+zCwUQVDIHgXzMFwaiwOReAlJQ6AABRPtoACUXB9rK5HbJR36TrRb4MUxCbboyzR7o65gmJorZ8hKHQdJY2hzL6Jjered4RuYXi2JoEYkSBcoUV+ABGACu9CFBAeoQIUoHEEcACCABKRwKfaykILoR7aLeXZzIE4yqZoBi+hGenGSKgzaEGdh6SY1kxrZ4kOc5rnuZ5XDeQA8kqQW7sykLhZFcImDFjhqQlEKhboXj6AEgTpeeekSsRUZvnln6jk5LlufQHmgT5xAANIAPoACIAKIADIrUcK1VYhrRhZl9XRQGzXxYlAwGClAznuK3p2Dl-ZxvlY2FZN01cAAqkqS0+VtO1KTVoV1elDVNXFrWjANRiXUGFhmeG92iY9o1UONRVTSVADCPkAHIY+tf3lvtEXA0dsUtYlhmHneZgniebqDaspHvoOEmo25eCwJg6RLWAnkEqtG2-fBimE7ouhQ2YcJ2BK5hme0l5tR6nS4UKEbeE4d1DUzI2sy9VAc1zzA83zoFY7j63Letm3bcLwUAx1RgRR1R4RHYllOhKvrNi2uGBD4HXegjZFI7rE2zgAjo5cAJhs6CkDq1y3NQ7F4HHdF0iWCH-UhQomJ04bQnpEwB4lqk3t1Oi6Z2miekHzN2c9YfbJH0fpLH8dEpmZJQSnadvgTIW5+pEqu7Y8wim7iVePFVDjwMst8hERh1zrBVN2ALfsW3pCoDgCd6jcBpUOxu-pwP9udfoedmEYxidYRnttftF0V3yuiaG74zL1rNlicjbMRyjlvZgZBd5gS7pBCkx8d44DPrbaqOdOomFvDYRYR57DHXBrVc8VBX6DGMM2SMjNf4hzXkVZuQCEzG1SKBZcicj4QF5jQuBmcRYhQlGLKsZMlhBCDJ2CmhlcHdQRDYBYNciHSm1n-UO5CN6UO5kwgkncSRZigowk2LCQAMjtkhDhLpuGCj4QrCG8UurCPHmKPSEiRLBw-DItyFDW7MAxngRgRBCj71XIaVx7jNHaIQXtOqVhHaWBvtfMwiUAw+zvB-MugQlgr2kWQhxcinFQBcW43m4CVHdygQQHxvM-Gll2pCJ0N53R1mnldEIdhEqhKpiKPOvR3TpWyj-XKSTG6yMAjiKANILh0IPknaC1JTj4gzlo4p2c9oGBnt4OEEpF6NXaIlYUyDX5hm0sYRJpCukpJ6ccMZtJskQVINmEZBx+nvHgSUwGB0SaNWOmDRKSxlZ3iWE4YUxhJTEI6bsyS6Q-JwFQI5dARBYDxkBcC0F4LPGHzuLQIFsAQVgrgOfXRFhOjvzbD4XopjjC+jrMTEyfJvTmUajsuxX5qJSSgEilF4LIXMHpTCuAJzVFQMRdC1FxZJlZ3LIsMyrYgjeHmN6aEBKn5u0uueDCsy1IM0kSQqlVFxy0pZTy2SzEoCsRTskTi3EeJzyEjY+uT0AXMu5Yyxi2r0X7jFFi8y5gIgBFUosSwvp35Q1wqeBwsyAxtN+Q9FVVAGDlF5kweUaRmAACEXrakGV4hiw1OmhvoOGqSUaExxomrABA+p8lNAANraAALp2sdLFW8x4P7iLmA2SElgERUCsH7PQbsF4-KVX8kNYaI26loNm+N7LcmVFNavUcfbM1MqgDm1yeaC0UFTCW8tNzpmVscNWustbGr1sJRGKGFcxaVNsLoSlLMvxTsjVQQoqBhyQDnRACF6NQJrTKj5JaC0ACyZU-IrQWjG96ABJNaS1iAVuQr0Ks15vmEVmV4T1jsWwV2njFWV38g2I17em-t1Bb33ogI+59b0-0ADE-3EAABILSOEBr960gPYxtqwnRgTGpUAlOZJtstOqy09f6ltsJRQRmFGe9pwaL2Tpw9O-DjDCPxpvXeiA6Qv0YDAERuFwzx2pqvbqWTD6FOyZU2poj+bD6FuXWWiDiwPQcfCB0AYEQ5iGU9SeZDsIwxHiCGJzDtjJNpozde-T8nc2KeHMZ7YGnlGnPOdp-5um8NKYM6FozzBVORfjWZ4QFm+Arus-YZBI8HMf1MHMYYT9oSTCPUMMMIQfPdok1QZRXAADim0FosF-XNYgSofJ4wg+0ToboeiggGEMQl79kEoW822R2PhA0Naw6BNrRwOtdZ6315jfK2EAx9G1AITpWywjzt4fw3RohSkYKgRhvK4vJBfRBvbowAgWCO01XF-gu13azcBGMEHwxO00mEMMnUwwmAmz4WewZb5+sIhS8TS2Lm9KuRM-xtynu8nfgKAMnGWkInPTOpMhYtT-c4dWQicsTzg6flYCK039oM4w4tvzDcLUZCZkj37W410CvVlu26dbfBYQ-rPO8wQP7dk7AT81NKuf-htfJHn7CQgXRsJlR8YpVLGMQBTy678RFWBfAjlnMu1Vy6INZ8X-Od2hKF21POOETJ1kakKaE0v-56we0rgGD485Q7hKyewftqcQ26BdOecwPnYvd-Y-WnMFEmwg22OZSDwwIgfPUs6ukugigIuGCwNcY-JMAWk9uSeVdHaiXTd+7rS4iiET1MWrT2g9mN2aj369N4xxgUnwIL9U8Z4z41T1SVBM9VmdLXw78i97JL8AqA1DrksYCZW8y1u3a7rtyYptY+m3BAn44axKb-kAMcfPjJ7je8p7hGn7wCJh9tWnmYUXIoljzF8KpL7x+Q2n7nE8Q5rwmwvejsuCmUt8syz85kLyboDeOg7qzqzYM+KML0lu0qR426G+tuDaCAYQ0S94Yo8q08revm7eEksulqyKrKEKg6UKlBPK1mfO6BAum+2Boql0ooD86utgSB5BdKVqcAWqiuy+tyD4UGRW0sCIuchE+6xK7y6sryR+Ui-yvBGq4KqB6kTBNue6j+syl0sUoowQ4sSBCWxUqOUyAqH+uCjgD4nGQIQo+6IuralkwosssyxBzOpBl60m16NBsa8aSe+uXCru0swoiwWCoUt8w8wYkecOiq32rOJhwWRGph1mCI6kt8oh5khEYUYo-GO+FcdYAQ50hExh3hemSWIW86YWymaWJm-h3uuiYYqEGBgu2B4scIY+OgHa0InYTO8R5qiRFRRGDBO+mhmB2hjYjUL80wQQcwxgUQbeE6AWuGDBAw6+rRnq4wnQThE8YYHUcIM+luwo6xLBZ0jur+ToAcOgqkBOyilugiYxGx+2hhsBHUPgAYeg561m5WEMN4uEecQYp24scwF2kQQAA */
  createMachine(
    {
      id: "workspaceState",
      predictableActionArguments: true,
      tsTypes: {} as import("./workspaceXService.typegen").Typegen0,
      schema: {
        context: {} as WorkspaceContext,
        events: {} as WorkspaceEvent,
        services: {} as {
          getWorkspace: {
            data: TypesGen.Workspace
          }
          getTemplate: {
            data: TypesGen.Template
          }
          startWorkspace: {
            data: TypesGen.WorkspaceBuild
          }
          stopWorkspace: {
            data: TypesGen.WorkspaceBuild
          }
          deleteWorkspace: {
            data: TypesGen.WorkspaceBuild
          }
          cancelWorkspace: {
            data: Types.Message
          }
          refreshWorkspace: {
            data: TypesGen.Workspace | undefined
          }
          getResources: {
            data: TypesGen.WorkspaceResource[]
          }
          getBuilds: {
            data: TypesGen.WorkspaceBuild[]
          }
          loadMoreBuilds: {
            data: TypesGen.WorkspaceBuild[]
          }
          checkPermissions: {
            data: TypesGen.UserAuthorizationResponse
          }
        },
      },
      initial: "idle",
      on: {
        GET_WORKSPACE: {
          target: ".gettingWorkspace",
          internal: false,
        },
      },
      states: {
        idle: {
          tags: "loading",
        },
        gettingWorkspace: {
          entry: ["clearGetWorkspaceError", "clearContext"],
          invoke: {
            src: "getWorkspace",
            id: "getWorkspace",
            onDone: [
              {
                actions: "assignWorkspace",
                target: "refreshingTemplate",
              },
            ],
            onError: [
              {
                actions: "assignGetWorkspaceError",
                target: "error",
              },
            ],
          },
          tags: "loading",
        },
        refreshingTemplate: {
          entry: "clearRefreshTemplateError",
          invoke: {
            src: "getTemplate",
            id: "refreshTemplate",
            onDone: [
              {
                actions: "assignTemplate",
                target: "gettingPermissions",
              },
            ],
            onError: [
              {
                actions: ["assignRefreshTemplateError", "displayRefreshTemplateError"],
                target: "error",
              },
            ],
          },
          tags: "loading",
        },
        gettingPermissions: {
          entry: "clearGetPermissionsError",
          invoke: {
            src: "checkPermissions",
            id: "checkPermissions",
            onDone: [
              {
                actions: "assignPermissions",
                target: "ready",
              },
            ],
            onError: [
              {
                actions: "assignGetPermissionsError",
                target: "error",
              },
            ],
          },
        },
        ready: {
          type: "parallel",
          states: {
            pollingWorkspace: {
              initial: "refreshingWorkspace",
              states: {
                refreshingWorkspace: {
                  entry: "clearRefreshWorkspaceError",
                  invoke: {
                    src: "refreshWorkspace",
                    id: "refreshWorkspace",
                    onDone: [
                      {
                        actions: ["refreshTimeline", "assignWorkspace"],
                        target: "waiting",
                      },
                    ],
                    onError: [
                      {
                        actions: "assignRefreshWorkspaceError",
                        target: "waiting",
                      },
                    ],
                  },
                },
                waiting: {
                  after: {
                    "1000": {
                      target: "refreshingWorkspace",
                    },
                  },
                },
              },
            },
            build: {
              initial: "idle",
              states: {
                idle: {
                  on: {
                    START: {
                      target: "requestingStart",
                    },
                    STOP: {
                      target: "requestingStop",
                    },
                    ASK_DELETE: {
                      target: "askingDelete",
                    },
                    UPDATE: {
                      target: "refreshingTemplate",
                    },
                    CANCEL: {
                      target: "requestingCancel",
                    },
                  },
                },
                askingDelete: {
                  on: {
                    DELETE: {
                      target: "requestingDelete",
                    },
                    CANCEL_DELETE: {
                      target: "idle",
                    },
                  },
                },
                requestingStart: {
                  entry: "clearBuildError",
                  invoke: {
                    src: "startWorkspace",
                    id: "startWorkspace",
                    onDone: [
                      {
                        actions: ["assignBuild", "refreshTimeline"],
                        target: "idle",
                      },
                    ],
                    onError: [
                      {
                        actions: "assignBuildError",
                        target: "idle",
                      },
                    ],
                  },
                },
                requestingStop: {
                  entry: "clearBuildError",
                  invoke: {
                    src: "stopWorkspace",
                    id: "stopWorkspace",
                    onDone: [
                      {
                        actions: ["assignBuild", "refreshTimeline"],
                        target: "idle",
                      },
                    ],
                    onError: [
                      {
                        actions: "assignBuildError",
                        target: "idle",
                      },
                    ],
                  },
                },
                requestingDelete: {
                  entry: "clearBuildError",
                  invoke: {
                    src: "deleteWorkspace",
                    id: "deleteWorkspace",
                    onDone: [
                      {
                        actions: ["assignBuild", "refreshTimeline"],
                        target: "idle",
                      },
                    ],
                    onError: [
                      {
                        actions: "assignBuildError",
                        target: "idle",
                      },
                    ],
                  },
                },
                requestingCancel: {
                  entry: ["clearCancellationMessage", "clearCancellationError"],
                  invoke: {
                    src: "cancelWorkspace",
                    id: "cancelWorkspace",
                    onDone: [
                      {
                        actions: [
                          "assignCancellationMessage",
                          "displayCancellationMessage",
                          "refreshTimeline",
                        ],
                        target: "idle",
                      },
                    ],
                    onError: [
                      {
                        actions: "assignCancellationError",
                        target: "idle",
                      },
                    ],
                  },
                },
                refreshingTemplate: {
                  entry: "clearRefreshTemplateError",
                  invoke: {
                    src: "getTemplate",
                    id: "refreshTemplate",
                    onDone: [
                      {
                        actions: "assignTemplate",
                        target: "requestingStart",
                      },
                    ],
                    onError: [
                      {
                        actions: ["assignRefreshTemplateError", "displayRefreshTemplateError"],
                        target: "idle",
                      },
                    ],
                  },
                },
              },
            },
            pollingResources: {
              initial: "gettingResources",
              states: {
                gettingResources: {
                  entry: "clearGetResourcesError",
                  invoke: {
                    src: "getResources",
                    id: "getResources",
                    onDone: [
                      {
                        actions: "assignResources",
                        target: "waiting",
                      },
                    ],
                    onError: [
                      {
                        actions: "assignGetResourcesError",
                        target: "waiting",
                      },
                    ],
                  },
                },
                waiting: {
                  after: {
                    "5000": {
                      target: "gettingResources",
                    },
                  },
                },
              },
            },
            timeline: {
              initial: "gettingBuilds",
              states: {
                idle: {},
                gettingBuilds: {
                  entry: "clearGetBuildsError",
                  invoke: {
                    src: "getBuilds",
                    onDone: [
                      {
                        actions: "assignBuilds",
                        target: "loadedBuilds",
                      },
                    ],
                    onError: [
                      {
                        actions: "assignGetBuildsError",
                        target: "idle",
                      },
                    ],
                  },
                },
                loadedBuilds: {
                  initial: "idle",
                  states: {
                    idle: {
                      on: {
                        LOAD_MORE_BUILDS: {
                          cond: "hasMoreBuilds",
                          target: "loadingMoreBuilds",
                        },
                        REFRESH_TIMELINE: {
                          target: "#workspaceState.ready.timeline.gettingBuilds",
                        },
                      },
                    },
                    loadingMoreBuilds: {
                      entry: "clearLoadMoreBuildsError",
                      invoke: {
                        src: "loadMoreBuilds",
                        onDone: [
                          {
                            actions: "assignNewBuilds",
                            target: "idle",
                          },
                        ],
                        onError: [
                          {
                            actions: "assignLoadMoreBuildsError",
                            target: "idle",
                          },
                        ],
                      },
                    },
                  },
                },
              },
            },
          },
        },
        error: {
          on: {
            GET_WORKSPACE: {
              target: "gettingWorkspace",
            },
          },
        },
      },
    },
    {
      actions: {
        // Clear data about an old workspace when looking at a new one
        clearContext: () =>
          assign({
            workspace: undefined,
            template: undefined,
            build: undefined,
            permissions: undefined,
          }),
        assignWorkspace: assign({
          workspace: (_, event) => event.data,
        }),
        assignGetWorkspaceError: assign({
          getWorkspaceError: (_, event) => event.data,
        }),
        clearGetWorkspaceError: (context) => assign({ ...context, getWorkspaceError: undefined }),
        assignTemplate: assign({
          template: (_, event) => event.data,
        }),
        assignPermissions: assign({
          // Setting event.data as Permissions to be more stricted. So we know
          // what permissions we asked for.
          permissions: (_, event) => event.data as Permissions,
        }),
        assignGetPermissionsError: assign({
          checkPermissionsError: (_, event) => event.data,
        }),
        clearGetPermissionsError: assign({
          checkPermissionsError: (_) => undefined,
        }),
        assignBuild: assign({
          build: (_, event) => event.data,
        }),
        assignBuildError: assign({
          buildError: (_, event) => event.data,
        }),
        clearBuildError: assign({
          buildError: (_) => undefined,
        }),
        assignCancellationMessage: assign({
          cancellationMessage: (_, event) => event.data,
        }),
        clearCancellationMessage: assign({
          cancellationMessage: (_) => undefined,
        }),
        displayCancellationMessage: (context) => {
          if (context.cancellationMessage) {
            displaySuccess(context.cancellationMessage.message)
          }
        },
        assignCancellationError: assign({
          cancellationError: (_, event) => event.data,
        }),
        clearCancellationError: assign({
          cancellationError: (_) => undefined,
        }),
        assignRefreshWorkspaceError: assign({
          refreshWorkspaceError: (_, event) => event.data,
        }),
        clearRefreshWorkspaceError: assign({
          refreshWorkspaceError: (_) => undefined,
        }),
        assignRefreshTemplateError: assign({
          refreshTemplateError: (_, event) => event.data,
        }),
        displayRefreshTemplateError: () => {
          displayError(Language.refreshTemplateError)
        },
        clearRefreshTemplateError: assign({
          refreshTemplateError: (_) => undefined,
        }),
        // Resources
        assignResources: assign({
          resources: (_, event) => event.data,
        }),
        assignGetResourcesError: assign({
          getResourcesError: (_, event) => event.data,
        }),
        clearGetResourcesError: assign({
          getResourcesError: (_) => undefined,
        }),
        // Timeline
        assignBuilds: assign({
          builds: (_, event) => event.data,
        }),
        assignGetBuildsError: assign({
          getBuildsError: (_, event) => event.data,
        }),
        clearGetBuildsError: assign({
          getBuildsError: (_) => undefined,
        }),
        assignNewBuilds: assign({
          builds: (context, event) => {
            const oldBuilds = context.builds

            if (!oldBuilds) {
              // This state is theoretically impossible, but helps TS
              throw new Error("workspaceXService: failed to load workspace builds")
            }

            return [...oldBuilds, ...event.data]
          },
        }),
        assignLoadMoreBuildsError: assign({
          loadMoreBuildsError: (_, event) => event.data,
        }),
        clearLoadMoreBuildsError: assign({
          loadMoreBuildsError: (_) => undefined,
        }),
        refreshTimeline: pure((context, event) => {
          // No need to refresh the timeline if it is not loaded
          if (!context.builds) {
            return
          }
          // When it is a refresh workspace event, we want to check if the latest
          // build was updated to not over fetch the builds
          if (event.type === "done.invoke.refreshWorkspace") {
            const latestBuildInTimeline = latestBuild(context.builds)
            const isUpdated =
              event.data?.latest_build.updated_at !== latestBuildInTimeline.updated_at
            if (isUpdated) {
              return send({ type: "REFRESH_TIMELINE" })
            }
          } else {
            return send({ type: "REFRESH_TIMELINE" })
          }
        }),
      },
      guards: {
        hasMoreBuilds: (_) => false,
      },
      services: {
        getWorkspace: async (_, event) => {
          return await API.getWorkspaceByOwnerAndName(event.username, event.workspaceName, {
            include_deleted: true,
          })
        },
        getTemplate: async (context) => {
          if (context.workspace) {
            return await API.getTemplate(context.workspace.template_id)
          } else {
            throw Error("Cannot get template without workspace")
          }
        },
        startWorkspace: async (context) => {
          if (context.workspace) {
            return await API.startWorkspace(
              context.workspace.id,
              context.template?.active_version_id,
            )
          } else {
            throw Error("Cannot start workspace without workspace id")
          }
        },
        stopWorkspace: async (context) => {
          if (context.workspace) {
            return await API.stopWorkspace(context.workspace.id)
          } else {
            throw Error("Cannot stop workspace without workspace id")
          }
        },
        deleteWorkspace: async (context) => {
          if (context.workspace) {
            return await API.deleteWorkspace(context.workspace.id)
          } else {
            throw Error("Cannot delete workspace without workspace id")
          }
        },
        cancelWorkspace: async (context) => {
          if (context.workspace) {
            return await API.cancelWorkspaceBuild(context.workspace.latest_build.id)
          } else {
            throw Error("Cannot cancel workspace without build id")
          }
        },
        refreshWorkspace: async (context) => {
          if (context.workspace) {
            return await API.getWorkspaceByOwnerAndName(
              context.workspace.owner_name,
              context.workspace.name,
              {
                include_deleted: true,
              },
            )
          } else {
            throw Error("Cannot refresh workspace without id")
          }
        },
        getResources: async (context) => {
          // If the job hasn't completed, fetching resources will result
          // in an unfriendly error for the user.
          if (!context.workspace?.latest_build.job.completed_at) {
            return []
          }
          const resources = await API.getWorkspaceResources(context.workspace.latest_build.id)
          return resources
        },
        getBuilds: async (context) => {
          if (context.workspace) {
            return await API.getWorkspaceBuilds(context.workspace.id)
          } else {
            throw Error("Cannot get builds without id")
          }
        },
        loadMoreBuilds: async (context) => {
          if (context.workspace) {
            return await API.getWorkspaceBuilds(context.workspace.id)
          } else {
            throw Error("Cannot load more builds without id")
          }
        },
        checkPermissions: async (context) => {
          if (context.workspace && context.userId) {
            return await API.checkUserPermissions(context.userId, {
              checks: permissionsToCheck(context.workspace),
            })
          } else {
            throw Error("Cannot check permissions without both workspace and user id")
          }
        },
      },
    },
  )

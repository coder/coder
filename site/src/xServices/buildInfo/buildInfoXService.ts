import { assign, createMachine } from "xstate"
import * as API from "../../api"
import * as Types from "../../api/types"

export interface BuildInfoContext {
  getBuildInfoError?: Error | unknown
  buildInfo?: Types.BuildInfoResponse
}

export const buildInfoMachine = createMachine(
  {
    tsTypes: {} as import("./buildInfoXService.typegen").Typegen0,
    schema: {
      context: {} as BuildInfoContext,
      services: {} as {
        getBuildInfo: {
          data: Types.BuildInfoResponse
        }
      },
    },
    context: {
      buildInfo: undefined,
    },
    id: "buildInfoState",
    initial: "gettingBuildInfo",
    states: {
      gettingBuildInfo: {
        invoke: {
          src: "getBuildInfo",
          id: "getBuildInfo",
          onDone: [
            {
              actions: ["assignBuildInfo", "clearGetBuildInfoError"],
            },
          ],
          onError: [
            {
              actions: ["assignGetBuildInfoError", "clearBuildInfo"],
            },
          ],
        },
      },
    },
  },
  {
    services: {
      getBuildInfo: API.getBuildInfo,
    },
    actions: {
      assignBuildInfo: assign({
        buildInfo: (_, event) => event.data,
      }),
      clearBuildInfo: assign((context: BuildInfoContext) => ({
        ...context,
        buildInfo: undefined,
      })),
      assignGetBuildInfoError: assign({
        getBuildInfoError: (_, event) => event.data,
      }),
      clearGetBuildInfoError: assign((context: BuildInfoContext) => ({
        ...context,
        getBuildInfoError: undefined,
      })),
    },
  },
)

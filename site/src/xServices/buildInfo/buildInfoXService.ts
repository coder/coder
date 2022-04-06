import { assign, createMachine } from "xstate"
import * as API from "../../api"
import * as Types from "../../api/types"

export interface BuildInfoContext {
  getBuildInfoError?: Error | unknown
  buildInfo?: Types.BuildInfoResponse
}

// TODO@asher: Add retry.  Maybe make it general for any API call (depending on
// the type of error)?
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
              actions: "assignGetBuildInfoError",
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

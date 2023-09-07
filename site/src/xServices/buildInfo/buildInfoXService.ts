import { assign, createMachine } from "xstate";
import * as API from "../../api/api";
import * as TypesGen from "../../api/typesGenerated";

export interface BuildInfoContext {
  getBuildInfoError?: unknown;
  buildInfo?: TypesGen.BuildInfoResponse;
}

export const buildInfoMachine = createMachine(
  {
    id: "buildInfoState",
    predictableActionArguments: true,
    tsTypes: {} as import("./buildInfoXService.typegen").Typegen0,
    schema: {
      context: {} as BuildInfoContext,
      services: {} as {
        getBuildInfo: {
          data: TypesGen.BuildInfoResponse;
        };
      },
    },
    context: {
      buildInfo: undefined,
    },
    initial: "gettingBuildInfo",
    states: {
      gettingBuildInfo: {
        invoke: {
          src: "getBuildInfo",
          id: "getBuildInfo",
          onDone: [
            {
              actions: ["assignBuildInfo", "clearGetBuildInfoError"],
              target: "#buildInfoState.success",
            },
          ],
          onError: [
            {
              actions: ["assignGetBuildInfoError", "clearBuildInfo"],
              target: "#buildInfoState.failure",
            },
          ],
        },
      },
      success: {
        type: "final",
      },
      failure: {
        type: "final",
      },
    },
  },
  {
    services: {
      getBuildInfo: async () => {
        // Build info is injected by the Coder server into the HTML document.
        const buildInfo = document.querySelector("meta[property=build-info]");
        if (buildInfo) {
          const rawContent = buildInfo.getAttribute("content");
          try {
            return JSON.parse(rawContent as string);
          } catch (ex) {
            // Ignore this and fetch as normal!
          }
        }

        return API.getBuildInfo();
      },
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
);

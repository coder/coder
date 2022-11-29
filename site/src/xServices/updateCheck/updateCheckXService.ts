import { assign, createMachine } from "xstate"
import * as API from "api/api"
import * as TypesGen from "api/typesGenerated"

export const Language = {
  updateAvailable: "New version available",
  updateAvailableMessage: (
    version: string,
    url: string,
    upgrade_instructions_url: string,
  ): string =>
    `Coder ${version} is now available at ${url}. See ${upgrade_instructions_url} for information on how to upgrade.`,
}

export interface UpdateCheckContext {
  getUpdateCheckError?: Error | unknown
  updateCheck?: TypesGen.UpdateCheckResponse
}

export const updateCheckMachine = createMachine(
  {
    id: "updateCheckState",
    predictableActionArguments: true,
    tsTypes: {} as import("./updateCheckXService.typegen").Typegen0,
    schema: {
      context: {} as UpdateCheckContext,
      services: {} as {
        getUpdateCheck: {
          data: TypesGen.UpdateCheckResponse
        }
      },
    },
    context: {
      updateCheck: undefined,
    },
    initial: "gettingUpdateCheck",
    states: {
      gettingUpdateCheck: {
        invoke: {
          src: "getUpdateCheck",
          id: "getUpdateCheck",
          onDone: [
            {
              actions: ["assignUpdateCheck", "clearGetUpdateCheckError"],
              target: "#updateCheckState.success",
            },
          ],
          onError: [
            {
              actions: ["assignGetUpdateCheckError", "clearUpdateCheck"],
              target: "#updateCheckState.failure",
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
      getUpdateCheck: API.getUpdateCheck,
    },
    actions: {
      assignUpdateCheck: assign({
        updateCheck: (_, event) => event.data,
      }),
      clearUpdateCheck: assign((context: UpdateCheckContext) => ({
        ...context,
        updateCheck: undefined,
      })),
      assignGetUpdateCheckError: assign({
        getUpdateCheckError: (_, event) => event.data,
      }),
      clearGetUpdateCheckError: assign((context: UpdateCheckContext) => ({
        ...context,
        getUpdateCheckError: undefined,
      })),
    },
  },
)

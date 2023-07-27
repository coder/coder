import { displaySuccess } from "components/GlobalSnackbar/utils"
import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import { AppearanceConfig } from "../../api/typesGenerated"

export type AppearanceContext = {
  appearance?: AppearanceConfig
  getAppearanceError?: unknown
  setAppearanceError?: unknown
  preview: boolean
}

export type AppearanceEvent =
  | { type: "SET_PREVIEW_APPEARANCE"; appearance: AppearanceConfig }
  | { type: "SAVE_APPEARANCE"; appearance: AppearanceConfig }

export const appearanceMachine = createMachine(
  {
    id: "appearanceMachine",
    predictableActionArguments: true,
    tsTypes: {} as import("./appearanceXService.typegen").Typegen0,
    schema: {
      context: {} as AppearanceContext,
      events: {} as AppearanceEvent,
      services: {
        getAppearance: {
          data: {} as AppearanceConfig,
        },
        setAppearance: {
          data: {},
        },
      },
    },
    context: {
      preview: false,
    },
    initial: "gettingAppearance",
    states: {
      idle: {
        on: {
          SET_PREVIEW_APPEARANCE: {
            actions: [
              "clearGetAppearanceError",
              "clearSetAppearanceError",
              "assignPreviewAppearance",
            ],
          },
          SAVE_APPEARANCE: "savingAppearance",
        },
      },
      gettingAppearance: {
        entry: "clearGetAppearanceError",
        invoke: {
          id: "getAppearance",
          src: "getAppearance",
          onDone: {
            target: "idle",
            actions: ["assignAppearance"],
          },
          onError: {
            target: "idle",
            actions: ["assignGetAppearanceError"],
          },
        },
      },
      savingAppearance: {
        entry: "clearSetAppearanceError",
        invoke: {
          id: "setAppearance",
          src: "setAppearance",
          onDone: {
            target: "idle",
            actions: ["assignAppearance", "notifyUpdateAppearanceSuccess"],
          },
          onError: {
            target: "idle",
            actions: ["assignSetAppearanceError"],
          },
        },
      },
    },
  },
  {
    actions: {
      assignPreviewAppearance: assign({
        appearance: (_, event) => event.appearance,
        preview: (_) => true,
      }),
      notifyUpdateAppearanceSuccess: () => {
        displaySuccess("Successfully updated appearance settings!")
      },
      assignAppearance: assign({
        appearance: (_, event) => event.data as AppearanceConfig,
        preview: (_) => false,
      }),
      assignGetAppearanceError: assign({
        getAppearanceError: (_, event) => event.data,
      }),
      clearGetAppearanceError: assign({
        getAppearanceError: (_) => undefined,
      }),
      assignSetAppearanceError: assign({
        setAppearanceError: (_, event) => event.data,
      }),
      clearSetAppearanceError: assign({
        setAppearanceError: (_) => undefined,
      }),
    },
    services: {
      getAppearance: async () => {
        // Appearance is injected by the Coder server into the HTML document.
        const appearance = document.querySelector("meta[property=appearance]")
        if (appearance) {
          const rawContent = appearance.getAttribute("content")
          try {
            return JSON.parse(rawContent as string)
          } catch (ex) {
            // Ignore this and fetch as normal!
          }
        }

        return API.getAppearance()
      },
      setAppearance: (_, event) => API.updateAppearance(event.appearance),
    },
  },
)

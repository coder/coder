import { displaySuccess } from "components/GlobalSnackbar/utils"
import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import { AppearanceConfig } from "../../api/typesGenerated"

export const Language = {
  getAppearanceError: "Error getting appearance.",
  setAppearanceError: "Error setting appearance.",
}

export type AppearanceContext = {
  appearance: AppearanceConfig
  getAppearanceError?: Error | unknown
  setAppearanceError?: Error | unknown
  preview: boolean
}

export type AppearanceEvent =
  | {
      type: "GET_APPEARANCE"
    }
  | { type: "SET_PREVIEW_APPEARANCE"; appearance: AppearanceConfig }
  | { type: "SET_APPEARANCE"; appearance: AppearanceConfig }

const emptyAppearance: AppearanceConfig = {
  logo_url: "",
  service_banner: {
    enabled: false,
  },
}

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
      appearance: emptyAppearance,
      preview: false,
    },
    initial: "idle",
    states: {
      idle: {
        on: {
          GET_APPEARANCE: "gettingAppearance",
          SET_PREVIEW_APPEARANCE: "settingPreviewAppearance",
          SET_APPEARANCE: "settingAppearance",
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
      settingPreviewAppearance: {
        entry: [
          "clearGetAppearanceError",
          "clearSetAppearanceError",
          "assignPreviewAppearance",
        ],
        always: {
          target: "idle",
        },
      },
      settingAppearance: {
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
        // The xState docs suggest that we can use a static value, but I failed
        // to find a way to do that that doesn't generate type errors.
        preview: (_, __) => true,
      }),
      notifyUpdateAppearanceSuccess: () => {
        displaySuccess("Successfully updated appearance settings!")
      },
      assignAppearance: assign({
        appearance: (_, event) => event.data as AppearanceConfig,
        preview: (_, __) => false,
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
      getAppearance: API.getAppearance,
      setAppearance: (_, event) => API.updateAppearance(event.appearance),
    },
  },
)

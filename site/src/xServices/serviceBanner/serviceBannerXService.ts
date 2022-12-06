import { displaySuccess } from "components/GlobalSnackbar/utils"
import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import { ServiceBanner } from "../../api/typesGenerated"

export const Language = {
  getServiceBannerError: "Error getting service banner.",
  setServiceBannerError: "Error setting service banner.",
}

export type ServiceBannerContext = {
  serviceBanner: ServiceBanner
  getServiceBannerError?: Error | unknown
  setServiceBannerError?: Error | unknown
  preview: boolean
}

export type ServiceBannerEvent =
  | {
      type: "GET_BANNER"
    }
  | { type: "SET_PREVIEW_BANNER"; serviceBanner: ServiceBanner }
  | { type: "SET_BANNER"; serviceBanner: ServiceBanner }

const emptyBanner = {
  enabled: false,
}

export const serviceBannerMachine = createMachine(
  {
    id: "serviceBannerMachine",
    predictableActionArguments: true,
    tsTypes: {} as import("./serviceBannerXService.typegen").Typegen0,
    schema: {
      context: {} as ServiceBannerContext,
      events: {} as ServiceBannerEvent,
      services: {
        getServiceBanner: {
          data: {} as ServiceBanner,
        },
        setServiceBanner: {
          data: {},
        },
      },
    },
    context: {
      serviceBanner: emptyBanner,
      preview: false,
    },
    initial: "idle",
    states: {
      idle: {
        on: {
          GET_BANNER: "gettingBanner",
          SET_PREVIEW_BANNER: "settingPreviewBanner",
          SET_BANNER: "settingBanner",
        },
      },
      gettingBanner: {
        entry: "clearGetBannerError",
        invoke: {
          id: "getBanner",
          src: "getBanner",
          onDone: {
            target: "idle",
            actions: ["assignBanner"],
          },
          onError: {
            target: "idle",
            actions: ["assignGetBannerError"],
          },
        },
      },
      settingPreviewBanner: {
        entry: [
          "clearGetBannerError",
          "clearSetBannerError",
          "assignPreviewBanner",
        ],
        always: {
          target: "idle",
        },
      },
      settingBanner: {
        entry: "clearSetBannerError",
        invoke: {
          id: "setBanner",
          src: "setBanner",
          onDone: {
            target: "idle",
            actions: ["assignBanner", "notifyUpdateBannerSuccess"],
          },
          onError: {
            target: "idle",
            actions: ["assignSetBannerError"],
          },
        },
      },
    },
  },
  {
    actions: {
      assignPreviewBanner: assign({
        serviceBanner: (_, event) => event.serviceBanner,
        // The xState docs suggest that we can use a static value, but I failed
        // to find a way to do that that doesn't generate type errors.
        preview: (_, __) => true,
      }),
      notifyUpdateBannerSuccess: () => {
        displaySuccess("Successfully updated Service Banner!")
      },
      assignBanner: assign({
        serviceBanner: (_, event) => event.data as ServiceBanner,
        preview: (_, __) => false,
      }),
      assignGetBannerError: assign({
        getServiceBannerError: (_, event) => event.data,
      }),
      clearGetBannerError: assign({
        getServiceBannerError: (_) => undefined,
      }),
      assignSetBannerError: assign({
        setServiceBannerError: (_, event) => event.data,
      }),
      clearSetBannerError: assign({
        setServiceBannerError: (_) => undefined,
      }),
    },
    services: {
      getBanner: API.getServiceBanner,
      setBanner: (_, event) => API.setServiceBanner(event.serviceBanner),
    },
  },
)

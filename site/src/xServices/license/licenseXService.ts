import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import { LicenseData  } from "../../api/types"

export const Language = {
  getLicenseError: "Error getting license information.",
}

/* deserves more thought but this is one way to handle unlicensed cases */
const defaultLicenseData = {
  warnings: [],
  features: {
    audit: {
      enabled: false,
      entitled: false
    },
    createUser: {
      enabled: true,
      entitled: true,
      limit: 0
    },
    createOrg: {
      enabled: false,
      entitled: false
    }
  }
}

export type LicenseContext = {
  licenseData: LicenseData
  getLicenseError?: Error | unknown
}

export type LicenseEvent = {
  type: "GET_LICENSE_DATA"
}

export const licenseMachine = createMachine(
  {
    id: "licenseMachine",
    initial: "idle",
    schema: {
      context: {} as LicenseContext,
      events: {} as LicenseEvent,
      services: {
        getLicenseData: {
          data: {} as LicenseData,
        },
      },
    },
    tsTypes: {} as import("./licenseXService.typegen").Typegen0,
    context: {
      licenseData: defaultLicenseData
    },
    states: {
      idle: {
        on: {
          GET_LICENSE_DATA: "gettingLicenseData",
        },
      },
      gettingLicenseData: {
        entry: "clearGetLicenseError",
        invoke: {
          id: "getLicenseData",
          src: "getLicenseData",
          onDone: {
            target: "idle",
            actions: ["assignLicenseData"],
          },
          onError: {
            target: "idle",
            actions: ["assignGetLicenseError"],
          },
        },
      },
    },
  },
  {
    actions: {
      assignLicenseData: assign({
        licenseData: (_, event) => event.data,
      }),
      assignGetLicenseError: assign({
        getLicenseError: (_, event) => event.data,
      }),
      clearGetLicenseError: assign({
        getLicenseError: (_) => undefined,
      }),
    },
    services: {
      getLicenseData: () => API.getLicenseData(),
    },
  },
)

import { getTemplateVersionByName } from "api/api"
import { TemplateVersion } from "api/typesGenerated"
import {
  getTemplateVersionFiles,
  TemplateVersionFiles,
} from "util/templateVersion"
import { assign, createMachine } from "xstate"

export interface TemplateVersionMachineContext {
  orgId: string
  versionName: string
  version?: TemplateVersion
  files?: TemplateVersionFiles
  error?: Error | unknown
}

export const templateVersionMachine = createMachine(
  {
    predictableActionArguments: true,
    id: "templateVersion",
    schema: {
      context: {} as TemplateVersionMachineContext,
      services: {} as {
        loadVersion: {
          data: TemplateVersion
        }
        loadFiles: {
          data: TemplateVersionFiles
        }
      },
    },
    tsTypes: {} as import("./templateVersionXService.typegen").Typegen0,
    initial: "loadingVersion",
    states: {
      loadingVersion: {
        invoke: {
          src: "loadVersion",
          onDone: {
            target: "loadingFiles",
            actions: ["assignVersion"],
          },
          onError: {
            target: "done.error",
            actions: ["assignError"],
          },
        },
      },
      loadingFiles: {
        invoke: {
          src: "loadFiles",
          onDone: {
            target: "done.ok",
            actions: ["assignFiles"],
          },
          onError: {
            target: "done.error",
            actions: ["assignError"],
          },
        },
      },
      done: {
        states: {
          ok: { type: "final" },
          error: { type: "final" },
        },
      },
    },
  },
  {
    actions: {
      assignError: assign({
        error: (_, { data }) => data,
      }),
      assignVersion: assign({
        version: (_, { data }) => data,
      }),
      assignFiles: assign({
        files: (_, { data }) => data,
      }),
    },
    services: {
      loadVersion: ({ orgId, versionName }) =>
        getTemplateVersionByName(orgId, versionName),
      loadFiles: async ({ version }) => {
        if (!version) {
          throw new Error("Version is not defined")
        }
        return getTemplateVersionFiles(version, ["tf", "md"])
      },
    },
  },
)

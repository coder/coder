import { getFile, getTemplateVersionByName } from "api/api"
import { TemplateVersion } from "api/typesGenerated"
import { assign, createMachine } from "xstate"
import untar from "js-untar"

/**
 * Content by filename
 */
export type TemplateVersionFiles = Record<string, string>

/**
 * File extensions to be displayed
 */
const allowedExtensions = ["tf", "md"]

export const templateVersionMachine = createMachine(
  {
    predictableActionArguments: true,
    id: "templateVersion",
    schema: {
      context: {} as {
        orgId: string
        versionName: string
        version?: TemplateVersion
        files?: TemplateVersionFiles
        error?: unknown
      },
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
        const files: TemplateVersionFiles = {}
        const tarFile = await getFile(version.job.file_id)
        await untar(tarFile).then(undefined, undefined, async (file) => {
          const paths = file.name.split("/")
          const filename = paths[paths.length - 1]
          const [_, extension] = filename.split(".")
          if (allowedExtensions.includes(extension)) {
            files[filename] = file.readAsString()
          }
        })
        return files
      },
    },
  },
)

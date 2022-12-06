import {
  getPreviousTemplateVersionByName,
  GetPreviousTemplateVersionByNameResponse,
  getTemplateVersionByName,
} from "api/api"
import { TemplateVersion } from "api/typesGenerated"
import {
  getTemplateVersionFiles,
  TemplateVersionFiles,
} from "util/templateVersion"
import { assign, createMachine } from "xstate"

export interface TemplateVersionMachineContext {
  orgId: string
  versionName: string
  currentVersion?: TemplateVersion
  currentFiles?: TemplateVersionFiles
  error?: Error | unknown
  // Get file diffs
  previousVersion?: TemplateVersion
  previousFiles?: TemplateVersionFiles
}

export const templateVersionMachine = createMachine(
  {
    predictableActionArguments: true,
    id: "templateVersion",
    schema: {
      context: {} as TemplateVersionMachineContext,
      services: {} as {
        loadVersions: {
          data: {
            currentVersion: GetPreviousTemplateVersionByNameResponse
            previousVersion: GetPreviousTemplateVersionByNameResponse
          }
        }
        loadFiles: {
          data: {
            currentFiles: TemplateVersionFiles
            previousFiles: TemplateVersionFiles
          }
        }
      },
    },
    tsTypes: {} as import("./templateVersionXService.typegen").Typegen0,
    initial: "loadingVersions",
    states: {
      loadingVersions: {
        invoke: {
          src: "loadVersions",
          onDone: {
            target: "loadingFiles",
            actions: ["assignVersions"],
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
      assignVersions: assign({
        currentVersion: (_, { data }) => data.currentVersion,
        previousVersion: (_, { data }) => data.previousVersion,
      }),
      assignFiles: assign({
        currentFiles: (_, { data }) => data.currentFiles,
        previousFiles: (_, { data }) => data.previousFiles,
      }),
    },
    services: {
      loadVersions: async ({ orgId, versionName }) => {
        const [currentVersion, previousVersion] = await Promise.all([
          getTemplateVersionByName(orgId, versionName),
          getPreviousTemplateVersionByName(orgId, versionName),
        ])

        return {
          currentVersion,
          previousVersion,
        }
      },
      loadFiles: async ({ currentVersion, previousVersion }) => {
        if (!currentVersion) {
          throw new Error("Version is not defined")
        }
        const loadFilesPromises: ReturnType<typeof getTemplateVersionFiles>[] =
          []
        const allowedExtensions = ["tf", "md"]
        const allowedFiles = ["Dockerfile"]
        loadFilesPromises.push(
          getTemplateVersionFiles(
            currentVersion,
            allowedExtensions,
            allowedFiles,
          ),
        )
        if (previousVersion) {
          loadFilesPromises.push(
            getTemplateVersionFiles(
              previousVersion,
              allowedExtensions,
              allowedFiles,
            ),
          )
        }
        const [currentFiles, previousFiles] = await Promise.all(
          loadFilesPromises,
        )
        return {
          currentFiles,
          previousFiles,
        }
      },
    },
  },
)

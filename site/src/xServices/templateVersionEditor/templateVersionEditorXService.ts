import {
  ProvisionerJobLog,
  ProvisionerJobStatus,
  TemplateVersion,
  TemplateVersionVariable,
  UploadResponse,
  VariableValue,
  WorkspaceResource,
} from "api/typesGenerated"
import { assign, createMachine } from "xstate"
import * as API from "api/api"
import { FileTree, traverse } from "utils/filetree"
import { isAllowedFile } from "utils/templateVersion"
import { TarReader, TarWriter } from "utils/tar"
import { PublishVersionData } from "pages/TemplateVersionPage/TemplateVersionEditorPage/types"

export interface TemplateVersionEditorMachineContext {
  orgId: string
  templateId?: string
  fileTree?: FileTree
  uploadResponse?: UploadResponse
  version?: TemplateVersion
  resources?: WorkspaceResource[]
  buildLogs?: ProvisionerJobLog[]
  tarReader?: TarReader
  publishingError?: unknown
  missingVariables?: TemplateVersionVariable[]
  missingVariableValues?: VariableValue[]
}

export const templateVersionEditorMachine = createMachine(
  {
    /** @xstate-layout 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 */
    predictableActionArguments: true,
    id: "templateVersionEditor",
    schema: {
      context: {} as TemplateVersionEditorMachineContext,
      events: {} as
        | { type: "INITIALIZE"; tarReader: TarReader }
        | {
            type: "CREATE_VERSION"
            fileTree: FileTree
            templateId: string
          }
        | { type: "CANCEL_VERSION" }
        | { type: "SET_MISSING_VARIABLE_VALUES"; values: VariableValue[] }
        | { type: "CANCEL_MISSING_VARIABLE_VALUES" }
        | { type: "ADD_BUILD_LOG"; log: ProvisionerJobLog }
        | { type: "BUILD_DONE" }
        | { type: "PUBLISH" }
        | ({ type: "CONFIRM_PUBLISH" } & PublishVersionData)
        | { type: "CANCEL_PUBLISH" },

      services: {} as {
        uploadTar: {
          data: UploadResponse
        }
        createBuild: {
          data: TemplateVersion
        }
        cancelBuild: {
          data: void
        }
        fetchVersion: {
          data: TemplateVersion
        }
        getResources: {
          data: WorkspaceResource[]
        }
        publishingVersion: {
          data: void
        }
        loadMissingVariables: {
          data: TemplateVersionVariable[]
        }
      },
    },
    tsTypes: {} as import("./templateVersionEditorXService.typegen").Typegen0,
    initial: "initializing",
    states: {
      initializing: {
        on: {
          INITIALIZE: {
            actions: ["assignTarReader"],
            target: "idle",
          },
        },
      },

      idle: {
        on: {
          CREATE_VERSION: {
            actions: ["assignCreateBuild"],
            target: "cancelingInProgressBuild",
          },
          PUBLISH: {
            target: "askPublishParameters",
          },
        },
      },

      askPublishParameters: {
        on: {
          CANCEL_PUBLISH: "idle",
          CONFIRM_PUBLISH: "publishingVersion",
        },
      },

      publishingVersion: {
        tags: "loading",
        entry: ["clearPublishingError"],
        invoke: {
          id: "publishingVersion",
          src: "publishingVersion",

          onError: {
            actions: ["assignPublishingError"],
            target: "askPublishParameters",
          },

          onDone: {
            target: ["idle"],
          },
        },
      },

      cancelingInProgressBuild: {
        tags: "loading",
        invoke: {
          id: "cancelBuild",
          src: "cancelBuild",
          onDone: {
            target: "uploadTar",
          },
        },
      },

      uploadTar: {
        tags: "loading",
        invoke: {
          id: "uploadTar",
          src: "uploadTar",
          onDone: {
            target: "creatingBuild",
            actions: "assignUploadResponse",
          },
        },
      },

      creatingBuild: {
        tags: "loading",
        invoke: {
          id: "createBuild",
          src: "createBuild",
          onDone: {
            actions: "assignBuild",
            target: "watchingBuildLogs",
          },
        },
      },

      watchingBuildLogs: {
        tags: "loading",
        invoke: {
          id: "watchBuildLogs",
          src: "watchBuildLogs",
        },
        on: {
          ADD_BUILD_LOG: {
            actions: "addBuildLog",
          },
          BUILD_DONE: "fetchingVersion",
          CANCEL_VERSION: {
            target: "cancelingInProgressBuild",
          },
          CREATE_VERSION: {
            actions: ["assignCreateBuild"],
            target: "uploadTar",
          },
        },
      },

      fetchingVersion: {
        tags: "loading",
        invoke: {
          id: "fetchVersion",
          src: "fetchVersion",

          onDone: [
            {
              actions: ["assignBuild"],
              target: "promptVariables",
              cond: "jobFailedWithMissingVariables",
            },
            {
              actions: ["assignBuild"],
              target: "fetchResources",
            },
          ],
        },
      },

      promptVariables: {
        initial: "loadingMissingVariables",
        states: {
          loadingMissingVariables: {
            invoke: {
              src: "loadMissingVariables",
              onDone: {
                actions: "assignMissingVariables",
                target: "idle",
              },
            },
          },
          idle: {
            on: {
              SET_MISSING_VARIABLE_VALUES: {
                actions: "assignMissingVariableValues",
                target: "#templateVersionEditor.creatingBuild",
              },
              CANCEL_MISSING_VARIABLE_VALUES: {
                target: "#templateVersionEditor.idle",
              },
            },
          },
        },
      },

      fetchResources: {
        tags: "loading",
        invoke: {
          id: "getResources",
          src: "getResources",
          onDone: {
            actions: ["assignResources"],
            target: "idle",
          },
        },
      },
    },
  },
  {
    actions: {
      assignCreateBuild: assign({
        fileTree: (_, event) => event.fileTree,
        templateId: (_, event) => event.templateId,
        buildLogs: (_, _1) => [],
        resources: (_, _1) => [],
      }),
      assignResources: assign({
        resources: (_, event) => event.data,
      }),
      assignUploadResponse: assign({
        uploadResponse: (_, event) => event.data,
      }),
      assignBuild: assign({
        version: (_, event) => event.data,
      }),
      addBuildLog: assign({
        buildLogs: (context, event) => {
          const previousLogs = context.buildLogs ?? []
          return [...previousLogs, event.log]
        },
        // Instead of periodically fetching the version,
        // we just assume the state is running after the first log.
        //
        // The machine fetches the version after the log stream ends anyways!
        version: (context) => {
          if (!context.version || context.buildLogs?.length !== 0) {
            return context.version
          }
          return {
            ...context.version,
            job: {
              ...context.version.job,
              status: "running" as ProvisionerJobStatus,
            },
          }
        },
      }),
      assignTarReader: assign({
        tarReader: (_, { tarReader }) => tarReader,
      }),
      assignPublishingError: assign({
        publishingError: (_, event) => event.data,
      }),
      clearPublishingError: assign({ publishingError: (_) => undefined }),
      assignMissingVariables: assign({
        missingVariables: (_, event) => event.data,
      }),
      assignMissingVariableValues: assign({
        missingVariableValues: (_, event) => event.values,
      }),
    },
    services: {
      uploadTar: async ({ fileTree, tarReader }) => {
        if (!fileTree) {
          throw new Error("file tree must to be set")
        }
        if (!tarReader) {
          throw new Error("tar reader must to be set")
        }
        const tar = new TarWriter()

        // Add previous non editable files
        for (const file of tarReader.fileInfo) {
          if (!isAllowedFile(file.name)) {
            if (file.type === "5") {
              tar.addFolder(file.name, {
                mode: file.mode, // https://github.com/beatgammit/tar-js/blob/master/lib/tar.js#L42
                mtime: file.mtime,
                user: file.user,
                group: file.group,
              })
            } else {
              tar.addFile(
                file.name,
                tarReader.getTextFile(file.name) as string,
                {
                  mode: file.mode, // https://github.com/beatgammit/tar-js/blob/master/lib/tar.js#L42
                  mtime: file.mtime,
                  user: file.user,
                  group: file.group,
                },
              )
            }
          }
        }
        // Add the editable files
        traverse(fileTree, (content, _filename, fullPath) => {
          // When a file is deleted. Don't add it to the tar.
          if (content === undefined) {
            return
          }

          if (typeof content === "string") {
            tar.addFile(fullPath, content)
            return
          }

          tar.addFolder(fullPath)
        })
        const blob = (await tar.write()) as Blob
        return API.uploadTemplateFile(new File([blob], "template.tar"))
      },
      createBuild: (ctx) => {
        if (!ctx.uploadResponse) {
          throw new Error("no upload response")
        }
        return API.createTemplateVersion(ctx.orgId, {
          provisioner: "terraform",
          storage_method: "file",
          tags: {},
          template_id: ctx.templateId,
          file_id: ctx.uploadResponse.hash,
          user_variable_values: ctx.missingVariableValues,
        })
      },
      fetchVersion: (ctx) => {
        if (!ctx.version) {
          throw new Error("template version must be set")
        }
        return API.getTemplateVersion(ctx.version.id)
      },
      watchBuildLogs:
        ({ version }) =>
        async (callback) => {
          if (!version) {
            throw new Error("version must be set")
          }

          const socket = API.watchBuildLogsByTemplateVersionId(version.id, {
            onMessage: (log) => {
              callback({ type: "ADD_BUILD_LOG", log })
            },
            onDone: () => {
              callback({ type: "BUILD_DONE" })
            },
            onError: (error) => {
              console.error(error)
            },
          })

          return () => {
            socket.close()
          }
        },
      getResources: (ctx) => {
        if (!ctx.version) {
          throw new Error("template version must be set")
        }
        return API.getTemplateVersionResources(ctx.version.id)
      },
      cancelBuild: async (ctx) => {
        if (!ctx.version) {
          return
        }
        if (ctx.version.job.status === "running") {
          await API.cancelTemplateVersionBuild(ctx.version.id)
        }
      },
      publishingVersion: async (
        { version, templateId },
        { name, message, isActiveVersion },
      ) => {
        if (!version) {
          throw new Error("Version is not set")
        }
        if (!templateId) {
          throw new Error("Template is not set")
        }
        const haveChanges = name !== version.name || message !== version.message
        await Promise.all([
          haveChanges
            ? API.patchTemplateVersion(version.id, { name, message })
            : Promise.resolve(),
          isActiveVersion
            ? API.updateActiveTemplateVersion(templateId, {
                id: version.id,
              })
            : Promise.resolve(),
        ])
      },
      loadMissingVariables: ({ version }) => {
        if (!version) {
          throw new Error("Version is not set")
        }
        const variables = API.getTemplateVersionVariables(version.id)
        return variables
      },
    },
    guards: {
      jobFailedWithMissingVariables: (_, { data }) => {
        return data.job.error_code === "REQUIRED_TEMPLATE_VARIABLES"
      },
    },
  },
)

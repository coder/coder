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
            target: "cancelingBuild",
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
          onDone: {
            actions: ["onPublish"],
          },
          onError: {
            actions: ["assignPublishingError"],
            target: "askPublishParameters",
          },
        },
      },
      cancelingBuild: {
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
            target: "cancelingBuild",
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
        { name, isActiveVersion },
      ) => {
        if (!version) {
          throw new Error("Version is not set")
        }
        if (!templateId) {
          throw new Error("Template is not set")
        }
        await Promise.all([
          // Only do a patch if the name is different
          name !== version.name
            ? API.patchTemplateVersion(version.id, { name })
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

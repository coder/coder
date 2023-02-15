import {
  ProvisionerJobLog,
  ProvisionerJobStatus,
  TemplateVersion,
  UploadResponse,
  WorkspaceResource,
} from "api/typesGenerated"
import { assign, createMachine } from "xstate"
import * as API from "api/api"
import { File as UntarFile } from "js-untar"
import { FileTree, traverse } from "util/filetree"
import { isAllowedFile } from "util/templateVersion"
import { saveAs } from "file-saver"
import { TarWriter } from "util/tar"

export interface CreateVersionData {
  file: File
}

export interface TemplateVersionEditorMachineContext {
  orgId: string
  templateId?: string
  fileTree?: FileTree
  uploadResponse?: UploadResponse
  version?: TemplateVersion
  resources?: WorkspaceResource[]
  buildLogs?: ProvisionerJobLog[]
  untarFiles?: UntarFile[]
}

export const templateVersionEditorMachine = createMachine(
  {
    predictableActionArguments: true,
    id: "templateVersionEditor",
    schema: {
      context: {} as TemplateVersionEditorMachineContext,
      events: {} as
        | { type: "INITIALIZE"; untarFiles: UntarFile[] }
        | {
            type: "CREATE_VERSION"
            fileTree: FileTree
            templateId: string
          }
        | { type: "CANCEL_VERSION" }
        | { type: "ADD_BUILD_LOG"; log: ProvisionerJobLog }
        | { type: "UPDATE_ACTIVE_VERSION" },
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
        updateActiveVersion: {
          data: void
        }
      },
    },
    tsTypes: {} as import("./templateVersionEditorXService.typegen").Typegen0,
    initial: "initializing",
    states: {
      initializing: {
        on: {
          INITIALIZE: {
            actions: ["assignUntarFiles"],
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
          UPDATE_ACTIVE_VERSION: {
            target: "updatingActiveVersion",
          },
        },
      },
      updatingActiveVersion: {
        tags: "loading",
        invoke: {
          id: "updateActiveVersion",
          src: "updateActiveVersion",
          onDone: {
            target: "idle",
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
          onDone: {
            target: "fetchingVersion",
          },
        },
        on: {
          ADD_BUILD_LOG: {
            actions: "addBuildLog",
          },
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
          onDone: {
            actions: ["assignBuild"],
            target: "fetchResources",
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
      assignUntarFiles: assign({
        untarFiles: (_, { untarFiles }) => untarFiles,
      }),
    },
    services: {
      uploadTar: async ({ fileTree, untarFiles }) => {
        if (!fileTree) {
          throw new Error("file tree must to be set")
        }
        if (!untarFiles) {
          throw new Error("untar files must to be set")
        }
        const tar = new TarWriter()

        // Add previous non editable files
        for (const untarFile of untarFiles) {
          if (!isAllowedFile(untarFile.name)) {
            if (untarFile.type === "5") {
              tar.addFolder(untarFile.name, {
                mode: parseInt(untarFile.mode, 8) & 0xfff, // https://github.com/beatgammit/tar-js/blob/master/lib/tar.js#L42
                mtime: untarFile.mtime,
                user: untarFile.uname,
                group: untarFile.gname,
              })
            } else {
              const buffer = await untarFile.blob.arrayBuffer()
              tar.addFile(untarFile.name, new Uint8Array(buffer), {
                mode: parseInt(untarFile.mode, 8) & 0xfff, // https://github.com/beatgammit/tar-js/blob/master/lib/tar.js#L42
                mtime: untarFile.mtime,
                user: untarFile.uname,
                group: untarFile.gname,
              })
            }
          }
        }
        // Add the editable files
        traverse(fileTree, (content, _filename, fullPath) => {
          if (typeof content === "string") {
            tar.addFile(fullPath, content)
          }
        })
        const blob = await tar.write()
        saveAs(blob, "template.tar")
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
        })
      },
      fetchVersion: (ctx) => {
        if (!ctx.version) {
          throw new Error("template version must be set")
        }
        return API.getTemplateVersion(ctx.version.id)
      },
      watchBuildLogs: (ctx) => async (callback) => {
        return new Promise<void>((resolve, reject) => {
          if (!ctx.version) {
            return reject("version must be set")
          }
          const proto = location.protocol === "https:" ? "wss:" : "ws:"
          const socket = new WebSocket(
            `${proto}//${location.host}/api/v2/templateversions/${ctx.version?.id}/logs?follow=true`,
          )
          socket.binaryType = "blob"
          socket.addEventListener("message", (event) => {
            callback({ type: "ADD_BUILD_LOG", log: JSON.parse(event.data) })
          })
          socket.addEventListener("error", () => {
            reject(new Error("socket errored"))
          })
          socket.addEventListener("close", () => {
            // When the socket closes, logs have finished streaming!
            resolve()
          })
        })
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
      updateActiveVersion: async (ctx) => {
        if (!ctx.templateId) {
          throw new Error("template must be set")
        }
        if (!ctx.version) {
          throw new Error("template version must be set")
        }
        await API.updateActiveTemplateVersion(ctx.templateId, {
          id: ctx.version.id,
        })
      },
    },
  },
)

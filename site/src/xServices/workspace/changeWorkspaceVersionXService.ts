import {
  getTemplate,
  getTemplateVersions,
  getWorkspaceByOwnerAndName,
  startWorkspace,
} from "api/api"
import {
  Template,
  TemplateVersion,
  Workspace,
  WorkspaceBuild,
} from "api/typesGenerated"
import { assign, createMachine } from "xstate"

export interface ChangeWorkspaceVersionContext {
  owner: string
  workspaceName: string
  workspace?: Workspace
  template?: Template
  templateVersions?: TemplateVersion[]
  error?: unknown
}

interface ChangeWorkspaceVersionSchema {
  context: ChangeWorkspaceVersionContext

  services: {
    getWorkspace: {
      data: Workspace
    }
    getTemplateData: {
      data: {
        template: Template
        versions: TemplateVersion[]
      }
    }
    updateVersion: {
      data: WorkspaceBuild
    }
  }

  events: {
    type: "UPDATE_VERSION"
    versionId: string
  }
}

export const changeWorkspaceVersionMachine = createMachine(
  {
    id: "changeWorkspaceVersion",
    predictableActionArguments: true,
    schema: {} as ChangeWorkspaceVersionSchema,
    tsTypes: {} as import("./changeWorkspaceVersionXService.typegen").Typegen0,
    initial: "loadingWorkspace",
    states: {
      loadingWorkspace: {
        invoke: {
          src: "getWorkspace",
          onDone: {
            target: "loadingTemplateData",
            actions: "assignWorkspace",
          },
          onError: {
            target: "idle",
            actions: "assignError",
          },
        },
      },
      loadingTemplateData: {
        invoke: {
          src: "getTemplateData",
          onDone: {
            target: "idle",
            actions: "assignTemplateData",
          },
          onError: {
            target: "idle",
            actions: "assignError",
          },
        },
      },
      idle: {
        on: {
          UPDATE_VERSION: "updatingVersion",
        },
      },
      updatingVersion: {
        invoke: {
          src: "updateVersion",
          onDone: {
            target: "idle",
            actions: "onUpdateVersion",
          },
          onError: {
            target: "idle",
            actions: "assignError",
          },
        },
      },
    },
  },
  {
    services: {
      getWorkspace: ({ owner, workspaceName }) =>
        getWorkspaceByOwnerAndName(owner, workspaceName),

      getTemplateData: async ({ workspace }) => {
        if (!workspace) {
          throw new Error("Workspace not defined.")
        }

        const [template, versions] = await Promise.all([
          getTemplate(workspace.template_id),
          getTemplateVersions(workspace.template_id),
        ])

        return { template, versions }
      },

      updateVersion: ({ workspace }, { versionId }) => {
        if (!workspace) {
          throw new Error("Workspace not defined.")
        }

        return startWorkspace(workspace.id, versionId)
      },
    },

    actions: {
      assignError: assign({
        error: (_, { data }) => data,
      }),

      assignWorkspace: assign({
        workspace: (_, { data }) => data,
      }),

      assignTemplateData: assign({
        template: (_, { data }) => data.template,
        templateVersions: (_, { data }) => data.versions,
      }),
    },
  },
)

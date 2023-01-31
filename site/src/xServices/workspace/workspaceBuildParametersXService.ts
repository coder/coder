import {
  getTemplateVersionRichParameters,
  getWorkspaceByOwnerAndName,
  getWorkspaceBuildParameters,
  postWorkspaceBuild,
} from "api/api"
import {
  CreateWorkspaceBuildRequest,
  Template,
  TemplateVersionParameter,
  Workspace,
  WorkspaceBuild,
  WorkspaceBuildParameter,
} from "api/typesGenerated"
import { assign, createMachine } from "xstate"

type WorkspaceBuildParametersContext = {
  workspaceOwner: string
  workspaceName: string

  selectedWorkspace?: Workspace
  selectedTemplate?: Template
  templateParameters?: TemplateVersionParameter[]
  workspaceBuildParameters?: WorkspaceBuildParameter[]

  createWorkspaceBuildRequest?: CreateWorkspaceBuildRequest

  getWorkspaceError?: Error | unknown
  getTemplateParametersError?: Error | unknown
  getWorkspaceBuildParametersError?: Error | unknown
  updateWorkspaceError?: Error | unknown
}

type UpdateWorkspaceEvent = {
  type: "UPDATE_WORKSPACE"
  request: CreateWorkspaceBuildRequest
}

export const workspaceBuildParametersMachine = createMachine(
  {
    id: "workspaceBuildParametersState",
    predictableActionArguments: true,
    tsTypes:
      {} as import("./workspaceBuildParametersXService.typegen").Typegen0,
    schema: {
      context: {} as WorkspaceBuildParametersContext,
      events: {} as UpdateWorkspaceEvent,
      services: {} as {
        getWorkspace: {
          data: Workspace
        }
        getTemplateParameters: {
          data: TemplateVersionParameter[]
        }
        getWorkspaceBuildParameters: {
          data: WorkspaceBuildParameter[]
        }
        updateWorkspace: {
          data: WorkspaceBuild
        }
      },
    },
    initial: "gettingWorkspace",
    states: {
      gettingWorkspace: {
        entry: "clearGetWorkspaceError",
        invoke: {
          src: "getWorkspace",
          onDone: [
            {
              actions: ["assignWorkspace"],
              target: "gettingTemplateParameters",
            },
          ],
          onError: {
            actions: ["assignGetWorkspaceError"],
            target: "error",
          },
        },
      },
      gettingTemplateParameters: {
        entry: "clearGetTemplateParametersError",
        invoke: {
          src: "getTemplateParameters",
          onDone: [
            {
              actions: ["assignTemplateParameters"],
              target: "gettingWorkspaceBuildParameters",
            },
          ],
          onError: {
            actions: ["assignGetTemplateParametersError"],
            target: "error",
          },
        },
      },
      gettingWorkspaceBuildParameters: {
        entry: "clearGetWorkspaceBuildParametersError",
        invoke: {
          src: "getWorkspaceBuildParameters",
          onDone: {
            actions: ["assignWorkspaceBuildParameters"],
            target: "fillingParams",
          },
          onError: {
            actions: ["assignGetWorkspaceBuildParametersError"],
            target: "error",
          },
        },
      },
      fillingParams: {
        on: {
          UPDATE_WORKSPACE: {
            actions: ["assignCreateWorkspaceBuildRequest"],
            target: "updatingWorkspace",
          },
        },
      },
      updatingWorkspace: {
        entry: "clearUpdateWorkspaceError",
        invoke: {
          src: "updateWorkspace",
          onDone: {
            actions: ["onUpdateWorkspace"],
            target: "updated",
          },
          onError: {
            actions: ["assignUpdateWorkspaceError"],
            target: "fillingParams",
          },
        },
      },
      updated: {
        entry: "onUpdateWorkspace",
        type: "final",
      },
      error: {},
    },
  },
  {
    services: {
      getWorkspace: (context) => {
        const { workspaceOwner, workspaceName } = context
        return getWorkspaceByOwnerAndName(workspaceOwner, workspaceName)
      },
      getTemplateParameters: (context) => {
        const { selectedWorkspace } = context

        if (!selectedWorkspace) {
          throw new Error("No workspace selected")
        }

        return getTemplateVersionRichParameters(
          selectedWorkspace.latest_build.template_version_id,
        )
      },
      getWorkspaceBuildParameters: (context) => {
        const { selectedWorkspace } = context

        if (!selectedWorkspace) {
          throw new Error("No workspace selected")
        }

        return getWorkspaceBuildParameters(selectedWorkspace.latest_build.id)
      },
      updateWorkspace: (context) => {
        const { selectedWorkspace, createWorkspaceBuildRequest } = context

        if (!selectedWorkspace) {
          throw new Error("No workspace selected")
        }

        if (!createWorkspaceBuildRequest) {
          throw new Error("No workspace build request")
        }

        return postWorkspaceBuild(
          selectedWorkspace.id,
          createWorkspaceBuildRequest,
        )
      },
    },
    actions: {
      assignWorkspace: assign({
        selectedWorkspace: (_, event) => event.data,
      }),
      assignTemplateParameters: assign({
        templateParameters: (_, event) => event.data,
      }),
      assignWorkspaceBuildParameters: assign({
        workspaceBuildParameters: (_, event) => event.data,
      }),

      assignCreateWorkspaceBuildRequest: assign({
        createWorkspaceBuildRequest: (_, event) => event.request,
      }),
      assignGetWorkspaceError: assign({
        getWorkspaceError: (_, event) => event.data,
      }),
      clearGetWorkspaceError: assign({
        getWorkspaceError: (_) => undefined,
      }),
      assignGetTemplateParametersError: assign({
        getTemplateParametersError: (_, event) => event.data,
      }),
      clearGetTemplateParametersError: assign({
        getTemplateParametersError: (_) => undefined,
      }),
      clearGetWorkspaceBuildParametersError: assign({
        getWorkspaceBuildParametersError: (_) => undefined,
      }),
      assignGetWorkspaceBuildParametersError: assign({
        getWorkspaceBuildParametersError: (_, event) => event.data,
      }),
      clearUpdateWorkspaceError: assign({
        updateWorkspaceError: (_) => undefined,
      }),
      assignUpdateWorkspaceError: assign({
        updateWorkspaceError: (_, event) => event.data,
      }),
    },
  },
)

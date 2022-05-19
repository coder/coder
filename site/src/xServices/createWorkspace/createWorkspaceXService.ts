import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import * as TypesGen from "../../api/typesGenerated"

interface CreateWorkspaceContext {
  name: string

  organizations?: TypesGen.Organization[]
  organizationsError?: Error | unknown
  template?: TypesGen.Template
  templateError?: Error | unknown
  templateVersion?: TypesGen.TemplateVersion
  templateVersionError?: Error | unknown
}

export const createWorkspaceMachine = createMachine(
  {
    tsTypes: {} as import("./createWorkspaceXService.typegen").Typegen0,
    schema: {
      context: {} as CreateWorkspaceContext,
      services: {} as {
        getOrganizations: {
          data: TypesGen.Organization[]
        }
        getPermissions: {
          data: boolean
        }
        getTemplate: {
          data: TypesGen.Template
        }
        getTemplateVersion: {
          data: TypesGen.TemplateVersion
        }
      },
    },
    id: "templateState",
    initial: "gettingOrganizations",
    states: {},
}
)

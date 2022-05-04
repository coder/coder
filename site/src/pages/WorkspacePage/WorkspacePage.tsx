import React from "react"
import { useParams } from "react-router-dom"
import * as Types from "../../api/types"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { Workspace } from "../../components/Workspace/Workspace"
import { firstOrItem } from "../../util/array"


export const WorkspacePage: React.FC = () => {
  const { workspace: workspaceQueryParam } = useParams()
  const workspaceParam = firstOrItem(workspaceQueryParam, null)

  const [workspaceState, workspaceSend] = useActor(workspaceXService)
  const { workspace, template, organization, getWorkspaceError, getTemplateError, getOrganizationError } = workspaceState.context

  if (state.matches('error')) {
    return <ErrorSummary error={getWorkspaceError || getTemplateError || getOrganizationError } />
  }

  if (!workspace || !template || !organization) {
    return <FullScreenLoader />
  }

  return (
    <Margins>
      <Stack spacing={4}>
        <Workspace organization={organization} template={template} workspace={workspace} />
      </Stack>
    </Margins>
  )
}

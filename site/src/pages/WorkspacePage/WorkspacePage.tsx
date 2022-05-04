import React from "react"
import { useParams } from "react-router-dom"
import useSWR from "swr"
import * as Types from "../../api/types"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { Workspace } from "../../components/Workspace/Workspace"
import { unsafeSWRArgument } from "../../util"
import { firstOrItem } from "../../util/array"

export const WorkspacePage: React.FC = () => {
  const { workspace: workspaceQueryParam } = useParams()

  const { data: workspace, error: workspaceError } = useSWR<Types.Workspace, Error>(() => {
    const workspaceParam = firstOrItem(workspaceQueryParam, null)

    return `/api/v2/workspaces/${workspaceParam}`
  })

  // Fetch parent template
  const { data: template, error: templateError } = useSWR<Types.Template, Error>(() => {
    return `/api/v2/templates/${unsafeSWRArgument(workspace).template_id}`
  })

  const { data: organization, error: organizationError } = useSWR<Types.Template, Error>(() => {
    return `/api/v2/organizations/${unsafeSWRArgument(template).organization_id}`
  })

  if (workspaceError) {
    return <ErrorSummary error={workspaceError} />
  }

  if (templateError) {
    return <ErrorSummary error={templateError} />
  }

  if (organizationError) {
    return <ErrorSummary error={organizationError} />
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

import React from "react"
import { useNavigate, useParams } from "react-router-dom"
import useSWR from "swr"
import * as TypesGen from "../../../../api/typesGenerated"
import { ErrorSummary } from "../../../../components/ErrorSummary/ErrorSummary"
import { Header } from "../../../../components/Header/Header"
import { Margins } from "../../../../components/Margins/Margins"
import { Stack } from "../../../../components/Stack/Stack"
import { WorkspacesTable } from "../../../../components/WorkspacesTable/WorkspacesTable"
import { unsafeSWRArgument } from "../../../../util"
import { firstOrItem } from "../../../../util/array"

export const Language = {
  subtitle: "workspaces",
}

export const TemplatePage: React.FC = () => {
  const navigate = useNavigate()
  const { template: templateName, organization: organizationName } = useParams()

  const { data: organizationInfo, error: organizationError } = useSWR<TypesGen.Organization, Error>(
    () => `/api/v2/users/me/organizations/${organizationName}`,
  )

  const { data: templateInfo, error: templateError } = useSWR<TypesGen.Template, Error>(
    () => `/api/v2/organizations/${unsafeSWRArgument(organizationInfo).id}/templates/${templateName}`,
  )

  // This just grabs all workspaces... and then later filters them to match the
  // current template.
  const { data: workspaces, error: workspacesError } = useSWR<TypesGen.Workspace[], Error>(
    () => `/api/v2/organizations/${unsafeSWRArgument(organizationInfo).id}/workspaces`,
  )

  const hasError = organizationError || templateError || workspacesError

  const createWorkspace = () => {
    navigate(`/templates/${organizationName}/${templateName}/create`)
  }

  const perTemplateWorkspaces =
    workspaces && templateInfo
      ? workspaces.filter((workspace) => {
          return workspace.template_id === templateInfo.id
        })
      : undefined

  return (
    <Stack spacing={4}>
      <Header
        title={firstOrItem(templateName, "")}
        description={firstOrItem(organizationName, "")}
        subTitle={perTemplateWorkspaces ? `${perTemplateWorkspaces.length} ${Language.subtitle}` : ""}
        action={{
          text: "Create Workspace",
          onClick: createWorkspace,
        }}
      />

      <Margins>
        {organizationError && <ErrorSummary error={organizationError} />}
        {templateError && <ErrorSummary error={templateError} />}
        {workspacesError && <ErrorSummary error={workspacesError} />}
        {!hasError && (
          <WorkspacesTable templateInfo={templateInfo} workspaces={workspaces} onCreateWorkspace={createWorkspace} />
        )}
      </Margins>
    </Stack>
  )
}

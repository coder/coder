import React from "react"
import { Link, useNavigate, useParams } from "react-router-dom"
import useSWR from "swr"
import { Organization, Template, Workspace, WorkspaceBuild } from "../../../../api/types"
import { EmptyState } from "../../../../components/EmptyState/EmptyState"
import { ErrorSummary } from "../../../../components/ErrorSummary/ErrorSummary"
import { Header } from "../../../../components/Header/Header"
import { FullScreenLoader } from "../../../../components/Loader/FullScreenLoader"
import { Margins } from "../../../../components/Margins/Margins"
import { Stack } from "../../../../components/Stack/Stack"
import { Column, Table } from "../../../../components/Table/Table"
import { unsafeSWRArgument } from "../../../../util"
import { firstOrItem } from "../../../../util/array"

export const TemplatePage: React.FC = () => {
  const navigate = useNavigate()
  const { template: templateName, organization: organizationName } = useParams()

  const { data: organizationInfo, error: organizationError } = useSWR<Organization, Error>(
    () => `/api/v2/users/me/organizations/${organizationName}`,
  )

  const { data: templateInfo, error: templateError } = useSWR<Template, Error>(
    () => `/api/v2/organizations/${unsafeSWRArgument(organizationInfo).id}/templates/${templateName}`,
  )

  // This just grabs all workspaces... and then later filters them to match the
  // current template.
  const { data: workspaces, error: workspacesError } = useSWR<Workspace[], Error>(() => `/api/v2/users/me/workspaces`)

  if (organizationError) {
    return <ErrorSummary error={organizationError} />
  }

  if (templateError) {
    return <ErrorSummary error={templateError} />
  }

  if (workspacesError) {
    return <ErrorSummary error={workspacesError} />
  }

  if (!templateInfo || !workspaces) {
    return <FullScreenLoader />
  }

  const createWorkspace = () => {
    navigate(`/templates/${organizationName}/${templateName}/create`)
  }

  const emptyState = (
    <EmptyState
      button={{
        children: "Create Workspace",
        onClick: createWorkspace,
      }}
      message="No workspaces have been created yet"
      description="Create a workspace to get started"
    />
  )

  const columns: Column<Workspace>[] = [
    {
      key: "name",
      name: "Name",
      renderer: (nameField: string | WorkspaceBuild, workspace: Workspace) => {
        return <Link to={`/workspaces/${workspace.id}`}>{nameField}</Link>
      },
    },
  ]

  const perTemplateWorkspaces = workspaces.filter((workspace) => {
    return workspace.template_id === templateInfo.id
  })

  const tableProps = {
    title: "Workspaces",
    columns,
    data: perTemplateWorkspaces,
    emptyState: emptyState,
  }

  return (
    <Stack spacing={4}>
      <Header
        title={firstOrItem(templateName, "")}
        description={firstOrItem(organizationName, "")}
        subTitle={`${perTemplateWorkspaces.length} workspaces`}
        action={{
          text: "Create Workspace",
          onClick: createWorkspace,
        }}
      />

      <Margins>
        <Table {...tableProps} />
      </Margins>
    </Stack>
  )
}

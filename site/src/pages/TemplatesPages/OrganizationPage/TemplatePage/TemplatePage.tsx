import Box from "@material-ui/core/Box"
import Button from "@material-ui/core/Button"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import React from "react"
import { Link, useNavigate, useParams } from "react-router-dom"
import useSWR from "swr"
import * as TypesGen from "../../../../api/typesGenerated"
import { EmptyState } from "../../../../components/EmptyState/EmptyState"
import { ErrorSummary } from "../../../../components/ErrorSummary/ErrorSummary"
import { Header } from "../../../../components/Header/Header"
import { Margins } from "../../../../components/Margins/Margins"
import { Stack } from "../../../../components/Stack/Stack"
import { TableHeaderRow } from "../../../../components/TableHeaders/TableHeaders"
import { TableLoader } from "../../../../components/TableLoader/TableLoader"
import { TableTitle } from "../../../../components/TableTitle/TableTitle"
import { unsafeSWRArgument } from "../../../../util"
import { firstOrItem } from "../../../../util/array"

export const Language = {
  tableTitle: "Workspaces",
  nameLabel: "Name",
  emptyMessage: "No workspaces have been created yet",
  emptyDescription: "Create a workspace to get started",
  totalLabel: "total",
  ctaAction: "Create workspace",
  subtitlePosfix: "workspaces",
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
  const isLoading = !templateInfo || !workspaces

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
        subTitle={perTemplateWorkspaces ? `${perTemplateWorkspaces.length} ${Language.subtitlePosfix}` : ""}
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
          <Table>
            <TableHead>
              <TableTitle title={Language.tableTitle} />
              <TableHeaderRow>
                <TableCell size="small">{Language.nameLabel}</TableCell>
              </TableHeaderRow>
            </TableHead>
            <TableBody>
              {isLoading && <TableLoader />}
              {workspaces &&
                workspaces.map((w) => (
                  <TableRow key={w.id}>
                    <TableCell>
                      <Link to={`/workspaces/${w.id}`}>{w.name}</Link>
                    </TableCell>
                  </TableRow>
                ))}

              {workspaces && workspaces.length === 0 && (
                <TableRow>
                  <TableCell colSpan={999}>
                    <Box p={4}>
                      <EmptyState
                        message={Language.emptyMessage}
                        description={Language.emptyDescription}
                        cta={
                          <Button variant="contained" color="primary" onClick={createWorkspace}>
                            {Language.ctaAction}
                          </Button>
                        }
                      />
                    </Box>
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        )}
      </Margins>
    </Stack>
  )
}

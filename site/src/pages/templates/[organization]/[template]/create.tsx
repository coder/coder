import { makeStyles } from "@material-ui/core/styles"
import React, { useCallback } from "react"
import { useNavigate, useParams } from "react-router-dom"
import useSWR from "swr"
import * as API from "../../../../api"
import * as Types from "../../../../api/types"
import { ErrorSummary } from "../../../../components/ErrorSummary"
import { FullScreenLoader } from "../../../../components/Loader/FullScreenLoader"
import { CreateWorkspaceForm } from "../../../../forms/CreateWorkspaceForm"
import { unsafeSWRArgument } from "../../../../util"

export const CreateWorkspacePage: React.FC = () => {
  const { organization: organizationName, template: templateName } = useParams()
  const navigate = useNavigate()
  const styles = useStyles()

  const { data: organizationInfo, error: organizationError } = useSWR<Types.Organization, Error>(
    () => `/api/v2/users/me/organizations/${organizationName}`,
  )

  const { data: template, error: templateError } = useSWR<Types.Template, Error>(() => {
    return `/api/v2/organizations/${unsafeSWRArgument(organizationInfo).id}/templates/${templateName}`
  })

  const onCancel = useCallback(async () => {
    navigate(`/templates/${organizationName}/${templateName}`)
  }, [navigate, organizationName, templateName])

  const onSubmit = async (req: Types.CreateWorkspaceRequest) => {
    const workspace = await API.Workspace.create(req)
    navigate(`/workspaces/${workspace.id}`)
    return workspace
  }

  if (organizationError) {
    return <ErrorSummary error={organizationError} />
  }

  if (templateError) {
    return <ErrorSummary error={templateError} />
  }

  if (!template) {
    return <FullScreenLoader />
  }

  return (
    <div className={styles.root}>
      <CreateWorkspaceForm onCancel={onCancel} onSubmit={onSubmit} template={template} />
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
    height: "100vh",
    backgroundColor: theme.palette.background.paper,
  },
}))

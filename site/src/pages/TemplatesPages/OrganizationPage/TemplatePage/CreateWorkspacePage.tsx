import { makeStyles } from "@material-ui/core/styles"
import { useSelector } from "@xstate/react"
import React, { useCallback, useContext } from "react"
import { useNavigate, useParams } from "react-router-dom"
import useSWR from "swr"
import * as API from "../../../../api/api"
import * as TypesGen from "../../../../api/typesGenerated"
import { ErrorSummary } from "../../../../components/ErrorSummary/ErrorSummary"
import { FullScreenLoader } from "../../../../components/Loader/FullScreenLoader"
import { CreateWorkspaceForm } from "../../../../forms/CreateWorkspaceForm"
import { unsafeSWRArgument } from "../../../../util"
import { selectOrgId } from "../../../../xServices/auth/authSelectors"
import { XServiceContext } from "../../../../xServices/StateContext"

export const CreateWorkspacePage: React.FC = () => {
  const { organization: organizationName, template: templateName } = useParams()
  const navigate = useNavigate()
  const styles = useStyles()

  const xServices = useContext(XServiceContext)
  const myOrgId = useSelector(xServices.authXService, selectOrgId)

  const { data: organizationInfo, error: organizationError } = useSWR<TypesGen.Organization, Error>(
    () => `/api/v2/users/me/organizations/${organizationName}`,
  )

  const { data: template, error: templateError } = useSWR<TypesGen.Template, Error>(() => {
    return `/api/v2/organizations/${unsafeSWRArgument(organizationInfo).id}/templates/${templateName}`
  })

  const onCancel = useCallback(async () => {
    navigate(`/templates/${organizationName}/${templateName}`)
  }, [navigate, organizationName, templateName])

  const onSubmit = async (organizationId: string, req: TypesGen.CreateWorkspaceRequest) => {
    const workspace = await API.Workspace.create(organizationId, req)
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

  if (!myOrgId) {
    return <ErrorSummary error={Error("no organization id")} />
  }

  return (
    <div className={styles.root}>
      <CreateWorkspaceForm onCancel={onCancel} onSubmit={onSubmit} template={template} organizationId={myOrgId} />
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

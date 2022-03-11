import React, { useCallback } from "react"
import { makeStyles } from "@material-ui/core/styles"
import { useNavigate, useParams } from "react-router-dom"
import useSWR from "swr"

import * as API from "../../../../api"
import { useUser } from "../../../../contexts/UserContext"
import { ErrorSummary } from "../../../../components/ErrorSummary"
import { FullScreenLoader } from "../../../../components/Loader/FullScreenLoader"
import { CreateWorkspaceForm } from "../../../../forms/CreateWorkspaceForm"
import { unsafeSWRArgument } from "../../../../util"

export const CreateWorkspacePage: React.FC = () => {
  const { organization: organizationName, project: projectName } = useParams()
  const navigate = useNavigate()
  const styles = useStyles()
  const { me } = useUser(/* redirectOnError */ true)

  const { data: organizationInfo, error: organizationError } = useSWR<API.Organization, Error>(
    () => `/api/v2/users/me/organizations/${organizationName}`,
  )

  const { data: project, error: projectError } = useSWR<API.Project, Error>(() => {
    return `/api/v2/organizations/${unsafeSWRArgument(organizationInfo).id}/projects/${projectName}`
  })

  const onCancel = useCallback(async () => {
    navigate(`/projects/${organizationName}/${projectName}`)
  }, [navigate, organizationName, projectName])

  const onSubmit = async (req: API.CreateWorkspaceRequest) => {
    const workspace = await API.Workspace.create(req)
    navigate(`/workspaces/${workspace.id}`)
    return workspace
  }

  if (organizationError) {
    return <ErrorSummary error={organizationError} />
  }

  if (projectError) {
    return <ErrorSummary error={projectError} />
  }

  if (!me || !project) {
    return <FullScreenLoader />
  }

  return (
    <div className={styles.root}>
      <CreateWorkspaceForm onCancel={onCancel} onSubmit={onSubmit} project={project} />
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

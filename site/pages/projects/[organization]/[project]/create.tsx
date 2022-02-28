import React, { useCallback } from "react"
import { makeStyles } from "@material-ui/core/styles"
import { useRouter } from "next/router"
import useSWR from "swr"

import * as API from "../../../../api"
import { useUser } from "../../../../contexts/UserContext"
import { ErrorSummary } from "../../../../components/ErrorSummary"
import { FullScreenLoader } from "../../../../components/Loader/FullScreenLoader"
import { CreateWorkspaceForm } from "../../../../forms/CreateWorkspaceForm"

const CreateWorkspacePage: React.FC = () => {
  const { push, query } = useRouter()
  const styles = useStyles()
  const { me } = useUser(/* redirectOnError */ true)
  const { organization, project: projectName } = query
  const { data: project, error: projectError } = useSWR<API.Project, Error>(
    `/api/v2/projects/${organization}/${projectName}`,
  )

  const onCancel = useCallback(async () => {
    await push(`/projects/${organization}/${projectName}`)
  }, [push, organization, projectName])

  const onSubmit = async (req: API.CreateWorkspaceRequest) => {
    const workspace = await API.Workspace.create(req)
    await push(`/workspaces/me/${workspace.name}`)
    return workspace
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

export default CreateWorkspacePage

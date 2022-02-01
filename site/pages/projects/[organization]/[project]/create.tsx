import React from "react"
import { makeStyles } from "@material-ui/core/styles"
import { useRouter } from "next/router"
import useSWR from "swr"

import * as API from "../../../../api"
import { useUser } from "../../../../contexts/UserContext"
import { ErrorSummary } from "../../../../components/ErrorSummary"
import { FullScreenLoader } from "../../../../components/Loader/FullScreenLoader"
import { CreateWorkspaceForm } from "../../../../forms/CreateWorkspaceForm"

const CreateWorkspacePage: React.FC = () => {
  const router = useRouter()
  const styles = useStyles()
  const { me } = useUser(/* redirectOnError */ true)
  const { organization, project: projectName } = router.query
  const { data: project, error: projectError } = useSWR<API.Project, Error>(
    `/api/v2/projects/${organization}/${projectName}`,
  )

  if (projectError) {
    return <ErrorSummary error={projectError} />
  }

  if (!me || !project) {
    return <FullScreenLoader />
  }

  const onCancel = async () => {
    await router.push(`/projects/${organization}/${projectName}`)
  }

  const onSubmit = async (req: API.CreateWorkspaceRequest) => {
    const workspace = await API.Workspace.create(req)
    await router.push(`/workspaces/${workspace.id}`)
    return workspace
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

import React from "react"
import { makeStyles } from "@material-ui/core/styles"
import { useRouter } from "next/router"
import useSWR from "swr"

import * as API from "../../api"
import { useUser } from "../../contexts/UserContext"
import { ErrorSummary } from "../../components/ErrorSummary"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import { CreateProjectForm } from "../../forms/CreateProjectForm"

const CreateProjectPage: React.FC = () => {
  const router = useRouter()
  const styles = useStyles()
  const { me } = useUser(true)
  const { data: organizations, error } = useSWR("/api/v2/users/me/organizations")

  if (error) {
    return <ErrorSummary error={error} />
  }

  if (!me || !organizations) {
    return <FullScreenLoader />
  }

  const onCancel = async () => {
    await router.push("/projects")
  }

  const onSubmit = async (req: API.CreateProjectRequest) => {
    const project = await API.Project.create(req)
    await router.push(`/projects/${req.organizationId}/${project.name}`)
    return project
  }

  return (
    <div className={styles.root}>
      <CreateProjectForm
        provisioners={API.provisioners}
        organizations={organizations}
        onSubmit={onSubmit}
        onCancel={onCancel}
      />
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

export default CreateProjectPage

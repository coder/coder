import React from "react"
import useSWR from "swr"
import { makeStyles } from "@material-ui/core/styles"
import { useParams } from "react-router-dom"
import { Footer } from "../../components/Page"
import { firstOrItem } from "../../util/array"
import { ErrorSummary } from "../../components/ErrorSummary"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import { Workspace } from "../../components/Workspace"
import { unsafeSWRArgument } from "../../util"
import * as Types from "../../api/types"

export const WorkspacePage: React.FC = () => {
  const styles = useStyles()
  const { workspace: workspaceQueryParam } = useParams()

  const { data: workspace, error: workspaceError } = useSWR<Types.Workspace, Error>(() => {
    const workspaceParam = firstOrItem(workspaceQueryParam, null)

    return `/api/v2/workspaces/${workspaceParam}`
  })

  // Fetch parent project
  const { data: project, error: projectError } = useSWR<Types.Project, Error>(() => {
    return `/api/v2/projects/${unsafeSWRArgument(workspace).project_id}`
  })

  const { data: organization, error: organizationError } = useSWR<Types.Project, Error>(() => {
    return `/api/v2/organizations/${unsafeSWRArgument(project).organization_id}`
  })

  if (workspaceError) {
    return <ErrorSummary error={workspaceError} />
  }

  if (projectError) {
    return <ErrorSummary error={projectError} />
  }

  if (organizationError) {
    return <ErrorSummary error={organizationError} />
  }

  if (!workspace || !project || !organization) {
    return <FullScreenLoader />
  }

  return (
    <div className={styles.root}>
      <div className={styles.inner}>
        <Workspace organization={organization} project={project} workspace={workspace} />
      </div>

      <Footer />
    </div>
  )
}

const useStyles = makeStyles(() => ({
  root: {
    display: "flex",
    flexDirection: "column",
  },
  inner: {
    maxWidth: "1380px",
    margin: "1em auto",
    width: "100%",
  },
}))

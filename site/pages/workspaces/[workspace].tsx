import React from "react"
import useSWR from "swr"
import { makeStyles } from "@material-ui/core/styles"
import { useRouter } from "next/router"
import { Navbar } from "../../components/Navbar"
import { Footer } from "../../components/Page"
import { useUser } from "../../contexts/UserContext"
import { firstOrItem } from "../../util/array"
import { ErrorSummary } from "../../components/ErrorSummary"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import { Workspace } from "../../components/Workspace"

import * as API from "../../api"

const WorkspacesPage: React.FC = () => {
  const styles = useStyles()
  const router = useRouter()
  const { me, signOut } = useUser(true)

  const { workspace: workspaceQueryParam } = router.query

  const { data: workspace, error: workspaceError } = useSWR<API.Workspace, Error>(() => {
    const workspaceParam = firstOrItem(workspaceQueryParam, null)

    return `/api/v2/workspaces/${workspaceParam}`
  })

  if (workspaceError) {
    return <ErrorSummary error={workspaceError} />
  }

  if (!me || !workspace) {
    return <FullScreenLoader />
  }

  return (
    <div className={styles.root}>
      <Navbar user={me} onSignOut={signOut} />

      <div className={styles.inner}>
        <Workspace workspace={workspace} />
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

export default WorkspacesPage

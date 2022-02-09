import React from "react"
import { makeStyles } from "@material-ui/core/styles"
import { useRouter } from "next/router"
import { Navbar } from "../../components/Navbar"
import { Footer } from "../../components/Page"
import { useUser } from "../../contexts/UserContext"

//import { Workspace } from "../../components/Workspace"
//import { MockWorkspace } from "../../test_helpers"

const WorkspacesPage: React.FC = () => {
  const styles = useStyles()
  const router = useRouter()
  const { me, signOut } = useUser(true)

  const { user: userQueryParam, workspace: workspaceQueryParam } = router.query

  const userParam = firstOrDefault(userQueryParam, null)
  const workspaceParam = firstOrDefault(workspaceQueryParam, null)

  return (
    <div className={styles.root}>
      <Navbar user={me} onSignOut={signOut} />

      <div className={styles.inner}>
        <div>Hello, world</div>
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

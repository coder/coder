import React from "react"
import { makeStyles } from "@material-ui/core/styles"
import Paper from "@material-ui/core/Paper"
import { useRouter } from "next/router"
import Link from "next/link"
import { Navbar } from "../../../../components/Navbar"
import { Footer } from "../../../../components/Page"
import { useUser } from "../../../../contexts/UserContext"

import { Workspace } from "../../../../components/Workspace"
import { MockWorkspace } from "../../../../test_helpers"

const WorkspacesPage: React.FC = () => {
  const styles = useStyles()
  const router = useRouter()
  const { me, signOut } = useUser(true)

  const { user, workspace } = router.query

  return (
    <div className={styles.root}>
      <Navbar user={me} onSignOut={signOut} />

      <div className={styles.inner}>
        <Workspace workspace={MockWorkspace} />
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

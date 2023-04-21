import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import { SignInLayout } from "components/SignInLayout/SignInLayout"
import { Welcome } from "components/Welcome/Welcome"
import { FC, useEffect } from "react"
import { Link as RouterLink } from "react-router-dom"
import { REFRESH_GITAUTH_BROADCAST_CHANNEL } from "xServices/createWorkspace/createWorkspaceXService"

const GitAuthPage: FC = () => {
  const styles = useStyles()
  useEffect(() => {
    // This is used to notify the parent window that the Git auth token has been refreshed.
    // It's critical in the create workspace flow!
    // eslint-disable-next-line compat/compat -- It actually is supported... not sure why it's complaining.
    const bc = new BroadcastChannel(REFRESH_GITAUTH_BROADCAST_CHANNEL)
    // The message doesn't matter, any message refreshes the page!
    bc.postMessage("noop")
    window.close()
  }, [])

  return (
    <SignInLayout>
      <Welcome message="Authenticated with Git!" />
      <p className={styles.text}>
        Your Git authentication token will be refreshed to keep you signed in.
      </p>

      <div className={styles.links}>
        <Button
          component={RouterLink}
          size="large"
          to="/workspaces"
          fullWidth
          variant="outlined"
        >
          Go to workspaces
        </Button>
      </div>
    </SignInLayout>
  )
}

export default GitAuthPage

const useStyles = makeStyles((theme) => ({
  title: {
    fontSize: theme.spacing(4),
    fontWeight: 400,
    lineHeight: "140%",
    margin: 0,
  },

  text: {
    fontSize: 16,
    color: theme.palette.text.secondary,
    marginBottom: theme.spacing(4),
    textAlign: "center",
    lineHeight: "160%",
  },

  lineBreak: {
    whiteSpace: "nowrap",
  },

  links: {
    display: "flex",
    justifyContent: "flex-end",
    paddingTop: theme.spacing(1),
  },
}))

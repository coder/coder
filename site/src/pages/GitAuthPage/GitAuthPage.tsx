import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import { SignInLayout } from "components/SignInLayout/SignInLayout"
import { Welcome } from "components/Welcome/Welcome"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"

const GitAuthPage: FC = () => {
  const styles = useStyles()

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

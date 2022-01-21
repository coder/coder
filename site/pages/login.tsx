import { makeStyles } from "@material-ui/core/styles"
import React from "react"
import { SignInForm } from "./../components/SignIn"

export const useStyles = makeStyles((theme) => ({
  root: {
    height: "100vh",
    display: "flex",
    justifyContent: "center",
    alignItems: "center",
  },
  container: {
    marginTop: theme.spacing(-8),
    minWidth: "320px",
    maxWidth: "320px",
  },
}))

export const SignInPage: React.FC = () => {
  const styles = useStyles()
  return (
    <div className={styles.root}>
      <div className={styles.container}>
        <SignInForm />
      </div>
    </div>
  )
}

export default SignInPage

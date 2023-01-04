import { makeStyles } from "@material-ui/core/styles"
import { FC, ReactNode } from "react"

export const useStyles = makeStyles((theme) => ({
  root: {
    height: "100vh",
    display: "flex",
    justifyContent: "center",
    alignItems: "center",
  },
  layout: {
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
  },
  container: {
    marginTop: theme.spacing(-8),
    minWidth: "320px",
    maxWidth: "320px",
  },
  copyright: {
    marginTop: theme.spacing(2),
  },
}))

export const SignInLayout: FC<{ children: ReactNode }> = ({ children }) => {
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <div className={styles.layout}>
        <div className={styles.container}>{children}</div>
        <div className={styles.copyright}>
          {`\u00a9 ${new Date().getFullYear()} Coder Technologies, Inc.`}
        </div>
      </div>
    </div>
  )
}

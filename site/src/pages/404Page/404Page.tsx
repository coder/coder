import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import { FC } from "react"

export const NotFoundPage: FC<React.PropsWithChildren<unknown>> = () => {
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <div className={styles.headingContainer}>
        <Typography variant="h4">404</Typography>
      </div>
      <Typography variant="body2">This page could not be found.</Typography>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    width: "100vw",
    height: "100vh",
    display: "flex",
    flexDirection: "row",
    justifyContent: "center",
    alignItems: "center",
  },
  headingContainer: {
    margin: theme.spacing(1),
    padding: theme.spacing(1),
    borderRight: theme.palette.divider,
  },
}))

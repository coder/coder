import { makeStyles } from "@material-ui/core/styles"
import { Stack } from "../Stack/Stack"

export const PageHeader: React.FC = ({ children }) => {
  const styles = useStyles()

  return <div className={styles.root}>{children}</div>
}

export const PageHeaderTitle: React.FC = ({ children }) => {
  const styles = useStyles()

  return <h1 className={styles.title}>{children}</h1>
}

export const PageHeaderActions: React.FC = ({ children }) => {
  const styles = useStyles()

  return (
    <Stack direction="row" className={styles.actions}>
      {children}
    </Stack>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    display: "flex",
    alignItems: "center",
    paddingTop: theme.spacing(6),
    paddingBottom: theme.spacing(5),
  },

  title: {
    fontSize: theme.spacing(4),
    fontWeight: 400,
    margin: 0,
    display: "flex",
    alignItems: "center",
  },

  actions: {
    marginLeft: "auto",
  },
}))

import { makeStyles } from "@material-ui/core/styles"
import { FC, PropsWithChildren } from "react"

export const FullWidthPageHeader: FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles()

  return (
    <header className={styles.header} data-testid="header">
      {children}
    </header>
  )
}

export const PageHeaderActions: FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles()
  return <div className={styles.actions}>{children}</div>
}

export const PageHeaderTitle: FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles()
  return <h1 className={styles.title}>{children}</h1>
}

export const PageHeaderSubtitle: FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles()
  return <span className={styles.subtitle}>{children}</span>
}

const useStyles = makeStyles((theme) => ({
  header: {
    padding: theme.spacing(3),
    background: theme.palette.background.paper,
    borderBottom: `1px solid ${theme.palette.divider}`,
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(6),
    position: "sticky",
    top: 0,
    zIndex: 10,
    flexWrap: "wrap",

    [theme.breakpoints.down("md")]: {
      position: "unset",
      alignItems: "flex-start",
    },
    [theme.breakpoints.down("sm")]: {
      flexDirection: "column",
    },
  },
  actions: {
    marginLeft: "auto",
    [theme.breakpoints.down("sm")]: {
      marginLeft: "unset",
    },
  },
  title: {
    fontSize: 18,
    fontWeight: 500,
    margin: 0,
  },
  subtitle: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    marginTop: theme.spacing(0.25),
    display: "block",
  },
}))

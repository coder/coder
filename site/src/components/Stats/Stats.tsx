import { makeStyles } from "@material-ui/core/styles"
import { ComponentProps, FC, PropsWithChildren } from "react"

export const Stats: FC<PropsWithChildren<ComponentProps<"div">>> = (props) => {
  const styles = useStyles()
  return <div className={styles.stats} {...props} />
}

export const StatsItem: FC<{
  label: string
  value: string | number | JSX.Element
}> = ({ label, value }) => {
  const styles = useStyles()

  return (
    <div className={styles.statItem}>
      <span className={styles.statsLabel}>{label}:</span>
      <span className={styles.statsValue}>{value}</span>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  stats: {
    paddingLeft: theme.spacing(2),
    paddingRight: theme.spacing(2),
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.divider}`,
    display: "flex",
    alignItems: "center",
    color: theme.palette.text.secondary,
    margin: "0px",

    [theme.breakpoints.down("sm")]: {
      display: "block",
      padding: theme.spacing(2),
    },
  },

  statItem: {
    padding: theme.spacing(1.75),
    paddingLeft: theme.spacing(2),
    paddingRight: theme.spacing(2),
    display: "flex",
    alignItems: "baseline",
    gap: theme.spacing(1),

    [theme.breakpoints.down("sm")]: {
      padding: theme.spacing(1),
    },
  },

  statsLabel: {
    display: "block",
    wordWrap: "break-word",
  },

  statsValue: {
    marginTop: theme.spacing(0.25),
    display: "flex",
    wordWrap: "break-word",
    color: theme.palette.text.primary,
    alignItems: "center",

    "& a": {
      color: theme.palette.text.primary,
      textDecoration: "none",
      fontWeight: 600,

      "&:hover": {
        textDecoration: "underline",
      },
    },
  },
}))

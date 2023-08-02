import Box from "@mui/material/Box"
import { makeStyles } from "@mui/styles"
import { ComponentProps, FC, PropsWithChildren } from "react"
import { combineClasses } from "utils/combineClasses"

export const Stats: FC<ComponentProps<typeof Box>> = (props) => {
  const styles = useStyles()
  return (
    <Box
      {...props}
      className={combineClasses([styles.stats, props.className])}
    />
  )
}

export const StatsItem: FC<
  {
    label: string
    value: string | number | JSX.Element
  } & ComponentProps<typeof Box>
> = ({ label, value, ...divProps }) => {
  const styles = useStyles()

  return (
    <Box
      {...divProps}
      className={combineClasses([styles.statItem, divProps.className])}
    >
      <span className={styles.statsLabel}>{label}:</span>
      <span className={styles.statsValue}>{value}</span>
    </Box>
  )
}

const useStyles = makeStyles((theme) => ({
  stats: {
    ...theme.typography.body2,
    paddingLeft: theme.spacing(2),
    paddingRight: theme.spacing(2),
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.divider}`,
    display: "flex",
    alignItems: "center",
    color: theme.palette.text.secondary,
    margin: "0px",
    flexWrap: "wrap",

    [theme.breakpoints.down("md")]: {
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

    [theme.breakpoints.down("md")]: {
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

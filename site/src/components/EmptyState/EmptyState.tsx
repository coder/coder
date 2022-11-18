import Box from "@material-ui/core/Box"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import { FC, ReactNode } from "react"
import { combineClasses } from "../../util/combineClasses"

export interface EmptyStateProps {
  /** Text Message to display, placed inside Typography component */
  message: string
  /** Longer optional description to display below the message */
  description?: string | React.ReactNode
  descriptionClassName?: string
  cta?: ReactNode
  className?: string
}

/**
 * Component to place on screens or in lists that have no content. Optionally
 * provide a button that would allow the user to return from where they were,
 * or to add an item that they currently have none of.
 *
 * EmptyState's props extend the [Material UI Box component](https://material-ui.com/components/box/)
 * that you can directly pass props through to to customize the shape and layout of it.
 */
export const EmptyState: FC<React.PropsWithChildren<EmptyStateProps>> = (
  props,
) => {
  const {
    message,
    description,
    cta,
    descriptionClassName,
    className,
    ...boxProps
  } = props
  const styles = useStyles()

  return (
    <Box className={combineClasses([styles.root, className])} {...boxProps}>
      <Typography variant="h5" className={styles.title}>
        {message}
      </Typography>
      {description && (
        <Typography
          variant="body2"
          color="textSecondary"
          className={combineClasses([styles.description, descriptionClassName])}
        >
          {description}
        </Typography>
      )}
      {cta && <div className={styles.cta}>{cta}</div>}
    </Box>
  )
}

const useStyles = makeStyles(
  (theme) => ({
    root: {
      display: "flex",
      flexDirection: "column",
      justifyContent: "center",
      alignItems: "center",
      textAlign: "center",
      minHeight: 300,
      padding: theme.spacing(3),
    },

    title: {
      fontSize: theme.spacing(3),
    },
    description: {
      marginTop: theme.spacing(1.5),
      fontSize: theme.spacing(2),
    },
    cta: {
      marginTop: theme.spacing(4),
    },
  }),
  { name: "EmptyState" },
)

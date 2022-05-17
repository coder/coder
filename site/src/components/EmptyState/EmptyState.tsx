import Box from "@material-ui/core/Box"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import React from "react"

export interface EmptyStateProps {
  /** Text Message to display, placed inside Typography component */
  message: string
  /** Longer optional description to display below the message */
  description?: string
  cta?: React.ReactNode
}

/**
 * Component to place on screens or in lists that have no content. Optionally
 * provide a button that would allow the user to return from where they were,
 * or to add an item that they currently have none of.
 *
 * EmptyState's props extend the [Material UI Box component](https://material-ui.com/components/box/)
 * that you can directly pass props through to to customize the shape and layout of it.
 */
export const EmptyState: React.FC<EmptyStateProps> = (props) => {
  const { message, description, cta, ...boxProps } = props
  const styles = useStyles()

  return (
    <Box className={styles.root} {...boxProps}>
      <div className={styles.header}>
        <Typography variant="h5" className={styles.title}>
          {message}
        </Typography>
        {description && (
          <Typography variant="body2" color="textSecondary" className={styles.description}>
            {description}
          </Typography>
        )}
      </div>
      {cta}
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
      minHeight: 120,
      padding: theme.spacing(3),
    },
    header: {
      marginBottom: theme.spacing(3),
    },
    title: {
      fontWeight: 400,
    },
    description: {
      marginTop: theme.spacing(1),
    },
  }),
  { name: "EmptyState" },
)

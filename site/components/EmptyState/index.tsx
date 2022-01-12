import React from "react"
import { makeStyles } from "@material-ui/core/styles"
import Box from "@material-ui/core/Box"
import Button, { ButtonProps } from "@material-ui/core/Button"
import Typography from "@material-ui/core/Typography"

export interface EmptyStateProps {
  /** Text Message to display, placed inside Typography component */
  message: React.ReactNode
  /** Longer optional description to display below the message */
  description?: React.ReactNode
  button?: ButtonProps
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
  const { message, description, button, ...boxProps } = props
  const styles = useStyles()

  const descClassName = `${styles.description}`
  const buttonClassName = `${styles.button} ${button && button.className ? button.className : ""}`

  return (
    <Box className={styles.root} {...boxProps}>
      <Typography variant="h5" color="textSecondary" className={styles.header}>
        {message}
      </Typography>
      {description && (
        <Typography variant="body2" color="textSecondary" className={descClassName}>
          {description}
        </Typography>
      )}
      {button && <Button variant="contained" color="primary" {...button} className={buttonClassName} />}
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
      fontWeight: 400,
    },
    description: {
      marginTop: theme.spacing(2),
      marginBottom: theme.spacing(1),
    },
    button: {
      marginTop: theme.spacing(2),
    },
    icon: {
      fontSize: theme.typography.h2.fontSize,
      color: theme.palette.text.secondary,
      marginBottom: theme.spacing(1),
      opacity: 0.5,
    },
  }),
  { name: "EmptyState" },
)
